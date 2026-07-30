package automationfleet

// Phase 3, R2 — native capacity (Test Spec 11-17).
//
// The cap is machine-wide, not per-kit: the limit is one mouse and one
// keyboard, so nine Chrome windows and nine Firefox windows are the same nine
// slots. Check-and-increment must happen inside ONE critical section — counting
// outside the mutex is how a cap of nine admits ten.

import (
	"errors"
	"fmt"
	"sync"
	"testing"
)

// 11. Default cap.
func TestNativeCapacity_DefaultIsNine(t *testing.T) {
	f := newTestFleet(t)
	used, max := f.NativeCapacity()
	if used != 0 {
		t.Errorf("fresh fleet used = %d, want 0", used)
	}
	if max != 9 {
		t.Errorf("default capacity = %d, want 9", max)
	}
}

// 12. Nine in, tenth out — and the tenth leaves no trace.
func TestRegister_TenthNativeRejected(t *testing.T) {
	f := newTestFleet(t)
	for i := 0; i < 9; i++ {
		if err := registerFake(f, fmt.Sprintf("n%d", i), true); err != nil {
			t.Fatalf("native #%d rejected: %v", i+1, err)
		}
	}
	err := registerFake(f, "n9", true)
	if !errors.Is(err, ErrNativeCapacityExceeded) {
		t.Fatalf("10th native: want ErrNativeCapacityExceeded, got %v", err)
	}
	if registeredCount(f) != 9 {
		t.Errorf("registry holds %d handles, want 9", registeredCount(f))
	}
	if used, _ := f.NativeCapacity(); used != 9 {
		t.Errorf("used = %d, want 9", used)
	}
}

// 13. One budget across kits. RegisterChrome and RegisterFirefox both funnel
// into Register, so the fake exercises the same counter both wrappers use.
func TestRegister_CapCountsAcrossKits(t *testing.T) {
	f := newTestFleet(t)
	for i := 0; i < 5; i++ {
		if err := registerFake(f, fmt.Sprintf("chrome-%d", i), true); err != nil {
			t.Fatalf("chrome #%d: %v", i+1, err)
		}
	}
	for i := 0; i < 4; i++ {
		if err := registerFake(f, fmt.Sprintf("firefox-%d", i), true); err != nil {
			t.Fatalf("firefox #%d: %v", i+1, err)
		}
	}
	if err := registerFake(f, "chrome-5", true); !errors.Is(err, ErrNativeCapacityExceeded) {
		t.Fatalf("6th chrome after 5+4 native: want ErrNativeCapacityExceeded, got %v — "+
			"the cap is being counted per kit instead of per machine", err)
	}
}

// 14. Remote handles are unbounded: they never touch the OS cursor.
func TestRegister_RemoteUnlimited(t *testing.T) {
	f := newTestFleet(t)
	for i := 0; i < 50; i++ {
		if err := registerFake(f, fmt.Sprintf("r%d", i), false); err != nil {
			t.Fatalf("remote #%d rejected: %v", i+1, err)
		}
	}
	if registeredCount(f) != 50 {
		t.Errorf("registry holds %d handles, want 50", registeredCount(f))
	}
	if used, _ := f.NativeCapacity(); used != 0 {
		t.Errorf("remote registrations consumed %d native slots, want 0", used)
	}
}

// 15. Unregister frees the slot immediately, not at some later reconcile.
func TestUnregister_FreesNativeSlot(t *testing.T) {
	f := newTestFleet(t)
	for i := 0; i < 9; i++ {
		if err := registerFake(f, fmt.Sprintf("n%d", i), true); err != nil {
			t.Fatalf("native #%d: %v", i+1, err)
		}
	}
	if err := f.Unregister("n0"); err != nil {
		t.Fatalf("Unregister: %v", err)
	}
	if used, _ := f.NativeCapacity(); used != 8 {
		t.Errorf("used after Unregister = %d, want 8", used)
	}
	if err := registerFake(f, "n9", true); err != nil {
		t.Fatalf("register into the freed slot: %v", err)
	}
}

// 16. EXACTLY nine. Not "at most nine on a good day" — check and increment must
// sit in the same critical section.
func TestRegister_CapRaceSafe(t *testing.T) {
	f := newTestFleet(t)

	var (
		mu       sync.Mutex
		ok       int
		capErr   int
		otherErr []error
		wg       sync.WaitGroup
	)
	start := make(chan struct{})
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			<-start
			err := registerFake(f, fmt.Sprintf("n%d", i), true)
			mu.Lock()
			defer mu.Unlock()
			switch {
			case err == nil:
				ok++
			case errors.Is(err, ErrNativeCapacityExceeded):
				capErr++
			default:
				otherErr = append(otherErr, err)
			}
		}(i)
	}
	close(start)
	wg.Wait()

	if len(otherErr) > 0 {
		t.Fatalf("unexpected errors: %v", otherErr)
	}
	if ok != 9 {
		t.Errorf("%d registrations succeeded, want exactly 9 — check-and-increment is not atomic", ok)
	}
	if capErr != 11 {
		t.Errorf("%d rejections, want 11", capErr)
	}
	if registeredCount(f) != 9 {
		t.Errorf("registry holds %d handles, want 9", registeredCount(f))
	}
	if used, _ := f.NativeCapacity(); used != 9 {
		t.Errorf("used = %d, want 9", used)
	}
}

// 17. Custom cap, and the two "not a number of slots" inputs fall back to 9.
func TestWithNativeCapacity_Custom(t *testing.T) {
	f := newTestFleet(t, WithNativeCapacity(3))
	if _, max := f.NativeCapacity(); max != 3 {
		t.Fatalf("capacity = %d, want 3", max)
	}
	for i := 0; i < 3; i++ {
		if err := registerFake(f, fmt.Sprintf("n%d", i), true); err != nil {
			t.Fatalf("native #%d: %v", i+1, err)
		}
	}
	if err := registerFake(f, "n3", true); !errors.Is(err, ErrNativeCapacityExceeded) {
		t.Errorf("4th native under cap 3: want ErrNativeCapacityExceeded, got %v", err)
	}

	for _, n := range []int{0, -1} {
		g := newTestFleet(t, WithNativeCapacity(n))
		if _, max := g.NativeCapacity(); max != 9 {
			t.Errorf("WithNativeCapacity(%d) → capacity %d, want the default 9", n, max)
		}
	}
}
