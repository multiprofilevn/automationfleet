//go:build windows

package automationfleet

import (
	"context"
	"errors"
	"testing"

	"github.com/tuwibu/automationfleet/internal/faketest"
)

// clickAt is the ONE place in the system where two coordinate frames meet: the
// kit computes where to put the cursor, and the drift guard independently
// computes where the cursor should have ended up. Every one of the false-drift
// bugs is the same shape — the two sides used different numbers, the click was
// correct, and the guard called it drift, burned three retries and failed the
// job.
//
// The fix is to pin ONE frame at the top of the critical section and hand the
// same numbers to both sides. These specs pin the pieces of that:
//   - the scale comes from the kit's live measurement, not from the handle
//   - the content offset is absolute, so no window origin is added twice
//   - a window that moves mid-operation is its own error, not "drift"
//   - and real drift is still caught, so none of the above can be "fixed" by
//     quietly weakening the guard

// coordFixture wires a fleet + native handle + fake driver. handleX/Y are the
// numbers a runner declared BEFORE launch; the driver's offset/scale/rect are
// what the window actually turned out to be. Those disagreeing is the normal
// case, not the exotic one.
//
// handleScale is accepted and ignored on purpose: phase 3 removed the declared
// scale from BrowserHandle entirely (the kit measures it live), and the call
// sites still read better naming the number the runner used to assume.
type coordFixture struct {
	f    *Fleet
	h    *BrowserHandle
	page Page
	d    *faketest.Driver
}

func newCoordFixture(t *testing.T, handleX, handleY int, _ float64) coordFixture {
	t.Helper()
	f := New(WithPauseHotkeyDisabled(), WithResumeHotkeyDisabled())
	d := faketest.New()
	// A window that is nowhere near moving, unless a spec says otherwise.
	d.SetWindowRect(faketest.Rect{Left: 0, Top: 0, Right: 1920, Bottom: 1080}, nil)
	drv := fakeDriver{d}
	h := &BrowserHandle{ID: "n1", Driver: drv, Native: true}
	return coordFixture{f: f, h: h, page: drv.Current(), d: d}
}

// pinCursorAt makes the OS cursor seam report a fixed position for this test.
func pinCursorAt(t *testing.T, x, y int) {
	t.Helper()
	prev := cursorPos
	t.Cleanup(func() { cursorPos = prev })
	cursorPos = func() (int, int, error) { return x, y, nil }
}

// 13. TestClickAt_NoFalseDriftAtScale125 (B2).
//
// The kit measures the window at 1.25 (a 120-DPI monitor). The handle carries
// the 1.0 the runner assumed at launch. The kit puts the cursor at the correct
// screen pixel; the guard, computing with 1.0, expects it 100px short and calls
// a perfect click drift.
func TestClickAt_NoFalseDriftAtScale125(t *testing.T) {
	fx := newCoordFixture(t, 0, 0, 1.0) // handle says 1.0 …
	fx.d.SetWindowScale(1.25, nil)      // … the window is really 1.25
	fx.d.SetContentOffset(1000, 500, nil)

	// Where the kit actually leaves the cursor: origin + css * measured scale.
	pinCursorAt(t, 1000+500, 500+750)

	if err := clickAt(context.Background(), fx.f, fx.h, fx.page, 400, 600); err != nil {
		t.Fatalf("clickAt reported %v for a correctly placed click — the guard is computing with the handle's stale "+
			"scale instead of the scale the kit measured", err)
	}
	if got := fx.d.Count("clickat"); got != 1 {
		t.Fatalf("clickat = %d, want 1", got)
	}
}

// 9. TestWindowScale_KitAndGuardAgree pins the SOURCE, where spec 13 pins the
// arithmetic. The guard must ask the driver for the scale; reading it off the
// handle happens to give the same answer whenever the runner guessed right,
// which is exactly why a spec that only checks the numbers is not enough.
func TestWindowScale_KitAndGuardAgree(t *testing.T) {
	fx := newCoordFixture(t, 0, 0, 1.0)
	fx.d.SetWindowScale(1.5, nil)
	fx.d.SetContentOffset(200, 100, nil)
	pinCursorAt(t, 200+600, 100+900) // css(400,600) at scale 1.5

	err := clickAt(context.Background(), fx.f, fx.h, fx.page, 400, 600)

	if got := fx.d.Count("windowscale"); got == 0 {
		t.Fatal("clickAt never called Driver.WindowScale — the guard is still reading BrowserHandle.Scale, so it will " +
			"disagree with the kit the moment a window lands on a differently-scaled monitor")
	}
	if err != nil {
		t.Fatalf("clickAt reported %v using the measured scale; want success", err)
	}
}

// 14. TestClickAt_NoFalseDriftWhenWindowRectDiffersFromRequested (B3).
//
// The runner asked for x=1920; the shell snapped the window to 1913. The kit
// measures the window where it IS and reports an absolute content origin. The
// guard then adds the requested 1920 on top of an origin that already contains
// 1913 — the expected position lands roughly a whole screen away.
func TestClickAt_NoFalseDriftWhenWindowRectDiffersFromRequested(t *testing.T) {
	fx := newCoordFixture(t, 1920, 0, 1.0) // requested position
	fx.d.SetWindowScale(1.0, nil)
	fx.d.SetContentOffset(1921, 87, nil) // measured: window really at 1913, +8 border
	pinCursorAt(t, 1921+400, 87+600)

	if err := clickAt(context.Background(), fx.f, fx.h, fx.page, 400, 600); err != nil {
		t.Fatalf("clickAt reported %v — BrowserHandle.X is being added to an offset that is already absolute", err)
	}
	if got := fx.d.Count("clickat"); got != 1 {
		t.Fatalf("clickat = %d, want 1", got)
	}
}

