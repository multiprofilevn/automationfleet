package automationfleet

import (
	"errors"
	"fmt"
	"runtime"

	"github.com/tuwibu/automationfleet/internal/winapi"
)

// ErrForegroundLost signals that the OS foreground window (keyboard target) or
// the window under the cursor (mouse target) stopped being the browser we were
// driving, mid-input. Unlike errCursorDrift it is NOT retried blindly: keys may
// already have escaped into a foreign window, so the recovery path is
// verify-then-repair, never "run the whole action again".
var ErrForegroundLost = errors.New("automationfleet: foreground lost mid-input")

// ForegroundLostError wraps ErrForegroundLost with the number of characters
// that may already have reached a foreign window. Callers handling credentials
// need that upper bound — see the phase-1 security notes.
type ForegroundLostError struct {
	// Op is the action kind that was interrupted ("type", "click", …).
	Op string
	// Emitted is the upper bound on characters that may have escaped.
	Emitted int
}

func (e *ForegroundLostError) Error() string {
	return fmt.Sprintf("automationfleet: foreground lost during %s, up to %d character(s) may have escaped", e.Op, e.Emitted)
}

// Unwrap makes errors.Is(err, ErrForegroundLost) hold for the rich form.
func (e *ForegroundLostError) Unwrap() error { return ErrForegroundLost }

// Seams over the OS. Production code MUST call these vars rather than the
// winapi/runtime functions directly — the tests swap them to make the native
// critical section hermetic (no real cursor, no real windows, no reliance on
// the machine's actual foreground state).
var (
	// cursorPos reports the OS cursor position (drift guard).
	cursorPos = winapi.GetCursorPos
	// windowFromPoint reports the HWND under a screen coordinate. The mouse
	// goes here, NOT to the foreground window — which is why a foreground
	// guard alone cannot protect a click.
	windowFromPoint = winapi.WindowFromPoint
	// rootWindow maps a child HWND to its top-level owner (GetAncestor
	// GA_ROOT). Chrome renders content in a child window, so the click guard
	// must accept descendants of the target HWND.
	rootWindow = winapi.RootWindow
	// lockOSThread / unlockOSThread pin the native critical section to one OS
	// thread. ForceENUSLayout is thread-scoped, so without this the layout can
	// be forced on one thread and the typing run on another.
	lockOSThread   = runtime.LockOSThread
	unlockOSThread = runtime.UnlockOSThread
)
