package automationfleet

// Phase 3, R0 — shutdown safety (Test Spec 1-5).
//
// abortAll used to close cdpJobs and nil the field while nativeWorker read it
// unlocked. Phase 1 replaced that with a stopCh broadcast and workers that
// capture the channel at spawn, so several of these specs are REGRESSION
// GUARDS rather than red tests: they exist to make sure the runtime pool
// resizing this phase adds cannot quietly reintroduce the close/nil pattern.

import (
	"errors"
	"sync"
	"testing"
	"time"
)

// 1. AbortAll must leave cdpJobs alive. A closed channel makes every worker
// select fire instantly with a nil job; a nil one blocks them forever.
func TestAbortAll_DoesNotCloseCdpJobs(t *testing.T) {
	f := newTestFleet(t)
	f.Start()
	f.AbortAll("spec")

	d := f.dispatcher
	d.mu.Lock()
	ch := d.cdpJobs
	d.mu.Unlock()

	if ch == nil {
		t.Fatal("cdpJobs was nil'd by AbortAll — nativeWorker would read a nil field and block forever")
	}
	select {
	case _, ok := <-ch:
		if !ok {
			t.Fatal("cdpJobs was closed by AbortAll — a send from nativeWorker would panic")
		}
	default:
	}
}

// 2. Every worker exits through stopCh, none is left behind.
func TestAbortAll_WorkersExitViaStopCh(t *testing.T) {
	f := newTestFleet(t, WithRemoteWorkers(8))
	d := addFake(f, "r1", false)
	f.Start()

	d.HoldOp("evaluate")
	// Always release: a failed assertion below must not leave workers parked
	// inside the driver, or the fleet Stop in newTestFleet's cleanup never returns.
	t.Cleanup(d.ReleaseOps)
	for i := 0; i < 8; i++ {
		if _, err := f.Submit(remoteJob("r1")); err != nil {
			t.Fatalf("submit %d: %v", i, err)
		}
	}
	waitFor(t, 2*time.Second, "8 jobs to reach the driver", func() bool { return d.Held() == 8 })

	f.AbortAll("spec")
	d.ReleaseOps()

	waitWG(t, f, 3*time.Second, "AbortAll did not release the pool")
}

// 3. The spec that catches the original bug: resize the pool from 20 goroutines
// while AbortAll runs. Under -race -count=50 this must never panic, Stop must
// return, and no worker may be left hanging.
func TestSetRemoteWorkers_ConcurrentWithAbortAll(t *testing.T) {
	f := newTestFleet(t, WithRemoteWorkers(4))
	addFake(f, "r1", false)
	f.Start()

	var wg sync.WaitGroup
	start := make(chan struct{})
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			<-start
			for n := 0; n < 10; n++ {
				// Error is expected once the fleet stops — what must not
				// happen is a panic or a leaked goroutine.
				_ = f.SetRemoteWorkers(1 + (i+n)%16)
			}
		}(i)
	}
	wg.Add(1)
	go func() {
		defer wg.Done()
		<-start
		f.AbortAll("spec")
	}()

	close(start)
	wg.Wait()

	f.Stop()
	waitWG(t, f, 5*time.Second, "resize racing AbortAll left workers running")
}

// 4. Enqueueing right after AbortAll must be rejected, not sent into a closed
// channel.
func TestEnqueue_AfterAbortAll_NoSendOnClosedChannel(t *testing.T) {
	f := newTestFleet(t)
	addFake(f, "r1", false)
	f.Start()
	f.AbortAll("spec")

	if _, err := f.Submit(remoteJob("r1")); !errors.Is(err, ErrFleetStopped) {
		t.Fatalf("Submit after AbortAll: want ErrFleetStopped, got %v", err)
	}

	// White-box: the dispatcher's own entry point must reject too, without
	// touching cdpJobs.
	res := f.dispatcher.enqueue(remoteJob("r1"))
	select {
	case r := <-res:
		if r.Status != StatusRejected {
			t.Fatalf("enqueue after AbortAll: want StatusRejected, got %v", r.Status)
		}
		if !errors.Is(r.Err, ErrFleetStopped) {
			t.Fatalf("enqueue after AbortAll: want ErrFleetStopped, got %v", r.Err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("enqueue after AbortAll never delivered a result")
	}
}

// 5. One stopped flag, two readers. fleet.stopped and dispatcher.stopped are
// set on different lines under different locks; if they can disagree, Submit
// and enqueue disagree about whether the fleet is accepting work.
func TestStoppedFlags_SingleSourceOfTruth(t *testing.T) {
	f := newTestFleet(t)
	f.Start()

	fleetStopped := func() bool {
		f.mu.RLock()
		defer f.mu.RUnlock()
		return f.stopped
	}
	dispStopped := func() bool {
		f.dispatcher.mu.Lock()
		defer f.dispatcher.mu.Unlock()
		return f.dispatcher.stopped
	}

	if fleetStopped() != dispStopped() {
		t.Fatalf("flags disagree before stop: fleet=%v dispatcher=%v", fleetStopped(), dispStopped())
	}

	f.AbortAll("spec")

	waitFor(t, 2*time.Second, "both stopped flags to be set", func() bool {
		return fleetStopped() && dispStopped()
	})
	if fleetStopped() != dispStopped() {
		t.Fatalf("flags disagree after AbortAll: fleet=%v dispatcher=%v", fleetStopped(), dispStopped())
	}

	f.Stop()
	if fleetStopped() != dispStopped() {
		t.Fatalf("flags disagree after Stop: fleet=%v dispatcher=%v", fleetStopped(), dispStopped())
	}
}
