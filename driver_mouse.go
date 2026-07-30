package automationfleet

// Mouse mirrors the kits' MouseAPI surface. Every exported method on both
// chromekit.MouseAPI and firefoxkit.MouseAPI has an identical name and a
// primitive-only signature (x/y float64, selector string), so the full set
// wraps cleanly with no type divergence and no exemptions — see
// TestConformance_AdapterCoversMouse in parity_test.go, which enumerates
// both kits' MouseAPI reflectively and fails if this interface (or an
// adapter) drifts from it.
type Mouse interface {
	// MoveTo glides the cursor to (x, y) in viewport CSS pixels, no click.
	MoveTo(x, y float64) error
	// ClickAt clicks at (x, y) with the left button — no element lookup.
	ClickAt(x, y float64) error
	// Click moves to the element's center with a human-like path, then
	// left-clicks.
	Click(selector string) error
	// RightClick clicks the element's center with the right button. The kit
	// does not auto-dismiss the resulting context menu.
	RightClick(selector string) error
	// MiddleClick clicks at (x, y) with the middle button.
	MiddleClick(x, y float64) error
	// DoubleClick fires two left clicks at the element's center within the
	// browser's double-click window.
	DoubleClick(selector string) error
	// Hover moves the cursor to the element's center without clicking.
	Hover(selector string) error
	// Scroll dispatches a wheel event with the given deltas.
	Scroll(deltaX, deltaY float64) error
	// Drag presses at (fromX, fromY), glides through intermediate points, and
	// releases at (toX, toY).
	Drag(fromX, fromY, toX, toY float64) error
	// DragAndDrop resolves fromSel/toSel to their centers, then drags.
	DragAndDrop(fromSel, toSel string) error
	// Tap dispatches a touch tap at (x, y).
	Tap(x, y float64) error
	// FocusElement scrolls the element into view, clicks it, and verifies (or
	// falls back to a JS .focus()) that it took keyboard focus.
	FocusElement(selector string) error
}
