// Package faketest provides an in-memory fake browser for automationfleet
// tests: a scriptable stand-in for a chromekit/firefoxkit driver with knobs for
// latency, injected errors and simulated foreground loss, plus a recorded
// transcript of every emitted key and click.
//
// It deliberately does NOT import automationfleet. The fleet's own tests are
// white-box (package automationfleet) and importing the package under test from
// a helper the test imports back would be an import cycle. Instead this package
// exposes plain methods and the test file wires them into the fleet's Driver /
// Page / Mouse / Keyboard interfaces with a one-line-per-method adapter.
//
// Split across driver.go (core state), driver_knobs.go (test-side setters),
// driver_transcript.go (recorded-call readback + injection plumbing),
// driver_core.go (Driver-interface mirrors), driver_page.go, driver_mouse.go,
// driver_keyboard.go (Page/Mouse/Keyboard-interface mirrors) — one file per
// fleet interface surface, mirroring how the fleet itself splits
// driver_mouse.go / driver_keyboard.go / driver_page.go.
package faketest

import (
	"sync"
	"time"
)

// Event is one recorded interaction. Op is the lowercase method name ("focus",
// "moveto", "clickat", "typehuman", "key", "clearinput", "readvalue", …).
type Event struct {
	Op   string
	Arg  string
	X, Y float64
	At   time.Time
}

// Driver is the fake. Zero value is not usable — call New.
//
// Every field is guarded by mu: the fleet drives it from the dispatcher's
// native worker goroutine while the test goroutine reads the transcript, so
// all access must be race-free under `go test -race`.
type Driver struct {
	mu sync.Mutex

	native bool
	noPage bool

	// hwnd models the kit-resolved window handle. hwndErr models the paths
	// where a kit cannot resolve one (firefox Connect, non-Windows).
	hwnd    uintptr
	hwndErr error

	// value models the focused input field. TypeHuman appends to it,
	// ClearInput empties it, ReadValue returns it.
	value string

	offsetX, offsetY int
	offsetErr        error

	// scale models Driver.WindowScale — the DPI factor the kit actually
	// converts CSS pixels with, as opposed to the one the runner declared on
	// the handle. The two differing is the whole point of the B2 specs.
	scale    float64
	scaleErr error

	// rect models Driver.WindowRect, the out-of-band window geometry. It is
	// mutable mid-test so a spec can move the window between two reads.
	rect    Rect
	rectErr error

	invalidateErr error

	centerX, centerY float64
	centerErr        error

	focusErr error
	moveErr  error
	clickErr error
	readErr  error
	navErr   error
	waitErr  error
	evalErr  error
	clearErr error

	// extraErrs backs every mirror added past the original small surface
	// (widened Page/Mouse/Keyboard — phase 8): one shared map instead of a
	// dedicated field per op, since no test yet needs per-op tuning beyond
	// "fail this call". SetErr still writes here for any op name its switch
	// doesn't recognize.
	extraErrs map[string]error

	// dialogHandler stores whatever OnDialog was last called with. Nothing
	// invokes it yet (no test simulates a JS dialog) — kept so a future spec
	// can call it without widening the struct again.
	dialogHandler func(DialogEvent) DialogAction

	latency time.Duration
	perOp   map[string]time.Duration

	// Simulated foreground loss: TypeHuman emits loseAt characters into the
	// field, then returns loseErr. Rearmed on every call when loseAlways.
	loseAt     int
	loseErr    error
	loseAlways bool
	loseArmed  bool

	events []Event
	counts map[string]int

	onCall func(op string)
	onKey  func(i int)

	// Op gate — see op_gate.go. holdOp names the op currently held, holdCh is
	// closed to release, held/peakHeld count the overlap.
	holdOp   string
	holdCh   chan struct{}
	held     int
	peakHeld int
}

// New returns a fake with sane defaults: native backend, a live page, an
// element centered at (0,0) and no latency.
func New() *Driver {
	return &Driver{
		native:    true,
		scale:     1.0,
		perOp:     make(map[string]time.Duration),
		counts:    make(map[string]int),
		extraErrs: make(map[string]error),
	}
}

// Rect mirrors the fleet's Rect without importing it (faketest must not depend
// on the package under test). The test file converts between the two.
type Rect struct {
	Left, Top, Right, Bottom int
}

// DialogType/DialogEvent/DialogAction mirror the fleet's own (which itself
// mirrors chromekit/firefoxkit's, field-for-field) — see driver_page.go's
// OnDialog. Duplicated rather than imported for the same reason Rect is: this
// package must not depend on automationfleet.
type DialogType string

// DialogEvent describes a simulated JS dialog. Unused today (no test drives
// one through the fake) — kept so OnDialog's signature is complete.
type DialogEvent struct {
	Type          DialogType
	Message       string
	DefaultPrompt string
}

// DialogAction mirrors the fleet's DialogAction.
type DialogAction struct {
	Accept     bool
	PromptText string
}
