package automationfleet

// Shared fixtures for the phase-3 specs (Register / cap / pool). The faketest →
// fleet interface adapter itself lives in foreground_lease_test.go; this file
// only adds what the new specs need on top of it.

import (
	"testing"
	"time"

	"github.com/tuwibu/automationfleet/internal/faketest"
)

// newFakeHandle builds a BrowserHandle backed by the shared fake, in the mode
// the KIT reports. It deliberately does NOT set Native: deriving that is the
// behaviour under test, and pre-setting it would let a broken derivation pass.
func newFakeHandle(id string, native bool) (*BrowserHandle, *faketest.Driver) {
	d := faketest.New()
	d.SetNative(native)
	return &BrowserHandle{ID: id, Driver: fakeDriver{d}}, d
}

// registerFake registers a fake browser through the REAL Register path (unlike
// addFake, which writes the registry directly). The cap and expectation specs
// have to go through Register — that is where both live.
func registerFake(f *Fleet, id string, native bool, opts ...RegisterOption) error {
	h, _ := newFakeHandle(id, native)
	return f.Register(h, opts...)
}

// registeredIDs returns the ids currently in the registry.
func registeredIDs(f *Fleet) []string {
	f.mu.RLock()
	defer f.mu.RUnlock()
	out := make([]string, 0, len(f.handles))
	for id := range f.handles {
		out = append(out, id)
	}
	return out
}

// registeredCount returns how many handles are in the registry.
func registeredCount(f *Fleet) int { return len(registeredIDs(f)) }

// waitWG waits for the dispatcher's worker WaitGroup with a deadline, so a
// worker that never exits fails the test instead of hanging the package.
func waitWG(t *testing.T, f *Fleet, timeout time.Duration, msg string) {
	t.Helper()
	done := make(chan struct{})
	go func() {
		f.dispatcher.wg.Wait()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(timeout):
		t.Fatalf("dispatcher workers still running after %s: %s", timeout, msg)
	}
}

// remoteJob is a job that never needs the native critical section, so it always
// lands on the remote pool regardless of the handle's mode.
func remoteJob(id string) Job {
	return Job{BrowserID: id, Action: ScrollAction{DeltaY: 10}, Timeout: 5 * time.Second}
}
