//go:build integration && windows

package automationfleet

// Tier-2 items 15 and 17 (mouse-side guards) — split from
// stress_fleet_real_test.go (200-line rule). Same warning and gates apply:
// real input, FLEET_ALLOW_REAL_INPUT=1 required.

import (
	"errors"
	"fmt"
	"testing"
	"time"
)

// 15. The cursor is yanked away mid critical section. SetCursorPos arrives
// with LLMHF_INJECTED, so the takeover watchdog may legitimately NOT fire —
// the defense under test is the drift guard: no click may land anywhere but
// the target, and every job ends Done (after retry) or fails loudly.
func TestStressReal_UserGrabsMouse(t *testing.T) {
	requireRealInput(t)
	useRealOSSeams(t)
	var pauses int
	f := newTestFleet(t, OnPause(func(string) { pauses++ }))
	launchChromeNative(t, f, "A", 100, 100)
	pageA := navigateHandle(t, f, "A", clickRecorderPage)
	f.Start()

	stop := make(chan struct{})
	go func() { // the "user": jerk the cursor to a corner every 40ms
		for {
			select {
			case <-stop:
				return
			case <-time.After(40 * time.Millisecond):
				setCursorTo(10, 10)
			}
		}
	}()
	done, failed := 0, 0
	for i := 0; i < 10; i++ {
		r := resultWithin(t, mustSubmit(t, f, Job{BrowserID: "A", Timeout: 30 * time.Second,
			Action: ClickAction{Selector: "#target"}}), 40*time.Second, "grabbed-mouse click")
		switch r.Status {
		case StatusDone:
			done++
		case StatusFailed:
			failed++ // loud failure (drift retries exhausted) is acceptable
		default:
			t.Fatalf("click %d: status=%s err=%v", i, r.Status, r.Err)
		}
	}
	close(stop)
	v, err := pageA.ReadValue("#count")
	if err != nil {
		t.Fatalf("readValue count: %v", err)
	}
	if v != fmt.Sprintf("%d", done) {
		t.Fatalf("page recorded %s clicks but %d jobs reported Done — clicks landed somewhere unaccounted", v, done)
	}
	t.Logf("grabbed-mouse run: done=%d failed-loud=%d watchdog-pauses=%d", done, failed, pauses)
}

// 17. Pin of red-team finding 2: browser B sits TOPMOST exactly over A's
// target coordinates. WindowFromPoint sees B, the click guard must refuse,
// and B's page must record zero clicks.
func TestStressReal_ClickRejectedWhenWindowOverlaps(t *testing.T) {
	requireRealInput(t)
	useRealOSSeams(t)
	f := newTestFleet(t)
	launchChromeNative(t, f, "A", 100, 100)
	navigateHandle(t, f, "A", clickRecorderPage)
	launchChromeNative(t, f, "B", 900, 100)
	pageB := navigateHandle(t, f, "B", clickRecorderPage)
	f.Start()

	rectA, err := f.handle("A").Driver.WindowRect()
	if err != nil {
		t.Fatalf("rect A: %v", err)
	}
	hwndB, err := f.handle("B").Driver.HWND()
	if err != nil || hwndB == 0 {
		t.Fatalf("HWND B: %v", err)
	}
	overlayWindowOn(t, hwndB, rectA) // B topmost, exactly over A

	r := resultWithin(t, mustSubmit(t, f, Job{BrowserID: "A", Timeout: 30 * time.Second,
		Action: ClickAction{Selector: "#target"}}), 40*time.Second, "click into overlapped window")
	if r.Status != StatusFailed || !errors.Is(r.Err, ErrForegroundLost) {
		t.Fatalf("click on overlapped A: status=%s err=%v, want failed(ErrForegroundLost)", r.Status, r.Err)
	}
	if v, err := pageB.ReadValue("#count"); err != nil || v != "0" {
		t.Fatalf("browser B recorded %q clicks (err %v), want 0 — the click fell through into B", v, err)
	}
}
