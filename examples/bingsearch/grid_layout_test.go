package bingsearch

import (
	"fmt"
	"math"
	"testing"
)

// TestGridLayout_CellsDoNotOverlapAndStayInBounds pins the two properties that
// are the whole point of the table tiling: two instances never claim
// overlapping screen space, and no instance is placed partly off the work
// area. Both hold by construction (exact division, no jitter), so this guards
// against a future edit reintroducing the overlap the examples already hit
// once in a real 25-window run.
func TestGridLayout_CellsDoNotOverlapAndStayInBounds(t *testing.T) {
	for _, n := range []int{1, 2, 3, 9, 25, 26} {
		t.Run(fmt.Sprintf("n=%d", n), func(t *testing.T) {
			layout := NewGridLayout(n)
			cells := make([]GridCell, n)
			for i := range cells {
				cells[i] = layout.Cell(i, fmt.Sprintf("id-%d", i))
			}
			for i, c := range cells {
				if c.Width <= 0 || c.Height <= 0 {
					t.Fatalf("cell %d has non-positive size %dx%d", i, c.Width, c.Height)
				}
				if c.X < 0 || c.Y < 0 ||
					c.X+c.Width > layout.DisplaySize.Width ||
					c.Y+c.Height > layout.DisplaySize.Height {
					t.Errorf("cell %d (%d,%d %dx%d) escapes work area %dx%d",
						i, c.X, c.Y, c.Width, c.Height,
						layout.DisplaySize.Width, layout.DisplaySize.Height)
				}
			}
			for i := 0; i < len(cells); i++ {
				for j := i + 1; j < len(cells); j++ {
					if overlaps(cells[i], cells[j]) {
						t.Errorf("cell %d %v overlaps cell %d %v", i, cells[i], j, cells[j])
					}
				}
			}
		})
	}
}

// TestGridLayout_DefaultWorkAreaIs1920x1080 pins the fix for the measurement
// bug: the ported SPI_GETWORKAREA path divided by the DPI scale on top of
// coordinates Windows had already virtualised, so a 1920x1080 display at 150%
// tiled into 853x480 and covered under half the screen. The size is now fixed.
func TestGridLayout_DefaultWorkAreaIs1920x1080(t *testing.T) {
	t.Setenv(screenSizeEnv, "")
	got := NewGridLayout(4).DisplaySize
	if got.Width != 1920 || got.Height != 1080 {
		t.Errorf("default work area = %dx%d, want 1920x1080", got.Width, got.Height)
	}
}

// TestGridLayout_ScaleFitsLogicalViewportIntoCell pins what Scale means: the
// factor that squeezes a full 1920x1080 page layout into the cell's physical
// footprint. The page never gets a smaller viewport — only the window shrinks.
func TestGridLayout_ScaleFitsLogicalViewportIntoCell(t *testing.T) {
	full := GridSize{Width: 1920, Height: 1080}

	for _, tc := range []struct {
		n    int
		want float64 // cellW / LogicalViewportWidth
	}{
		{1, 1.0},           // 1920x1080 cell — no shrinking
		{9, 640.0 / 1920},  // 3x3 → 640x360 cell
		{25, 384.0 / 1920}, // 5x5 → 384x216 cell
	} {
		got := NewGridLayoutForSize(full, tc.n).Cell(0, "id").Scale
		if math.Abs(got-tc.want) > 1e-9 {
			t.Errorf("n=%d scale = %v, want %v", tc.n, got, tc.want)
		}
	}
}

// TestChromeWindowPosition_PreDividesByScale pins the measured Chrome
// behaviour this whole arrangement exists for: Chrome multiplies
// --window-position by the forced device scale factor. Observed directly at
// scale 0.617 — a request of 384 put the window at physical x=236. So the flag
// has to carry cellX/scale for the window to land on its cell.
func TestChromeWindowPosition_PreDividesByScale(t *testing.T) {
	layout := NewGridLayoutForSize(GridSize{Width: 1920, Height: 1080}, 25)
	for _, idx := range []int{0, 1, 4, 5, 24} {
		cell := layout.Cell(idx, "id")
		px, py := cell.ChromeWindowPosition()
		// Chrome will multiply back down by Scale — that must reproduce the
		// cell's real top-left, within a pixel of integer rounding.
		if gotX := int(math.Round(float64(px) * cell.Scale)); math.Abs(float64(gotX-cell.X)) > 1 {
			t.Errorf("cell %d: pos %d x scale %v = %d, want %d", idx, px, cell.Scale, gotX, cell.X)
		}
		if gotY := int(math.Round(float64(py) * cell.Scale)); math.Abs(float64(gotY-cell.Y)) > 1 {
			t.Errorf("cell %d: pos %d x scale %v = %d, want %d", idx, py, cell.Scale, gotY, cell.Y)
		}
	}
}

// TestGridLayout_CellIsDeterministic — the same index must always land in the
// same place, so a rerun puts every profile back where the operator saw it.
func TestGridLayout_CellIsDeterministic(t *testing.T) {
	layout := NewGridLayoutForSize(GridSize{Width: 1920, Height: 1080}, 25)
	for i := 0; i < 25; i++ {
		a := layout.Cell(i, "id")
		b := layout.Cell(i, "id")
		if a != b {
			t.Fatalf("cell %d not deterministic: %v vs %v", i, a, b)
		}
	}
}

func TestParseScreenSize(t *testing.T) {
	for _, tc := range []struct {
		in   string
		w, h int
		ok   bool
	}{
		{"1920x1080", 1920, 1080, true},
		{"2560X1440", 2560, 1440, true},
		{" 1280 x 720 ", 1280, 720, true},
		{"", 0, 0, false},
		{"1920", 0, 0, false},
		{"1920x", 0, 0, false},
		{"0x1080", 0, 0, false},
		{"-1920x1080", 0, 0, false},
		{"widexhigh", 0, 0, false},
	} {
		w, h, ok := parseScreenSize(tc.in)
		if ok != tc.ok || w != tc.w || h != tc.h {
			t.Errorf("parseScreenSize(%q) = (%d,%d,%v), want (%d,%d,%v)",
				tc.in, w, h, ok, tc.w, tc.h, tc.ok)
		}
	}
}

// TestWorkAreaSize_OverrideAndFallback — a valid override wins; a malformed
// one falls back to the default rather than failing, since a typo in an env
// var should not stop the demo from running.
func TestWorkAreaSize_OverrideAndFallback(t *testing.T) {
	t.Setenv(screenSizeEnv, "2560x1440")
	if got := workAreaSize(); got.Width != 2560 || got.Height != 1440 {
		t.Errorf("override work area = %dx%d, want 2560x1440", got.Width, got.Height)
	}

	t.Setenv(screenSizeEnv, "not-a-size")
	if got := workAreaSize(); got.Width != 1920 || got.Height != 1080 {
		t.Errorf("malformed override work area = %dx%d, want fallback 1920x1080", got.Width, got.Height)
	}
}

func overlaps(a, b GridCell) bool {
	return a.X < b.X+b.Width && b.X < a.X+a.Width &&
		a.Y < b.Y+b.Height && b.Y < a.Y+a.Height
}
