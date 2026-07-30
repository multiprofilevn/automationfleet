//go:build windows

package automationfleet

// Shared helpers for the phase-5 stress suite (tier 1, fake driver only).
// Tier 2 lives in stress_fleet_real_test.go behind the `integration` build tag.
//
// Leak detection uses go.uber.org/goleak (added to go.mod for this phase, the
// alternative runtime.NumGoroutine polling is kept for the absolute-count
// growth test where goleak's pass/fail shape does not fit).

import (
	"math/rand"
	"runtime"
	"testing"
	"time"

	"go.uber.org/goleak"
)

// leakSnapshot captures the goroutines alive right now; verifyNoNewGoroutines
// later asserts nothing NEW survived. IgnoreCurrent rather than a bare
// VerifyNone keeps each stress scenario independent of whatever earlier
// package tests left behind.
func leakSnapshot() goleak.Option { return goleak.IgnoreCurrent() }

func verifyNoNewGoroutines(t *testing.T, snap goleak.Option) {
	t.Helper()
	goleak.VerifyNone(t, snap)
}

// stopAndDrain shuts the fleet down and waits for every dispatcher worker to
// exit — Stop alone only signals.
func stopAndDrain(t *testing.T, f *Fleet) {
	t.Helper()
	f.Stop()
	waitWG(t, f, 10*time.Second, "stress fleet shutdown")
}

// oneResult reads exactly one JobResult and then proves the channel closes
// without a second one — "no job reports twice" is an invariant of every
// stress scenario, not just the mixed-fleet test. deliver() closes the channel
// immediately after the send, so the second receive is effectively instant.
func oneResult(t *testing.T, ch <-chan JobResult, timeout time.Duration, what string) JobResult {
	t.Helper()
	r := resultWithin(t, ch, timeout, what)
	select {
	case _, ok := <-ch:
		if ok {
			t.Fatalf("%s: second result delivered on a job channel", what)
		}
	case <-time.After(timeout):
		t.Fatalf("%s: result channel not closed after delivery", what)
	}
	return r
}

// waitDrained bounds Fleet.Wait with a deadline so a wedged queue fails the
// test instead of hanging the whole package run.
func waitDrained(t *testing.T, f *Fleet, timeout time.Duration, msg string) {
	t.Helper()
	done := make(chan struct{})
	go func() { f.Wait(); close(done) }()
	select {
	case <-done:
	case <-time.After(timeout):
		t.Fatalf("queue not drained after %s: %s", timeout, msg)
	}
}

// gcStable runs the collector twice (finalizer-freed objects from the previous
// round need a second cycle) and samples goroutines + live heap objects.
func gcStable() (goroutines int, heapObjects uint64) {
	runtime.GC()
	runtime.GC()
	var ms runtime.MemStats
	runtime.ReadMemStats(&ms)
	return runtime.NumGoroutine(), ms.HeapObjects
}

// settledSample polls (with GC) until two consecutive goroutine samples agree,
// then returns the settled pair. Poll-with-deadline, never a bare sleep.
func settledSample(within time.Duration) (goroutines int, heapObjects uint64) {
	deadline := time.Now().Add(within)
	g, h := gcStable()
	for time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
		g2, h2 := gcStable()
		if g2 == g {
			return g2, h2
		}
		g, h = g2, h2
	}
	return g, h
}

// randText returns n lowercase letters from rng — deterministic per seed.
func randText(rng *rand.Rand, n int) string {
	const letters = "abcdefghijklmnopqrstuvwxyz"
	b := make([]byte, n)
	for i := range b {
		b[i] = letters[rng.Intn(len(letters))]
	}
	return string(b)
}
