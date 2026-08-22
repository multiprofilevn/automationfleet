//go:build windows

package automationfleet

// Phase-5 tier-1 stress: scenarios 9-11 — the red-team findings around the
// foreground lease and the pinned coordinate frame. Fake driver, CI-safe.

import (
	"context"
	"fmt"
	"sync/atomic"
	"testing"
	"time"

	"github.com/multiprofilevn/automationfleet/internal/faketest"
)

// 9. Pins red-team finding 1 (a lease that wrongly blocks the remote path).
// The native job sits AHEAD of the 200 remote jobs — submitted first AND at
// the highest priority, so it is unambiguously the heap head. A head-blocking
// implementation passes only if the pop-filter skips it; without that this
// test would be vacuous.
func TestStress_LeaseHeld_RemoteStillFlows(t *testing.T) {
	f := newTestFleet(t, WithRemoteWorkers(8))
	nd := addFake(f, "n1", true)
	for i := 0; i < 4; i++ {
		addFake(f, fmt.Sprintf("r%d", i), false)
	}
	f.Start()

	lease := mustAcquire(t, f, context.Background(), "open-browser")
	natCh := mustSubmit(t, f, Job{BrowserID: "n1", Priority: 100, Action: ClickAction{Selector: "#head"}})
	remCh := make([]<-chan JobResult, 200)
	for i := range remCh {
		remCh[i] = mustSubmit(t, f, Job{BrowserID: fmt.Sprintf("r%d", i%4), Priority: 0,
			Action: ScrollAction{DeltaY: 1}})
	}
	for i, ch := range remCh {
		if r := oneResult(t, ch, 10*time.Second, fmt.Sprintf("remote job %d behind blocked native head", i)); r.Status != StatusDone {
			t.Fatalf("remote job %d: status=%s err=%v — lease starves the remote path", i, r.Status, r.Err)
		}
	}
	if got := nd.Count("focus"); got != 0 {
		t.Fatalf("native head job ran while the lease was held (focus=%d)", got)
	}
	lease.Release()
	if r := oneResult(t, natCh, 5*time.Second, "native head after release"); r.Status != StatusDone {
		t.Fatalf("native head job after release: status=%s err=%v", r.Status, r.Err)
	}
	if got := nd.Count("clickat"); got != 1 {
		t.Fatalf("native clickat = %d, want 1", got)
	}
}

// 10. Stop while a lease is held, with native jobs queued BEHIND the lease.
// Stop must return promptly (not after the 60s lease timeout), the blocked
// native jobs must come back cancelled instead of stranded, and Release after
// stop must stay a safe no-op. goleak-clean.
func TestStress_StopWhileLeaseHeld(t *testing.T) {
	snap := leakSnapshot()
	f := newTestFleet(t)
	addFake(f, "n1", true)
	addFake(f, "r1", false)
	f.Start()

	lease := mustAcquire(t, f, context.Background(), "holder")
	natCh := make([]<-chan JobResult, 5)
	for i := range natCh {
		natCh[i] = mustSubmit(t, f, Job{BrowserID: "n1", Action: ClickAction{Selector: "#blocked"}})
	}
	remCh := make([]<-chan JobResult, 5)
	for i := range remCh {
		remCh[i] = mustSubmit(t, f, remoteJob("r1"))
	}
	for i, ch := range remCh {
		if r := oneResult(t, ch, 5*time.Second, fmt.Sprintf("remote %d", i)); r.Status != StatusDone {
			t.Fatalf("remote %d under held lease: status=%s err=%v", i, r.Status, r.Err)
		}
	}

	stopped := make(chan time.Duration, 1)
	go func() {
		start := time.Now()
		f.Stop()
		stopped <- time.Since(start)
	}()
	select {
	case took := <-stopped:
		if took > 3*time.Second {
			t.Fatalf("Stop took %s while a lease was held — waiting out the lease instead of aborting", took)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("Stop wedged while a lease was held")
	}
	for i, ch := range natCh {
		if r := oneResult(t, ch, 5*time.Second, fmt.Sprintf("lease-blocked native %d", i)); r.Status != StatusCancelled {
			t.Fatalf("lease-blocked native %d: status=%s err=%v, want cancelled", i, r.Status, r.Err)
		}
	}
	lease.Release() // after stop: must not panic, must not wedge
	if _, err := f.AcquireForeground(context.Background(), "post-stop"); err == nil {
		t.Fatal("AcquireForeground succeeded on a stopped fleet")
	}
	waitWG(t, f, 5*time.Second, "workers after stop-while-leased")
	verifyNoNewGoroutines(t, snap)
}

// 11. The window "moves" exactly during the cursor glide (MoveTo) of a
// multi-step click. The post-move rect re-check must abort THAT attempt with
// errWindowMoved before any click fires, the retry must re-pin the frame, and
// the click must fire exactly once — on the fresh frame, never torn across
// two frames. Ten inner iterations × -count=10 = the spec's 100 runs.
//
// Tier-1 limit, stated plainly: the fake's ClickAt is atomic, so a click torn
// between button-down and button-up is structurally invisible here. "Exactly
// one clickat, only after the second pin" is the strongest tearing invariant
// the fake can express; the real down/move/up tear belongs to tier 2.
func TestStress_InvalidateDuringMultiStepOp(t *testing.T) {
	f := newTestFleet(t, WithDriftRetries(2), WithDriftRetryDelay(time.Millisecond))
	d := addFake(f, "n1", true)
	d.SetHWND(1)
	base := faketest.Rect{Left: 0, Top: 0, Right: 800, Bottom: 600}
	moved := faketest.Rect{Left: 300, Top: 40, Right: 1100, Bottom: 640}
	f.Start()

	for iter := 0; iter < 10; iter++ {
		d.SetWindowRect(base, nil)
		var tripped atomic.Bool
		d.SetOnCall(func(op string) {
			if op == "moveto" && tripped.CompareAndSwap(false, true) {
				d.SetWindowRect(moved, nil)
			}
		})
		pinsBefore := d.Count("invalidatenativewindow")
		movesBefore := d.Count("moveto")
		clicksBefore := d.Count("clickat")

		r := oneResult(t, mustSubmit(t, f, Job{BrowserID: "n1", Action: ClickAction{Selector: "#t"}}),
			5*time.Second, fmt.Sprintf("iter %d click", iter))
		if r.Status != StatusDone {
			t.Fatalf("iter %d: status=%s err=%v — retry after errWindowMoved did not recover", iter, r.Status, r.Err)
		}
		if got := d.Count("clickat") - clicksBefore; got != 1 {
			t.Fatalf("iter %d: clickat delta = %d, want exactly 1 — click fired on a stale frame or fired twice", iter, got)
		}
		if got := d.Count("invalidatenativewindow") - pinsBefore; got != 2 {
			t.Fatalf("iter %d: frame pins = %d, want 2 (initial + re-measure after errWindowMoved)", iter, got)
		}
		if got := d.Count("moveto") - movesBefore; got != 2 {
			t.Fatalf("iter %d: moveto = %d, want 2 (aborted attempt + retried attempt)", iter, got)
		}
	}
	// Transcript-order check over the whole run: every clickat must be
	// preceded more recently by a frame pin than by a moveto that saw the rect
	// change — i.e. the aborted attempt never clicked.
	events := d.Events()
	lastPin, lastClick := -1, -1
	for i, e := range events {
		switch e.Op {
		case "invalidatenativewindow":
			lastPin = i
		case "clickat":
			if lastPin == -1 || lastPin < lastClick {
				t.Fatalf("clickat at event %d without a fresh frame pin since the previous click", i)
			}
			lastClick = i
		}
	}
}
