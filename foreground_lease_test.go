package automationfleet

import (
	"context"
	"errors"
	"os"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/multiprofilevn/automationfleet/internal/faketest"
)

// TestMain pins the OS seams for the whole package so the native critical
// section is hermetic: no test depends on where the real mouse happens to be,
// which window is really in the foreground, or whether a human nudges either
// mid-run. Individual tests may override a seam *before* starting their fleet
// (see TestAcquireForeground_WaitsAcrossDriftRetries).
func TestMain(m *testing.M) {
	cursorPos = func() (int, int, error) { return 0, 0, nil }
	windowFromPoint = func(int, int) (uintptr, error) { return 0, nil }
	rootWindow = func(h uintptr) uintptr { return h }
	// Registering a native handle reconciles the takeover watchdog, which
	// installs a real WH_MOUSE_LL hook on Windows. Pinned here so the cap and
	// register specs can use the real Register path without the human's mouse
	// pausing the fleet mid-assertion.
	installMouseHook = func(func()) (func(), error) { return func() {}, nil }
	os.Exit(m.Run())
}

// The faketest → fleet interface adapter (fakeDriver/fakePage/fakeMouse/
// fakeKeyboard) lives in fake_adapters_test.go.

// ------------------------------------------------------------- helpers ----

// newTestFleet builds a fleet with the hotkey listeners off (they register real
// global hotkeys) and shuts it down deterministically at test end. Stop fully
// drains workers and in-flight jobs before returning.
func newTestFleet(t *testing.T, opts ...Option) *Fleet {
	t.Helper()
	base := []Option{WithPauseHotkeyDisabled(), WithResumeHotkeyDisabled()}
	f := New(append(base, opts...)...)
	t.Cleanup(f.Stop)
	return f
}

// addFake registers a fake browser by writing the registry directly. Register()
// would call reconcileTakeover, which installs a REAL low-level mouse hook on
// Windows — the watchdog would then pause the fleet whenever the human moves
// the mouse, destroying determinism.
func addFake(f *Fleet, id string, native bool) *faketest.Driver {
	d := faketest.New()
	d.SetNative(native)
	h := &BrowserHandle{ID: id, Driver: fakeDriver{d}, Native: native}
	f.mu.Lock()
	f.handles[id] = h
	f.mu.Unlock()
	return d
}

// handleOf returns the registered handle (tests that need to set HWND).
func handleOf(f *Fleet, id string) *BrowserHandle {
	f.mu.RLock()
	defer f.mu.RUnlock()
	return f.handles[id]
}

// stableFor fails the test if cond ever stops holding within d. Complement of
// waitFor (takeover_watchdog_test.go): proves something did NOT happen without
// resting the verdict on a single fixed sleep.
func stableFor(t *testing.T, d time.Duration, msg string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(d)
	for time.Now().Before(deadline) {
		if !cond() {
			t.Fatalf("condition broke early: %s", msg)
		}
		time.Sleep(time.Millisecond)
	}
}

// resultWithin reads one JobResult or fails.
func resultWithin(t *testing.T, ch <-chan JobResult, d time.Duration, what string) JobResult {
	t.Helper()
	select {
	case r := <-ch:
		return r
	case <-time.After(d):
		t.Fatalf("timeout (%s) waiting for %s", d, what)
		return JobResult{}
	}
}

func mustSubmit(t *testing.T, f *Fleet, j Job) <-chan JobResult {
	t.Helper()
	ch, err := f.Submit(j)
	if err != nil {
		t.Fatalf("Submit(%s): %v", j.Action.kind(), err)
	}
	return ch
}

func mustAcquire(t *testing.T, f *Fleet, ctx context.Context, reason string) *ForegroundLease {
	t.Helper()
	l, err := f.AcquireForeground(ctx, reason)
	if err != nil {
		t.Fatalf("AcquireForeground(%s): %v", reason, err)
	}
	if l == nil {
		t.Fatalf("AcquireForeground(%s): nil lease with nil error", reason)
	}
	return l
}

// ---------------------------------------------------------------- specs ----

// 1. AcquireForeground must not return while a native job is mid-flight —
// handing the foreground to a consumer at that moment is exactly the bug this
// phase closes (browser B steals focus while browser A is being typed into).
func TestAcquireForeground_WaitsForInflightNativeJob(t *testing.T) {
	f := newTestFleet(t)
	d := addFake(f, "n1", true)
	// Focus alone burns 300ms, so the window between "job started" and "job
	// finished" is wide enough to observe without a racy fixed sleep.
	d.SetOpLatency("focus", 300*time.Millisecond)
	f.Start()

	res := mustSubmit(t, f, Job{BrowserID: "n1", Action: ClickAction{Selector: "#go"}})
	waitFor(t, 2*time.Second, "native job to start", func() bool { return d.Count("focus") == 1 })

	lease := mustAcquire(t, f, context.Background(), "open-browser")
	defer lease.Release()

	if got := d.Count("clickat"); got != 1 {
		t.Fatalf("lease acquired while the native job was still running: clickat=%d, want 1", got)
	}
	if r := resultWithin(t, res, 2*time.Second, "native job result"); r.Status != StatusDone {
		t.Fatalf("native job status = %s (err %v), want done", r.Status, r.Err)
	}
}

