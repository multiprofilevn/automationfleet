//go:build windows

package automationfleet

// Phase-5 tier-1 stress: scenarios 7-8 — pause/resume hotkey spam under load,
// queue priority and starvation resistance. Fake driver, CI-safe.

import (
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"
)

// 7. 1000 Pause/Resume transitions from 4 racing goroutines while a mixed
// load is being submitted. The hotkey listener itself is disabled in tests;
// the hotkey path calls exactly these two methods, so spamming them IS the
// hotkey spam. End state must be consistent (resumed, queue drains, no
// deadlock, no job lost).
func TestStress_HotkeySpamUnderLoad(t *testing.T) {
	snap := leakSnapshot()
	f := newTestFleet(t, WithRemoteWorkers(8))
	addFake(f, "n1", true)
	addFake(f, "r1", false)
	f.Start()

	const jobs = 300
	chans := make([]<-chan JobResult, jobs)
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < jobs; i++ {
			j := remoteJob("r1")
			if i%10 == 0 {
				j = Job{BrowserID: "n1", Action: ClickAction{Selector: "#spam"}}
			}
			ch, err := f.Submit(j)
			if err != nil {
				t.Errorf("submit %d during spam: %v", i, err)
				return
			}
			chans[i] = ch
		}
	}()
	for g := 0; g < 4; g++ {
		wg.Add(1)
		go func(g int) {
			defer wg.Done()
			for i := 0; i < 250; i++ {
				if (i+g)%2 == 0 {
					f.Pause("hotkey")
				} else {
					f.Resume("hotkey")
				}
			}
		}(g)
	}
	wg.Wait()
	if t.Failed() {
		t.FailNow()
	}
	f.Resume("hotkey") // deterministic final state: running
	waitDrained(t, f, 30*time.Second, "300 jobs under pause/resume spam")
	for i, ch := range chans {
		if r := oneResult(t, ch, 5*time.Second, fmt.Sprintf("spam-load job %d", i)); r.Status != StatusDone {
			t.Fatalf("job %d: status=%s err=%v, want done", i, r.Status, r.Err)
		}
	}
	if f.dispatcher.hasReason("hotkey") {
		t.Fatal("dispatcher still paused by 'hotkey' after the final Resume — reason set inconsistent")
	}
	stopAndDrain(t, f)
	verifyNoNewGoroutines(t, snap)
}

// 8. Priority ordering under load, two angles: (a) with a full queue released
// at once, every high-priority job executes before every low one (transcript
// order on the serial native worker is execution order); (b) a high-priority
// job submitted into a CONTINUOUS low-priority feed still completes promptly —
// the anti-starvation guarantee.
func TestStress_QueuePriorityUnderLoad(t *testing.T) {
	f := newTestFleet(t)
	d := addFake(f, "n1", true)
	f.Start()

	// (a) deterministic bulk ordering.
	f.Pause("gate")
	chans := make([]<-chan JobResult, 0, 120)
	for i := 0; i < 120; i++ {
		prio, sel := 1, fmt.Sprintf("#lo-%d", i)
		if i%6 == 0 {
			prio, sel = 10, fmt.Sprintf("#hi-%d", i)
		}
		chans = append(chans, mustSubmit(t, f, Job{BrowserID: "n1", Priority: prio,
			Action: ClickAction{Selector: sel}}))
	}
	f.Resume("gate")
	waitDrained(t, f, 30*time.Second, "120 prioritized jobs")
	for i, ch := range chans {
		if r := oneResult(t, ch, 5*time.Second, fmt.Sprintf("prio job %d", i)); r.Status != StatusDone {
			t.Fatalf("prio job %d: status=%s err=%v", i, r.Status, r.Err)
		}
	}
	lastHi, firstLo := -1, -1
	pos := 0
	for _, e := range d.Events() {
		if e.Op != "elementcenter" {
			continue
		}
		if strings.HasPrefix(e.Arg, "#hi-") {
			lastHi = pos
		} else if strings.HasPrefix(e.Arg, "#lo-") && firstLo == -1 {
			firstLo = pos
		}
		pos++
	}
	if lastHi == -1 || firstLo == -1 || lastHi > firstLo {
		t.Fatalf("priority inversion: last high-prio at position %d, first low-prio at %d", lastHi, firstLo)
	}

	// (b) starvation resistance under a live feed.
	d.SetOpLatency("clickat", 200*time.Microsecond)
	stop := make(chan struct{})
	var feederWG sync.WaitGroup
	feederWG.Add(1)
	go func() {
		defer feederWG.Done()
		for i := 0; i < 1500; i++ {
			select {
			case <-stop:
				return
			default:
			}
			if _, err := f.Submit(Job{BrowserID: "n1", Priority: 0,
				Action: ClickAction{Selector: "#feed"}}); err != nil {
				t.Errorf("feeder submit: %v", err)
				return
			}
		}
	}()
	// Let the feed build a real backlog before injecting the urgent job.
	waitFor(t, 5*time.Second, "backlog to form", func() bool { return d.Count("clickat") > 50 })
	hi := mustSubmit(t, f, Job{BrowserID: "n1", Priority: 10, Action: ClickAction{Selector: "#urgent"}})
	if r := resultWithin(t, hi, 3*time.Second, "high-prio job under continuous low-prio feed"); r.Status != StatusDone {
		t.Fatalf("urgent job starved: status=%s err=%v", r.Status, r.Err)
	}
	close(stop)
	feederWG.Wait()
	waitDrained(t, f, 30*time.Second, "feeder backlog drain")
}
