//go:build windows

package automationfleet

import (
	"context"
	"sync"
	"testing"

	"github.com/multiprofilevn/automationfleet/internal/faketest"
	"github.com/multiprofilevn/automationfleet/internal/winapi"
)

// 22. ForceENUSLayout acts on the CALLING OS THREAD, not on the foreground
// window. Go may move a goroutine between threads at any preemption point
// inside TypeHuman, so without runtime.LockOSThread the layout can be forced on
// one thread while the typing runs on another (IME mitigation silently
// disabled), and the deferred RestoreLayout can pin an unrelated runtime thread
// to en-US forever.
//
// The assertion is on the lock/unlock seams rather than on observed thread
// migration: migration is probabilistic, so a test that only watched thread ids
// would pass by luck on an unfixed build. Thread stability is checked as a
// supporting signal.
func TestCriticalSection_LocksOSThread(t *testing.T) {
	prevLock, prevUnlock := lockOSThread, unlockOSThread
	t.Cleanup(func() { lockOSThread, unlockOSThread = prevLock, prevUnlock })

	var mu sync.Mutex
	var seq []string
	var tids []uint32
	rec := func(op string) {
		mu.Lock()
		seq = append(seq, op)
		tids = append(tids, winapi.CurrentThreadID())
		mu.Unlock()
	}

	lockOSThread = func() { prevLock(); rec("lock") }
	unlockOSThread = func() { rec("unlock"); prevUnlock() }

	f := New(WithPauseHotkeyDisabled(), WithResumeHotkeyDisabled())
	d := faketest.New()
	d.SetOnCall(rec)
	d.SetOnKey(func(i int) {
		if i%3 == 0 {
			rec("key")
		}
	})
	h := &BrowserHandle{ID: "n1", Driver: fakeDriver{d}, Native: true}

	if err := executeCritical(context.Background(), f, h,
		TypeAction{Selector: "#u", Text: "abcdefghij"}); err != nil {
		t.Fatalf("executeCritical: %v", err)
	}

	mu.Lock()
	defer mu.Unlock()

	if len(seq) < 3 {
		t.Fatalf("transcript too short: %v", seq)
	}
	var locks, unlocks int
	for _, op := range seq {
		switch op {
		case "lock":
			locks++
		case "unlock":
			unlocks++
		}
	}
	if locks != 1 || unlocks != 1 {
		t.Fatalf("lock/unlock = %d/%d, want 1/1 — transcript %v", locks, unlocks, seq)
	}
	if seq[0] != "lock" {
		t.Fatalf("first event = %q, want %q: the thread must be pinned before any native input", seq[0], "lock")
	}
	if seq[len(seq)-1] != "unlock" {
		t.Fatalf("last event = %q, want %q: the thread must stay pinned until RestoreLayout has run",
			seq[len(seq)-1], "unlock")
	}

	// Supporting signal: everything inside the section ran on one OS thread.
	for i, id := range tids {
		if id != tids[0] {
			t.Fatalf("critical section migrated OS thread at %q: %d != %d", seq[i], id, tids[0])
		}
	}
}
