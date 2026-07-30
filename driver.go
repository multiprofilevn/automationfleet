package automationfleet

import (
	"context"
	"fmt"
)

// readValueExpr builds the JS both adapters evaluate for Page.ReadValue. It
// returns null (not "") when the element is missing, so a missing element is
// distinguishable from an empty field — verify-then-repair keys off that
// difference. contentEditable elements have no .value, hence the textContent
// fallback.
func readValueExpr(selector string) string {
	return fmt.Sprintf(
		`(()=>{const el=document.querySelector(%q); if(!el) return null; return el.value !== undefined && el.value !== null ? String(el.value) : String(el.textContent ?? "")})()`,
		selector,
	)
}

// InputMode is a fleet-internal enum identifying how a Driver dispatches input.
// It does NOT map 1:1 to any kit's integer values — chromekit uses
// BackendCDP=0/BackendNative=1, firefoxkit uses BackendBiDi=0/BackendNative=1.
// Adapters MUST translate via the named constants below; never cast a kit's
// backend int directly to InputMode.
type InputMode int

const (
	// InputRemote dispatches input over the wire — CDP (chrome) or BiDi
	// (firefox). Parallel-safe: it never touches the OS cursor or keyboard.
	InputRemote InputMode = iota
	// InputNative drives the OS pointer/keyboard directly (winapi on Windows).
	// Serialised through the single native worker and capped, because the
	// machine has one mouse and one keyboard.
	InputNative
)

// String renders the mode for logs and error messages.
func (m InputMode) String() string {
	switch m {
	case InputNative:
		return "native"
	case InputRemote:
		return "remote"
	default:
		return "unknown"
	}
}

// Backend is the former name of InputMode.
//
// Deprecated: use InputMode. Kept as an alias for at least one version so
// existing consumers keep compiling.
type Backend = InputMode

const (
	// BackendNative is the former name of InputNative.
	//
	// Deprecated: use InputNative.
	BackendNative = InputNative
	// BackendRemote is the former name of InputRemote.
	//
	// Deprecated: use InputRemote.
	BackendRemote = InputRemote
)

// Rect is a window's outer rectangle in absolute screen pixels, as reported by
// the window manager (Win32 GetWindowRect). It is deliberately NOT the kits'
// input.Rect: the fleet's Driver interface must stay free of any one kit's
// types, and faketest has to be able to satisfy it without importing a kit.
type Rect struct {
	Left, Top, Right, Bottom int
}

// BoundingBox is a viewport-relative box in CSS pixels, top-left origin.
type BoundingBox struct {
	X, Y, Width, Height float64
}

// Center returns the box center in CSS pixels.
func (b BoundingBox) Center() (float64, float64) {
	return b.X + b.Width/2, b.Y + b.Height/2
}

// Driver is a browser the fleet can drive, independent of the underlying kit
// (chromekit / firefoxkit). Adapters wrap a concrete kit Browser into this
// interface — see WrapChrome / WrapFirefox.
type Driver interface {
	// Current returns the active page, or nil when no page is live. Adapters
	// MUST return a nil interface (not a non-nil interface wrapping a nil
	// pointer) so callers' `page == nil` checks work.
	Current() Page
	Focus(ctx context.Context) error
	InputBackend() Backend
	ContentOffset() (int, int, error)
	// HWND returns the browser's top-level window handle (Windows), used by
	// the click guard to confirm the window under the cursor before ClickAt.
	// It comes from the Driver rather than from a caller-supplied field so
	// there is exactly one source of truth: the kit already resolves the
	// handle (SetHWND → PID → title) and re-validates it, and a stale copy
	// held by the fleet would silently point at a recycled window. Returning
	// an error (or 0) means "unknown" — the guard then skips rather than
	// failing the job.
	HWND() (uintptr, error)
	// WindowScale returns the DPI scale factor the kit is ACTUALLY mapping CSS
	// pixels through. The drift guard must read the scale from here rather than
	// from BrowserHandle.Scale: the handle carries a number the runner decided
	// before launch, the kit measures the live window, and the two disagree the
	// moment a window lands on a monitor with different scaling. Comparing a
	// position computed with one against a position produced with the other is
	// what makes a correct click look like drift.
	WindowScale() (float64, error)
	// InvalidateNativeWindow tells the kit to re-measure its coordinate frame
	// and push it into the live input backends. Call it after anything that can
	// move or rescale the window.
	InvalidateNativeWindow() error
	// WindowRect returns the window's outer rectangle in absolute screen pixels.
	// Out-of-band: it comes from the window manager, never from page script, so
	// it can bound coordinates that were measured inside the page and can show
	// that a window moved mid-operation.
	WindowRect() (Rect, error)
}