// 15. TestClickAt_FirefoxAbsoluteOffsetNotDoubleCounted (B1).
//
// firefoxkit has ALWAYS returned an absolute content origin, so any handle with
// a non-zero X/Y double-counts it today. This is the same defect as spec 14 seen
// from the other kit, and it is why the two ContentOffset methods must end up
// with one meaning rather than two.
func TestClickAt_FirefoxAbsoluteOffsetNotDoubleCounted(t *testing.T) {
	fx := newCoordFixture(t, 100, 200, 1.0)
	fx.d.SetWindowScale(1.0, nil)
	fx.d.SetContentOffset(108, 287, nil) // window at (100,200) + firefox chrome
	pinCursorAt(t, 108+50, 287+60)

	if err := clickAt(context.Background(), fx.f, fx.h, fx.page, 50, 60); err != nil {
		t.Fatalf("clickAt reported %v — firefox's absolute offset was added to the handle origin a second time", err)
	}
	if got := fx.d.Count("clickat"); got != 1 {
		t.Fatalf("clickat = %d, want 1", got)
	}
}

// 16. TestClickAt_WindowMovesDuringMoveTo.
//
// The cursor is exactly where it was told to go — the frame was pinned before
// the glide, so nothing drifted. What changed is the target: the window moved
// out from under the coordinates. Reporting that as errCursorDrift sends the
// retry loop straight back into the same stale frame; it needs its own error so
// the recovery path re-measures first.
//
// No click may be emitted either way: the pixel is now inside whatever the
// window uncovered.
func TestClickAt_WindowMovesDuringMoveTo(t *testing.T) {
	fx := newCoordFixture(t, 0, 0, 1.0)
	fx.d.SetWindowScale(1.0, nil)
	fx.d.SetContentOffset(100, 200, nil)
	fx.d.SetWindowRect(faketest.Rect{Left: 0, Top: 0, Right: 800, Bottom: 600}, nil)

	// The window jumps 200px right in the middle of the glide.
	fx.d.SetOnCall(func(op string) {
		if op == "moveto" {
			fx.d.SetWindowRect(faketest.Rect{Left: 200, Top: 0, Right: 1000, Bottom: 600}, nil)
		}
	})

	// The cursor arrived precisely where the pinned frame aimed it.
	pinCursorAt(t, 100+400, 200+600)

	err := clickAt(context.Background(), fx.f, fx.h, fx.page, 400, 600)

	if !errors.Is(err, errWindowMoved) {
		t.Fatalf("err = %v, want errWindowMoved — a window that moved mid-glide is not the same failure as a human "+
			"grabbing the mouse, and the two need different recovery", err)
	}
	if errors.Is(err, errCursorDrift) {
		t.Fatal("a moved window was reported as cursor drift; the retry path would re-run against the same stale frame")
	}
	if got := fx.d.Count("clickat"); got != 0 {
		t.Fatalf("clickat = %d, want 0 — the click would have landed on whatever the window uncovered", got)
	}
}

// 17. TestClickAt_RealDriftStillDetected.
//
// This spec exists because of the specs above it. Every one of them is satisfied
// by deleting the drift guard, and that is the single worst way to make them
// pass: a human bumping the mouse mid-job would then get a real click at a real
// screen pixel nobody chose. 40px is not rounding.
func TestClickAt_RealDriftStillDetected(t *testing.T) {
	fx := newCoordFixture(t, 0, 0, 1.0)
	fx.d.SetWindowScale(1.0, nil)
	fx.d.SetContentOffset(1000, 500, nil)

	// Correct target is (1400, 1100); something dragged the cursor 40px right.
	pinCursorAt(t, 1440, 1100)

	err := clickAt(context.Background(), fx.f, fx.h, fx.page, 400, 600)

	if !errors.Is(err, errCursorDrift) {
		t.Fatalf("err = %v, want errCursorDrift — a genuine 40px displacement must still be caught, whatever the "+
			"coordinate-frame work does to the false-positive cases", err)
	}
	if got := fx.d.Count("clickat"); got != 0 {
		t.Fatalf("clickat = %d, want 0 — a click was emitted 40px from the target", got)
	}
}

// 10. TestContentOffset_RejectsSpoofedWindowMetrics (fleet half).
//
// chromekit's ContentOffset fails when the page-reported origin cannot be
// reconciled with the window rect. The fleet must let that failure stop the job.
// Today the error is dropped on the floor with `if ..., err == nil` and the
// click goes out anyway using an unadjusted frame — which is the exact outcome
// the kit-side check exists to prevent.
func TestContentOffset_RejectsSpoofedWindowMetrics(t *testing.T) {
	fx := newCoordFixture(t, 0, 0, 1.0)
	fx.d.SetWindowScale(1.0, nil)
	spoof := errors.New("contentOffset: content origin outside window rect")
	fx.d.SetContentOffset(0, 0, spoof)
	// Deliberately the position today's guard computes when it drops the offset
	// error: without this the spec would pass on a drift complaint and prove
	// nothing about whether the untrusted frame was rejected.
	pinCursorAt(t, 400, 600)

	err := clickAt(context.Background(), fx.f, fx.h, fx.page, 400, 600)

	if err == nil {
		t.Fatal("clickAt succeeded although the coordinate frame could not be trusted")
	}
	if got := fx.d.Count("clickat"); got != 0 {
		t.Fatalf("clickat = %d, want 0 — a click was emitted with an untrusted coordinate frame", got)
	}
}
