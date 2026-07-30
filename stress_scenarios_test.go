//go:build windows

package automationfleet

// Phase-5 tier-1 stress: mandatory scenarios 4-6 — pool resize under load,
// register/unregister churn, AbortAll mid native type. Fake driver, CI-safe.

import (
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"
)

// 4. Resize the remote pool 4↔16 a hundred times while remote jobs are
// genuinely in flight (one wave of jobs submitted per flip). Every job must
// end StatusDone and the live pool must converge to the final target.
//
// Deviation from the spec's 50ms flip interval: 100×50ms = 5s per run, ×10
// under -count=10 blows the package budget. The interval does not change the
// interleaving being tested — a wave of in-flight jobs straddles every flip
// either way — so it is 2ms here; the 50ms cadence belongs to tier 2.
func TestStress_SetRemoteWorkersUnderLoad(t *testing.T) {
	snap := leakSnapshot()
	f := newTestFleet(t, WithRemoteWorkers(4))
	d := addFake(f, "r1", false)
	d.SetOpLatency("evaluate", time.Millisecond) // keep workers mid-job across flips
	f.Start()

	const flips, wave = 100, 4
	chans := make([]<-chan JobResult, 0, flips*wave)
	for i := 0; i < flips; i++ {
		for k := 0; k < wave; k++ {
			chans = append(chans, mustSubmit(t, f, remoteJob("r1")))
		}
		n := 4
		if i%2 == 0 {
			n = 16
		}
		if err := f.SetRemoteWorkers(n); err != nil {
			t.Fatalf("flip %d → %d: %v", i, n, err)
		}
		time.Sleep(2 * time.Millisecond)
	}
	waitDrained(t, f, 30*time.Second, "400 remote jobs across 100 resizes")
	for i, ch := range chans {
		if r := oneResult(t, ch, 5*time.Second, fmt.Sprintf("resize job %d", i)); r.Status != StatusDone {
			t.Fatalf("job %d: status=%s err=%v, want done", i, r.Status, r.Err)
		}
	}
	// Last flip is i=99 (odd) → target 4. remoteLive trails intent during a
	// shrink, so poll instead of asserting a single sample.
	waitFor(t, 5*time.Second, "pool settle to final target", func() bool {
		return f.Snapshot().RemoteWorkers == 4
	})
	stopAndDrain(t, f)
	verifyNoNewGoroutines(t, snap)
}

// 5. 500 register/unregister cycles racing in-flight jobs for those same ids,
// with a background feeder keeping the queue warm. No panic, exactly one
// result per job, and — the part a plain churn test misses — the native slot
// accounting must still be exact afterwards: 9 native registers fit, the 10th
// is rejected. A count corrupted by the churn fails there loudly.
func TestStress_RegisterUnregisterChurn(t *testing.T) {
	snap := leakSnapshot()
	f := newTestFleet(t, WithRemoteWorkers(4))
	addFake(f, "keep", false)
	f.Start()

	stop := make(chan struct{})
	var feederWG sync.WaitGroup
	feederWG.Add(1)
	go func() {
		defer feederWG.Done()
		for {
			select {
			case <-stop:
				return
			default:
			}
			ch, err := f.Submit(remoteJob("keep"))
			if err != nil {
				t.Errorf("feeder submit: %v", err)
				return
			}
			select {
			case r := <-ch:
				if r.Status != StatusDone {
					t.Errorf("feeder job: status=%s err=%v", r.Status, r.Err)
					return
				}
			case <-time.After(10 * time.Second):
				t.Error("feeder job result timeout")
				return
			}
		}
	}()

	for i := 0; i < 500; i++ {
		id := fmt.Sprintf("churn-%d", i%7)
		native := i%3 == 0
		if err := registerFake(f, id, native); err != nil {
			t.Fatalf("iter %d register: %v", i, err)
		}
		action := Action(ScrollAction{DeltaY: 1})
		if native {
			action = ClickAction{Selector: "#churn"} // native-critical path racing the unregister
		}
		ch := mustSubmit(t, f, Job{BrowserID: id, Action: action, Timeout: 5 * time.Second})
		if i%2 == 0 {
			// Unregister BEFORE the job has necessarily run: it must end Done
			// (ran first) or Failed(ErrUnknownBrowser) (popped after) — never
			// hang, never panic, never a third state.
			if err := f.Unregister(id); err != nil {
				t.Fatalf("iter %d unregister: %v", i, err)
			}
			r := oneResult(t, ch, 5*time.Second, fmt.Sprintf("churn job %d", i))
			if r.Status != StatusDone && !(r.Status == StatusFailed && errors.Is(r.Err, ErrUnknownBrowser)) {
				t.Fatalf("iter %d: status=%s err=%v, want done or failed(unknown browser)", i, r.Status, r.Err)
			}
		} else {
			if r := oneResult(t, ch, 5*time.Second, fmt.Sprintf("churn job %d", i)); r.Status != StatusDone {
				t.Fatalf("iter %d: status=%s err=%v, want done", i, r.Status, r.Err)
			}
			if err := f.Unregister(id); err != nil {
				t.Fatalf("iter %d unregister: %v", i, err)
			}
		}
	}
	close(stop)
	feederWG.Wait()

	if got := registeredCount(f); got != 1 {
		t.Fatalf("registry holds %d handles after churn, want 1 (only 'keep'): %v", got, registeredIDs(f))
	}
	for i := 0; i < 9; i++ {
		if err := registerFake(f, fmt.Sprintf("cap-%d", i), true); err != nil {
			t.Fatalf("native slot accounting corrupted by churn: register %d/9: %v", i+1, err)
		}
	}
	if err := registerFake(f, "cap-9", true); !errors.Is(err, ErrNativeCapacityExceeded) {
		t.Fatalf("10th native register after churn: err=%v, want ErrNativeCapacityExceeded", err)
	}
	stopAndDrain(t, f)
	verifyNoNewGoroutines(t, snap)
}

