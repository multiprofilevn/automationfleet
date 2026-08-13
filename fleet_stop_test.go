package automationfleet

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestFleet_StopWaitsForInflightAndWorkers(t *testing.T) {
	f := newTestFleet(t, WithRemoteWorkers(1))
	d := addFake(f, "r1", false)
	d.HoldOp("evaluate")
	t.Cleanup(d.ReleaseOps)
	f.Start()

	result := mustSubmit(t, f, remoteJob("r1"))
	waitFor(t, 2*time.Second, "remote job to enter driver", func() bool { return d.Held() == 1 })

	stopped := make(chan struct{})
	go func() {
		f.Stop()
		close(stopped)
	}()
	stableFor(t, 100*time.Millisecond, "Stop must wait for the stubborn in-flight job", func() bool {
		select {
		case <-stopped:
			return false
		default:
			return true
		}
	})

	d.ReleaseOps()
	select {
	case <-stopped:
	case <-time.After(2 * time.Second):
		t.Fatal("Stop did not return after the in-flight job completed")
	}
	if got := f.Snapshot().RemoteWorkers; got != 0 {
		t.Fatalf("Stop returned with %d dispatcher workers still live", got)
	}
	if r := resultWithin(t, result, time.Second, "in-flight result"); r.Status != StatusDone {
		t.Fatalf("in-flight status = %s (err %v), want done", r.Status, r.Err)
	}
}

func TestFleet_StopContextHonorsDeadlineAndContinuesDraining(t *testing.T) {
	var stopCalls atomic.Int32
	f := newTestFleet(t, WithRemoteWorkers(1), OnStop(func(string) { stopCalls.Add(1) }))
	d := addFake(f, "r1", false)
	d.HoldOp("evaluate")
	t.Cleanup(d.ReleaseOps)
	f.Start()
	result := mustSubmit(t, f, remoteJob("r1"))
	waitFor(t, 2*time.Second, "remote job to enter driver", func() bool { return d.Held() == 1 })

	ctx, cancel := context.WithTimeout(context.Background(), 80*time.Millisecond)
	defer cancel()
	if err := f.StopContext(ctx); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("StopContext error = %v, want context.DeadlineExceeded", err)
	}
	if got := stopCalls.Load(); got != 1 {
		t.Fatalf("OnStop calls after deadline = %d, want 1", got)
	}

	d.ReleaseOps()
	if err := f.StopContext(context.Background()); err != nil {
		t.Fatalf("second StopContext after drain: %v", err)
	}
	if got := stopCalls.Load(); got != 1 {
		t.Fatalf("OnStop calls after repeated stop = %d, want 1", got)
	}
	_ = resultWithin(t, result, time.Second, "eventual in-flight result")
}

func TestFleet_ConcurrentStartAndStopIsSafe(t *testing.T) {
	f := newTestFleet(t, WithRemoteWorkers(4))

	const callers = 32
	start := make(chan struct{})
	var wg sync.WaitGroup
	for i := 0; i < callers; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			<-start
			if i%2 == 0 {
				f.Start()
				return
			}
			if err := f.StopContext(context.Background()); err != nil {
				t.Errorf("StopContext: %v", err)
			}
		}(i)
	}
	close(start)
	wg.Wait()

	f.Start()
	f.Stop()
	f.dispatcher.mu.Lock()
	live := f.dispatcher.remoteLive
	started := f.dispatcher.started
	stopped := f.dispatcher.stopped
	f.dispatcher.mu.Unlock()
	if live != 0 {
		t.Fatalf("concurrent Start/Stop left %d workers alive", live)
	}
	if !stopped {
		t.Fatal("concurrent Start/Stop did not leave dispatcher stopped")
	}
	if got := f.Snapshot().RemoteWorkers; started && got != 0 {
		t.Fatalf("concurrent Start/Stop left %d workers alive", got)
	}
}

func TestFleet_HotkeyStopUsesSameDrain(t *testing.T) {
	var reason atomic.Value
	f := newTestFleet(t, WithRemoteWorkers(1), OnStop(func(got string) { reason.Store(got) }))
	d := addFake(f, "r1", false)
	d.HoldOp("evaluate")
	t.Cleanup(d.ReleaseOps)
	f.Start()
	_ = mustSubmit(t, f, remoteJob("r1"))
	waitFor(t, 2*time.Second, "remote job to enter driver", func() bool { return d.Held() == 1 })

	f.requestStop("hotkey")
	done := make(chan struct{})
	go func() {
		f.Stop()
		close(done)
	}()
	stableFor(t, 100*time.Millisecond, "Stop after hotkey must join the same drain", func() bool {
		select {
		case <-done:
			return false
		default:
			return true
		}
	})

	d.ReleaseOps()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Stop did not observe completion of hotkey-triggered drain")
	}
	if got, _ := reason.Load().(string); got != "hotkey" {
		t.Fatalf("OnStop reason = %q, want hotkey", got)
	}
}

func TestFleet_StopContextWaitsForHotkeyTeardown(t *testing.T) {
	f := newTestFleet(t)
	hotkeyDone := make(chan struct{})
	var releaseOnce sync.Once
	releaseHotkey := func() { releaseOnce.Do(func() { close(hotkeyDone) }) }
	t.Cleanup(releaseHotkey)

	f.lifecycleMu.Lock()
	f.hotkeyDone = hotkeyDone
	f.lifecycleMu.Unlock()

	stopped := make(chan error, 1)
	go func() { stopped <- f.StopContext(context.Background()) }()
	stableFor(t, 100*time.Millisecond, "StopContext must wait for hotkey teardown", func() bool {
		select {
		case <-stopped:
			return false
		default:
			return true
		}
	})

	releaseHotkey()
	select {
	case err := <-stopped:
		if err != nil {
			t.Fatalf("StopContext after hotkey teardown: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("StopContext did not return after hotkey teardown")
	}
}

func TestFleet_StopContextDeadlineIncludesBlockingOnStop(t *testing.T) {
	release := make(chan struct{})
	f := newTestFleet(t, OnStop(func(string) { <-release }))
	t.Cleanup(func() { close(release) })
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	if err := f.StopContext(ctx); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("StopContext with blocked callback = %v, want deadline", err)
	}
}