// 2. While a lease is held no NEW native job may start; releasing lets it run.
func TestAcquireForeground_BlocksNewNativeJobs(t *testing.T) {
	f := newTestFleet(t)
	d := addFake(f, "n1", true)
	f.Start()

	lease := mustAcquire(t, f, context.Background(), "open-browser")
	res := mustSubmit(t, f, Job{BrowserID: "n1", Action: ClickAction{Selector: "#go"}})

	stableFor(t, 200*time.Millisecond, "native job must not start while the lease is held",
		func() bool { return d.Count("focus") == 0 })

	lease.Release()
	if r := resultWithin(t, res, 2*time.Second, "native job after Release"); r.Status != StatusDone {
		t.Fatalf("native job status = %s (err %v), want done", r.Status, r.Err)
	}
}

// 3. Remote jobs keep flowing while the lease is held — and crucially they must
// do so with a NATIVE job standing AHEAD of them in the queue. A head-blocking
// predicate would deadlock the remote job behind the blocked native one; a test
// whose queue holds only remote jobs passes vacuously and proves nothing.
func TestAcquireForeground_RemoteJobsKeepFlowing(t *testing.T) {
	f := newTestFleet(t)
	native := addFake(f, "n1", true)
	remote := addFake(f, "r1", false)
	f.Start()

	lease := mustAcquire(t, f, context.Background(), "open-browser")

	// Higher priority + earlier insertion → the native job is unambiguously
	// at the head of the queue, in front of the remote one.
	nativeRes := mustSubmit(t, f, Job{BrowserID: "n1", Priority: 10, Action: ClickAction{Selector: "#native"}})
	remoteRes := mustSubmit(t, f, Job{BrowserID: "r1", Priority: 1, Action: ClickAction{Selector: "#remote"}})

	r := resultWithin(t, remoteRes, 2*time.Second, "remote job behind a blocked native job")
	if r.Status != StatusDone {
		t.Fatalf("remote job status = %s (err %v), want done", r.Status, r.Err)
	}
	if got := remote.Count("click"); got != 1 {
		t.Fatalf("remote click count = %d, want 1", got)
	}
	if got := native.Count("focus"); got != 0 {
		t.Fatalf("native job ran while the lease was held: focus=%d, want 0", got)
	}

	lease.Release()
	if r := resultWithin(t, nativeRes, 2*time.Second, "native job after Release"); r.Status != StatusDone {
		t.Fatalf("native job status = %s (err %v), want done", r.Status, r.Err)
	}
}

// 4. The lock is at executeCriticalWithRetry, not executeCritical: the
// driftRetryDelay gap between two attempts must not be a hole the lease slips
// through, or the consumer opens a browser exactly between retries.
func TestAcquireForeground_WaitsAcrossDriftRetries(t *testing.T) {
	prev := cursorPos
	t.Cleanup(func() { cursorPos = prev })

	var calls atomic.Int32
	// Attempt 1 reports a cursor far from the expected (0,0) → errCursorDrift.
	// Every later attempt reports the expected position → success.
	cursorPos = func() (int, int, error) {
		if calls.Add(1) == 1 {
			return 5000, 5000, nil
		}
		return 0, 0, nil
	}

	f := newTestFleet(t, WithDriftRetries(2), WithDriftRetryDelay(300*time.Millisecond))
	d := addFake(f, "n1", true)
	f.Start()

	res := mustSubmit(t, f, Job{BrowserID: "n1", Action: ClickAction{Selector: "#go"}})
	waitFor(t, 2*time.Second, "first attempt to start", func() bool { return d.Count("focus") == 1 })

	lease := mustAcquire(t, f, context.Background(), "open-browser")
	defer lease.Release()

	if got := d.Count("focus"); got < 2 {
		t.Fatalf("lease acquired in the gap between drift retries: focus attempts = %d, want >= 2", got)
	}
	if got := d.Count("clickat"); got != 1 {
		t.Fatalf("lease acquired before the retried job clicked: clickat = %d, want 1", got)
	}
	if r := resultWithin(t, res, 2*time.Second, "retried job result"); r.Status != StatusDone {
		t.Fatalf("retried job status = %s (err %v), want done", r.Status, r.Err)
	}
}