// 6. AbortAll fired from INSIDE the keystroke stream (onKey hook, char 3 of 6).
// The in-flight type must complete atomically — never torn mid-string — the
// pending jobs behind it must come back StatusCancelled, and the foreground
// token must be back in the channel once the worker finishes.
func TestStress_AbortAllMidNativeType(t *testing.T) {
	snap := leakSnapshot()
	f := newTestFleet(t)
	d := addFake(f, "n1", true)
	var abortOnce sync.Once
	d.SetOnKey(func(i int) {
		if i == 3 {
			abortOnce.Do(func() { f.AbortAll("mid-type") })
		}
	})
	f.Start()

	d.HoldOp("typehuman") // pin the worker inside the driver so pendings queue deterministically
	inflight := mustSubmit(t, f, Job{BrowserID: "n1", Timeout: 20 * time.Second,
		Action: TypeAction{Selector: "#in", Text: "abcdef", ClearFirst: true}})
	waitFor(t, 5*time.Second, "type job to reach the driver", func() bool { return d.Held() == 1 })
	pending := make([]<-chan JobResult, 3)
	for i := range pending {
		pending[i] = mustSubmit(t, f, Job{BrowserID: "n1", Action: ClickAction{Selector: "#p"}})
	}
	d.ReleaseOps() // typing proceeds; abort fires at char 3

	r := oneResult(t, inflight, 15*time.Second, "in-flight type during abort")
	if r.Status != StatusDone {
		t.Fatalf("in-flight type: status=%s err=%v — the critical section was torn by AbortAll", r.Status, r.Err)
	}
	if got := d.Value(); got != "abcdef" {
		t.Fatalf("field = %q after abort mid-type, want %q — keystroke stream was torn", got, "abcdef")
	}
	for i, ch := range pending {
		if r := oneResult(t, ch, 5*time.Second, fmt.Sprintf("pending job %d", i)); r.Status != StatusCancelled {
			t.Fatalf("pending job %d: status=%s err=%v, want cancelled", i, r.Status, r.Err)
		}
	}
	waitFor(t, 5*time.Second, "foreground token returned after abort", func() bool {
		return len(f.leaseToken) == 1
	})
	if _, err := f.Submit(remoteJob("n1")); !errors.Is(err, ErrFleetStopped) {
		t.Fatalf("Submit after AbortAll: err=%v, want ErrFleetStopped", err)
	}
	stopAndDrain(t, f)
	verifyNoNewGoroutines(t, snap)
}
