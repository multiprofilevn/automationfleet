package automationfleet

// Phase 3, R3 — runtime worker pool (Test Spec 18-22).
//
// Concurrency is measured with the fake's op gate, never with a sleep: a sleep
// shows that time passed, not that eight jobs were inside the driver at once.

import (
	"errors"
	"fmt"
	"runtime"
	"testing"
	"time"
)

// 18. Growing the pool actually raises parallelism. The gate holds every job
// inside the driver, so PeakHeld is the exact number of workers that ran at the
// same moment — no timing guess involved.
func TestSetRemoteWorkers_GrowsPool(t *testing.T) {
	f := newTestFleet(t, WithRemoteWorkers(4))
	d := addFake(f, "r1", false)
	f.Start()

	if err := f.SetRemoteWorkers(12); err != nil {
		t.Fatalf("SetRemoteWorkers(12): %v", err)
	}
	if got := f.Snapshot().RemoteWorkers; got != 12 {
		t.Errorf("Snapshot().RemoteWorkers = %d, want 12", got)
	}

	d.HoldOp("evaluate")
	// Always release: a failed assertion below must not leave workers parked
	// inside the driver, or the fleet Stop in newTestFleet's cleanup never returns.
	t.Cleanup(d.ReleaseOps)
	for i := 0; i < 12; i++ {
		if _, err := f.Submit(remoteJob("r1")); err != nil {
			t.Fatalf("submit %d: %v", i, err)
		}
	}
	waitFor(t, 3*time.Second, "12 jobs to run concurrently after growing the pool", func() bool {
		return d.Held() == 12
	})
	d.ReleaseOps()

	if peak := d.PeakHeld(); peak != 12 {
		t.Errorf("peak concurrency = %d, want 12", peak)
	}
}

// 19. Shrinking must not cancel work. Eight jobs are held inside the driver;
// the pool drops to two while they are there. Every one of them still finishes,
// and only then does the pool actually reach two.
func TestSetRemoteWorkers_ShrinksAfterInflight(t *testing.T) {
	f := newTestFleet(t, WithRemoteWorkers(8))
	d := addFake(f, "r1", false)
	f.Start()

	d.HoldOp("evaluate")
	// Always release: a failed assertion below must not leave workers parked
	// inside the driver, or the fleet Stop in newTestFleet's cleanup never returns.
	t.Cleanup(d.ReleaseOps)
	results := make([]<-chan JobResult, 8)
	for i := range results {
		ch, err := f.Submit(remoteJob("r1"))
		if err != nil {
			t.Fatalf("submit %d: %v", i, err)
		}
		results[i] = ch
	}
	waitFor(t, 3*time.Second, "8 jobs in flight", func() bool { return d.Held() == 8 })

	if err := f.SetRemoteWorkers(2); err != nil {
		t.Fatalf("SetRemoteWorkers(2): %v", err)
	}
	d.ReleaseOps()

	for i, ch := range results {
		select {
		case r := <-ch:
			if r.Status != StatusDone {
				t.Errorf("job %d: status %v (err %v) — shrinking the pool cancelled work that was already running",
					i, r.Status, r.Err)
			}
		case <-time.After(5 * time.Second):
			t.Fatalf("job %d never completed after the pool shrank", i)
		}
	}

	waitFor(t, 3*time.Second, "surplus workers to exit after their job", func() bool {
		return f.Snapshot().RemoteWorkers == 2
	})
}

// 20. 100 resizes must not leak goroutines. runtime.NumGoroutine is polled
// rather than sampled once — worker exit is asynchronous by design.
func TestSetRemoteWorkers_NoGoroutineLeak(t *testing.T) {
	f := newTestFleet(t, WithRemoteWorkers(4))
	addFake(f, "r1", false)
	f.Start()

	waitFor(t, 2*time.Second, "initial pool to settle", func() bool {
		return f.Snapshot().RemoteWorkers == 4
	})
	base := runtime.NumGoroutine()

	for i := 0; i < 100; i++ {
		n := 4
		if i%2 == 1 {
			n = 16
		}
		if err := f.SetRemoteWorkers(n); err != nil {
			t.Fatalf("resize %d → %d: %v", i, n, err)
		}
	}
	if err := f.SetRemoteWorkers(4); err != nil {
		t.Fatalf("final resize: %v", err)
	}

	waitFor(t, 5*time.Second, fmt.Sprintf("goroutines to return to ~%d after 100 resizes", base), func() bool {
		return runtime.NumGoroutine() <= base+2
	})
}

// 21. n <= 0 is an error and changes nothing.
func TestSetRemoteWorkers_InvalidN(t *testing.T) {
	f := newTestFleet(t, WithRemoteWorkers(5))
	addFake(f, "r1", false)
	f.Start()

	before := f.Snapshot().RemoteWorkers
	for _, n := range []int{0, -1, -100} {
		if err := f.SetRemoteWorkers(n); err == nil {
			t.Errorf("SetRemoteWorkers(%d) returned nil, want an error", n)
		}
	}
	if after := f.Snapshot().RemoteWorkers; after != before {
		t.Errorf("pool changed on invalid input: %d → %d", before, after)
	}
}

// 22. After Stop the pool is frozen: no error swallowed, no goroutine spawned
// into a fleet that is already tearing down.
func TestSetRemoteWorkers_AfterStop(t *testing.T) {
	f := newTestFleet(t, WithRemoteWorkers(4))
	addFake(f, "r1", false)
	f.Start()
	f.Stop()
	waitWG(t, f, 3*time.Second, "Stop did not drain the pool")

	base := runtime.NumGoroutine()
	if err := f.SetRemoteWorkers(16); !errors.Is(err, ErrFleetStopped) {
		t.Fatalf("SetRemoteWorkers after Stop: want ErrFleetStopped, got %v", err)
	}
	// A spawn would show up immediately; poll briefly to catch a late one too.
	stableFor(t, 200*time.Millisecond, "SetRemoteWorkers spawned workers after Stop", func() bool {
		return runtime.NumGoroutine() <= base+2
	})
}
