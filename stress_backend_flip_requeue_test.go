//go:build windows

package automationfleet

// Phase-5 tier-1 stress, extra interleaving X4 — split from
// stress_extra_interleavings_test.go (200-line rule).

import (
	"fmt"
	"testing"
	"time"
)

// X4. Backend flip while jobs are queued: TypeActions enqueued against a
// NATIVE handle, then the id is unregistered and re-registered as REMOTE
// before the queue resumes. Routing is decided at pop time from the live
// registry, so every job must run down the remote path of the NEW driver —
// no native critical section, no frame pin, and the old driver untouched.
func TestStress_ReRegisterFlipsBackend_QueuedJobsFollowLiveHandle(t *testing.T) {
	f := newTestFleet(t, WithRemoteWorkers(4))
	oldH, oldD := newFakeHandle("b", true)
	if err := f.Register(oldH); err != nil {
		t.Fatalf("register native: %v", err)
	}
	f.Start()
	f.Pause("gate")
	chans := make([]<-chan JobResult, 20)
	for i := range chans {
		chans[i] = mustSubmit(t, f, Job{BrowserID: "b",
			Action: TypeAction{Selector: fmt.Sprintf("#t%d", i), Text: "hi"}})
	}
	if err := f.Unregister("b"); err != nil {
		t.Fatalf("unregister: %v", err)
	}
	newH, newD := newFakeHandle("b", false)
	if err := f.Register(newH); err != nil {
		t.Fatalf("re-register remote: %v", err)
	}
	f.Resume("gate")
	waitDrained(t, f, 15*time.Second, "20 flipped-backend type jobs")
	for i, ch := range chans {
		if r := oneResult(t, ch, 5*time.Second, fmt.Sprintf("flipped job %d", i)); r.Status != StatusDone {
			t.Fatalf("flipped job %d: status=%s err=%v", i, r.Status, r.Err)
		}
	}
	if got := newD.Count("focuselement"); got != 20 {
		t.Fatalf("remote focuselement = %d, want 20 — jobs did not follow the live handle", got)
	}
	if got := newD.Count("typehuman"); got != 20 {
		t.Fatalf("remote typehuman = %d, want 20", got)
	}
	if pins, clicks := newD.Count("invalidatenativewindow"), newD.Count("clickat"); pins != 0 || clicks != 0 {
		t.Fatalf("remote handle ran the native critical section (pins=%d clickat=%d) — routing used a stale mode", pins, clicks)
	}
	if got := len(oldD.Events()); got != 0 {
		t.Fatalf("old (unregistered) driver received %d ops, want 0", got)
	}
}
