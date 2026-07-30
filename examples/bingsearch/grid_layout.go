package bingsearch

import (
	"math"
	"os"
	"strconv"
	"strings"
)

// The layout below started as a PORT of
// d:/code/tu/multiprofile-v2/multiprofile-v2-auto/pkg/screen (struct.go,
// screen_windows.go, screen_other.go), then diverged twice against evidence
// from real runs.
//
// First: the ported sha1-jitter plus 0.85-1.0 size randomisation spilled
// windows past their cells and stacked them. Chrome enforces a minimum window
// size (roughly 500x350 physical) that a 384x216 cell — 25 windows on
// 1920x1080 — falls well under, and it clamps up to that minimum no matter
// what --window-size asked for. Clamped windows overlap. Exact table tiling
// alone cannot undo that clamp, so undersized cells also carry a Scale factor
// the caller hands to the browser's own UI-scale option (Chrome
// --force-device-scale-factor, Firefox layout.css.devPixelsPerPx): shrinking
// the whole browser UI shrinks its physical-pixel minimum right along with it.
//
// Second: the ported SPI_GETWORKAREA + GetDpiForMonitor measurement divided by
// the DPI scale a second time. A Go binary ships no DPI manifest, so it runs
// DPI-unaware and Windows already hands it virtualised (logical) coordinates —
// dividing again shrank the work area twice over. On a 1920x1080 display at
// 150% that yielded 853x480 and the tiling covered under half the screen. The
// measurement is gone: these examples take a fixed 1920x1080 unless
// overridden, which is what an operator actually wants from a demo and removes
// a whole class of DPI bugs from example code.
//
// Also dropped from the original: the isMobile branch (these examples only
// launch desktop browsers) and the math/rand getWidthFromString helper — that
// hash exists upstream to evade fingerprinting, which is irrelevant here and
// would make runs non-reproducible.

// Work-area size used for tiling. Fixed rather than measured — see above.
// Override with FLEET_SCREEN_SIZE=<width>x<height> for a different display.
const (
	defaultScreenWidth  = 1920
	defaultScreenHeight = 1080

	screenSizeEnv = "FLEET_SCREEN_SIZE"
)

// LogicalViewport is the CSS viewport every window keeps regardless of how
// small its physical cell is: the page always lays out as if on a full
// 1920x1080 desktop. Chrome gets this verbatim as --window-size and shrinks the
// result with --force-device-scale-factor; Firefox gets it via
// layout.css.devPixelsPerPx plus an explicit window rect.
//
// Sizing --window-size down to the cell instead does NOT work, and the failure
// is worth recording. Measured with 25 windows and scale 0.617: every window
// came back 321px wide at 236px spacing, overlapping by 85px. Chrome multiplies
// BOTH --window-size and --window-position by the forced device scale factor,
// so a 384px request became 237px physical — under Chrome's own minimum window
// width, which it then clamped back up to ~321 while leaving the (also scaled)
// positions 236 apart. Pinning the viewport at 1920x1080 and letting the scale
// factor do the shrinking keeps the physical size well clear of that minimum.
const (
	LogicalViewportWidth  = 1920
	LogicalViewportHeight = 1080
)

// GridSize is a screen or window dimension in logical pixels.
type GridSize struct {
	Width  int
	Height int
}

// GridCell is the placement for one instance: top-left corner and footprint in
// logical pixels, plus the UI scale factor the browser should apply to itself
// (see GridLayout.Cell for why Scale exists).
type GridCell struct {
	X, Y          int
	Width, Height int
	Scale         float64
}

// GridLayout is an n-window EXACT table tiling of the work area: Cols*Rows >= n,
// every cell the same size (work area / Cols|Rows, integer division), no
// jitter, no random sizing — deterministic and, by construction,
// non-overlapping. Build with NewGridLayout(n), then call Cell(i, id) once per
// instance.
type GridLayout struct {
	DisplaySize GridSize
	Cols, Rows  int
}

// NewGridLayout tiles n windows over the configured work area.
func NewGridLayout(n int) *GridLayout {
	return NewGridLayoutForSize(workAreaSize(), n)
}

// NewGridLayoutForSize is NewGridLayout with the work area supplied by the
// caller — the seam that pins the row/col/scale arithmetic in tests without
// depending on the machine's display.
func NewGridLayoutForSize(size GridSize, n int) *GridLayout {
	if n < 1 {
		n = 1
	}
	cols := int(math.Ceil(math.Sqrt(float64(n))))
	rows := int(math.Ceil(float64(n) / float64(cols)))
	return &GridLayout{DisplaySize: size, Cols: cols, Rows: rows}
}

// Cell returns the placement for instance index (0-based): cell k occupies
// exactly (col*cellW, row*cellH) sized cellW x cellH in PHYSICAL screen pixels.
// Adjacent cells share an edge and never overlap — integer division only
// undershoots the work area.
//
// Scale is the factor that shrinks a full LogicalViewport down into that cell:
// min(cellW/1920, cellH/1080). The page still lays out at 1920x1080; only the
// physical window shrinks.
//
// id is accepted for symmetry with earlier call sites; the tiling no longer
// depends on it, since there is no jitter left to seed.
func (g *GridLayout) Cell(index int, id string) GridCell {
	cols, rows := g.Cols, g.Rows
	if cols < 1 {
		cols = 1
	}
	if rows < 1 {
		rows = 1
	}
	cellW := g.DisplaySize.Width / cols
	cellH := g.DisplaySize.Height / rows

	scale := math.Min(
		float64(cellW)/float64(LogicalViewportWidth),
		float64(cellH)/float64(LogicalViewportHeight),
	)
	if scale > 1 {
		scale = 1
	}

	return GridCell{
		X:      cellW * (index % cols),
		Y:      cellH * (index / cols),
		Width:  cellW,
		Height: cellH,
		Scale:  scale,
	}
}

// ChromeWindowPosition converts the cell's physical top-left into the value
// Chrome's --window-position expects. Chrome multiplies that flag by the forced
// device scale factor, so it has to be pre-divided by the same factor to land
// where intended — measured directly: at scale 0.617, --window-position=384
// placed the window at physical x=236 (384 x 0.617).
func (c GridCell) ChromeWindowPosition() (x, y int) {
	if c.Scale <= 0 {
		return c.X, c.Y
	}
	return int(math.Round(float64(c.X) / c.Scale)), int(math.Round(float64(c.Y) / c.Scale))
}

// workAreaSize returns the FLEET_SCREEN_SIZE override when it parses, else the
// fixed default. A malformed value falls back rather than failing — a typo in
// an env var should not stop the demo.
func workAreaSize() GridSize {
	if w, h, ok := parseScreenSize(os.Getenv(screenSizeEnv)); ok {
		return GridSize{Width: w, Height: h}
	}
	return GridSize{Width: defaultScreenWidth, Height: defaultScreenHeight}
}

// parseScreenSize accepts "<width>x<height>", e.g. "2560x1440".
func parseScreenSize(s string) (w, h int, ok bool) {
	parts := strings.SplitN(strings.ToLower(strings.TrimSpace(s)), "x", 2)
	if len(parts) != 2 {
		return 0, 0, false
	}
	w, err := strconv.Atoi(strings.TrimSpace(parts[0]))
	if err != nil || w <= 0 {
		return 0, 0, false
	}
	h, err = strconv.Atoi(strings.TrimSpace(parts[1]))
	if err != nil || h <= 0 {
		return 0, 0, false
	}
	return w, h, true
}
