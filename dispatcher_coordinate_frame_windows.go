//go:build windows

package automationfleet

import (
	"fmt"
	"math"
)

// coordFrame is the ONE coordinate frame a critical section computes with. It
// is measured once, before anything moves, and then handed to both sides of the
// click: the kit converts CSS to screen pixels through it, and the drift guard
// predicts where the cursor should end up from the same numbers.
//
// Measuring twice is what produced the false-drift bugs. The kit and the guard
// read the same sources, so "same source" was never the problem — a window that
// moves between the two reads makes two correct measurements disagree, and the
// guard reports a perfect click as drift.
type coordFrame struct {
	// offsetX, offsetY are the ABSOLUTE screen position of CSS (0,0). Both kits
	// return it with that meaning, so nothing may add a window origin to it.
	offsetX, offsetY int
	// scale is the DPI factor the kit is actually converting with, measured
	// from the live window rather than declared on the handle before launch.
	scale float64
	// rect is the out-of-band window rectangle at pin time. Comparing it again
	// after the glide is how a window that moved is told apart from a cursor
	// that drifted.
	rect Rect
	// haveRect is false when the window manager could not be asked, in which
	// case the moved-window check is skipped rather than guessed at.
	haveRect bool
}

// pinFrame measures the coordinate frame once and pushes it into the kit, so
// the click and the guard that checks it are computed from the same numbers.
//
// A frame that cannot be measured fails the job. The kit refuses a content
// origin it cannot reconcile with the window manager, and that refusal exists
// precisely because the alternative is driving the physical cursor with
// coordinates page script chose — swallowing the error here would hand it back.
func pinFrame(h *BrowserHandle) (coordFrame, error) {
	if err := h.Driver.InvalidateNativeWindow(); err != nil {
		return coordFrame{}, fmt.Errorf("pin frame: %w", err)
	}
	ox, oy, err := h.Driver.ContentOffset()
	if err != nil {
		return coordFrame{}, fmt.Errorf("pin frame: contentOffset: %w", err)
	}
	scale, err := h.Driver.WindowScale()
	if err != nil || scale <= 0 {
		return coordFrame{}, fmt.Errorf("pin frame: windowScale: %w (scale %g)", err, scale)
	}
	fr := coordFrame{offsetX: ox, offsetY: oy, scale: scale}
	if rect, err := h.Driver.WindowRect(); err == nil {
		fr.rect, fr.haveRect = rect, true
	}
	return fr, nil
}

// screenFor converts CSS pixels to the absolute screen pixel the kit will move
// the cursor to. Mirrors both kits' cssToScreen for the absolute-offset frame.
func (fr coordFrame) screenFor(cssX, cssY float64) (int, int) {
	return fr.offsetX + int(math.Round(cssX*fr.scale)),
		fr.offsetY + int(math.Round(cssY*fr.scale))
}