// 5. Ten concurrent acquirers serialize: never two leases held at once, nobody
// starved, no deadlock. Run under -race.
func TestAcquireForeground_Concurrent(t *testing.T) {
	f := newTestFleet(t)
	addFake(f, "n1", true)
	f.Start()

	const n = 10
	var held atomic.Int32
	var maxHeld atomic.Int32
	var completed atomic.Int32

	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			l, err := f.AcquireForeground(ctx, "concurrent")
			if err != nil {
				return
			}
			cur := held.Add(1)
			for {
				old := maxHeld.Load()
				if cur <= old || maxHeld.CompareAndSwap(old, cur) {
					break
				}
			}
			time.Sleep(2 * time.Millisecond)
			held.Add(-1)
			completed.Add(1)
			l.Release()
		}()
	}

	done := make(chan struct{})
	go func() { wg.Wait(); close(done) }()
	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatalf("deadlock: %d/%d acquirers completed", completed.Load(), n)
	}

	if got := maxHeld.Load(); got != 1 {
		t.Fatalf("leases overlapped: max concurrent = %d, want 1", got)
	}
	if got := completed.Load(); got != n {
		t.Fatalf("starvation: %d/%d acquirers completed", got, n)
	}
}

// 6. Release is idempotent — a consumer with both a defer and an explicit call
// must not crash the process.
func TestForegroundLease_ReleaseIdempotent(t *testing.T) {
	f := newTestFleet(t)
	addFake(f, "n1", true)
	f.Start()

	lease := mustAcquire(t, f, context.Background(), "idempotent")
	lease.Release()
	lease.Release()
	lease.Release()

	// The foreground must genuinely be back, not merely "not panicking".
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	second, err := f.AcquireForeground(ctx, "after-triple-release")
	if err != nil {
		t.Fatalf("foreground not returned after repeated Release: %v", err)
	}
	second.Release()
}

// 7. Timeout escalates but does NOT release. Auto-release would drop the
// exclusivity invariant silently during a slow cold start — the one moment it
// is most needed.
func TestForegroundLease_TimeoutDoesNotRelease(t *testing.T) {
	var stuckCalls atomic.Int32
	var stuckReason atomic.Value
	f := newTestFleet(t,
		WithLeaseTimeout(60*time.Millisecond),
		OnLeaseStuck(func(reason string, _ time.Duration) {
			stuckReason.Store(reason)
			stuckCalls.Add(1)
		}),
	)
	addFake(f, "n1", true)
	f.Start()

	lease := mustAcquire(t, f, context.Background(), "slow-consumer")
	defer lease.Release()

	waitFor(t, 2*time.Second, "OnLeaseStuck to fire", func() bool { return stuckCalls.Load() >= 1 })
	if got, _ := stuckReason.Load().(string); got != "slow-consumer" {
		t.Fatalf("OnLeaseStuck reason = %q, want %q", got, "slow-consumer")
	}

	// Still held: a second acquire must not succeed.
	ctx, cancel := context.WithTimeout(context.Background(), 150*time.Millisecond)
	defer cancel()
	if second, err := f.AcquireForeground(ctx, "intruder"); err == nil {
		second.Release()
		t.Fatal("lease was auto-released on timeout; it must stay held")
	}
}

// 8. A waiting acquirer honours its context — Stop/AbortAll can cut it without
// waiting out the lease timeout.
func TestAcquireForeground_CancelledByContext(t *testing.T) {
	f := newTestFleet(t)
	addFake(f, "n1", true)
	f.Start()

	first := mustAcquire(t, f, context.Background(), "holder")
	defer first.Release()

	ctx, cancel := context.WithTimeout(context.Background(), 80*time.Millisecond)
	defer cancel()

	start := time.Now()
	second, err := f.AcquireForeground(ctx, "waiter")
	if err == nil {
		second.Release()
		t.Fatal("AcquireForeground returned a lease while another was held")
	}
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("err = %v, want context.DeadlineExceeded", err)
	}
	if second != nil {
		t.Fatal("AcquireForeground returned both an error and a non-nil lease")
	}
	if elapsed := time.Since(start); elapsed > time.Second {
		t.Fatalf("cancellation took %s — the acquirer is not selecting on ctx.Done()", elapsed)
	}
}

// 9. Stop while a lease is held must not block for the lease timeout, and it
// must release anyone blocked in AcquireForeground.
func TestAcquireForeground_StopWhileHeld(t *testing.T) {
	f := newTestFleet(t)
	addFake(f, "n1", true)
	f.Start()

	lease := mustAcquire(t, f, context.Background(), "holder")
	defer lease.Release()

	type acq struct {
		lease *ForegroundLease
		err   error
	}
	waiter := make(chan acq, 1)
	go func() {
		l, err := f.AcquireForeground(context.Background(), "waiter")
		waiter <- acq{l, err}
	}()

	// Give the waiter a moment to actually block before stopping.
	time.Sleep(50 * time.Millisecond)

	stopped := make(chan time.Duration, 1)
	go func() {
		start := time.Now()
		f.Stop()
		stopped <- time.Since(start)
	}()

	select {
	case took := <-stopped:
		if took > 2*time.Second {
			t.Fatalf("Stop blocked %s while a lease was held", took)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Stop did not return while a lease was held")
	}

	select {
	case got := <-waiter:
		if got.err == nil {
			got.lease.Release()
			t.Fatal("blocked AcquireForeground returned a lease after Stop; want an error")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("blocked AcquireForeground was not released by Stop")
	}
}
