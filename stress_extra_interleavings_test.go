//go:build windows

package automationfleet

// Phase-5 tier-1 stress: interleavings BEYOND the 18 listed in the phase file.
// Each targets a seam phases 1-3 added that the listed cases leave untested:
// lease acquisition fairness under continuous native load, the pause-set ×
// lease predicate, Submit racing AbortAll, and pop-time routing after a
// backend flip. The takeover-hook balance check lives in
// stress_takeover_hook_balance_test.go.

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"
)

// X1. AcquireForeground fired into a CONTINUOUS stream of native jobs must not
// wait for the whole queue: leaseWanted is raised before the token wait, so at
// most the one in-flight job finishes first. A regression that made the worker
// win the token indefinitely (lease starvation) hangs the acquire and trips
// the deadline; a lease that acquired but did not freeze pops trips stableFor.
func TestStress_LeaseAcquire_NoStarvationUnderNativeLoad(t *testing.T) {
	f := newTestFleet(t)
	d := addFake(f, "n1", true)
	d.SetOpLatency("clickat", 500*time.Microsecond)
	f.Start()

	const jobs = 150
	chans := make([]<-chan JobResult, jobs)
	for i := range chans {
		chans[i] = mustSubmit(t, f, Job{BrowserID: "n1", Action: ClickAction{Selector: "#x"}})
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	l, err := f.AcquireForeground(ctx, "mid-stream")
	if err != nil {
		t.Fatalf("AcquireForeground starved under continuous native load: %v", err)
	}
	focusAt := d.Count("focus")
	if focusAt >= jobs {
		t.Fatalf("lease only acquired after the entire native queue drained (%d jobs)", focusAt)
	}
	stableFor(t, 30*time.Millisecond, "native job started under a held lease", func() bool {
		return d.Count("focus") == focusAt
	})
	l.Release()
	for i, ch := range chans {
		if r := oneResult(t, ch, 10*time.Second, fmt.Sprintf("native job %d", i)); r.Status != StatusDone {
			t.Fatalf("native job %d after release: status=%s err=%v", i, r.Status, r.Err)
		}
	}
}

// X2. Pause-reason set × foreground lease, interleaved: paused blocks even the
// remote path; resuming with the lease still held frees ONLY remote; a churn
// of two independent pause reasons must never wedge the cond, and releasing
// the lease at the end must drain the native job that waited through all of it.
func TestStress_PauseReasonChurn_LeaseAndRemoteInterleaved(t *testing.T) {
	f := newTestFleet(t, WithRemoteWorkers(4))
	nd := addFake(f, "n1", true)
	rd := addFake(f, "r1", false)
	f.Start()

	l := mustAcquire(t, f, context.Background(), "consumer")
	f.Pause("ui")
	natCh := mustSubmit(t, f, Job{BrowserID: "n1", Action: ClickAction{Selector: "#n"}})
	remCh := mustSubmit(t, f, remoteJob("r1"))
	stableFor(t, 40*time.Millisecond, "job ran while paused", func() bool {
		return nd.Count("focus") == 0 && rd.Count("evaluate") == 0
	})
	f.Resume("ui")
	if r := oneResult(t, remCh, 5*time.Second, "remote after resume"); r.Status != StatusDone {
		t.Fatalf("remote after resume: status=%s err=%v", r.Status, r.Err)
	}
	stableFor(t, 30*time.Millisecond, "native ran under held lease after resume", func() bool {
		return nd.Count("focus") == 0
	})

	var wg sync.WaitGroup
	for g := 0; g < 4; g++ {
		wg.Add(1)
		go func(g int) {
			defer wg.Done()
			reason := "a"
			if g%2 == 1 {
				reason = "b"
			}
			for i := 0; i < 200; i++ {
				if i%2 == 0 {
					f.Pause(reason)
				} else {
					f.Resume(reason)
				}
			}
		}(g)
	}
	extra := make([]<-chan JobResult, 50)
	for i := range extra {
		extra[i] = mustSubmit(t, f, remoteJob("r1"))
	}
	wg.Wait()
	f.Resume("a")
	f.Resume("b")
	for i, ch := range extra {
		if r := oneResult(t, ch, 10*time.Second, fmt.Sprintf("remote %d after churn", i)); r.Status != StatusDone {
			t.Fatalf("remote %d after reason churn: status=%s err=%v", i, r.Status, r.Err)
		}
	}
	l.Release()
	if r := oneResult(t, natCh, 5*time.Second, "native after lease release"); r.Status != StatusDone {
		t.Fatalf("native job after pause churn + release: status=%s err=%v", r.Status, r.Err)
	}
}

// X3. Submit racing AbortAll from 8 goroutines, five rounds with the abort
// landing at different points of the submission burst. Invariant: an accepted
// Submit delivers EXACTLY one result (any terminal status), a rejected one
// fails only with ErrFleetStopped — no lost jobs, no double delivery, no
// send-on-closed panic.
func TestStress_SubmitRacesAbortAll_NoJobLostNoDoubleResult(t *testing.T) {
	for round := 0; round < 5; round++ {
		f := New(WithPauseHotkeyDisabled(), WithResumeHotkeyDisabled(), WithRemoteWorkers(4))
		addFake(f, "r1", false)
		addFake(f, "n1", true)
		f.Start()

		accepted := make(chan (<-chan JobResult), 8*40)
		var wg sync.WaitGroup
		for g := 0; g < 8; g++ {
			wg.Add(1)
			go func(g int) {
				defer wg.Done()
				for i := 0; i < 40; i++ {
					j := remoteJob("r1")
					if i%4 == 0 {
						j = Job{BrowserID: "n1", Action: ClickAction{Selector: "#r"}}
					}
					ch, err := f.Submit(j)
					if err != nil {
						if !errors.Is(err, ErrFleetStopped) {
							t.Errorf("round %d: Submit failed with %v, want only ErrFleetStopped", round, err)
						}
						continue
					}
					accepted <- ch
				}
			}(g)
		}
		go func() {
			time.Sleep(time.Duration(round) * 500 * time.Microsecond)
			f.AbortAll("race")
		}()
		wg.Wait()
		close(accepted)
		for ch := range accepted {
			r := oneResult(t, ch, 10*time.Second, fmt.Sprintf("round %d raced job", round))
			switch r.Status {
			case StatusDone, StatusCancelled, StatusRejected, StatusFailed:
			default:
				t.Fatalf("round %d: unexpected status %v", round, r.Status)
			}
		}
		waitWG(t, f, 10*time.Second, "workers after submit/abort race")
	}
}
