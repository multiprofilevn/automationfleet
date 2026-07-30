//go:build windows

package automationfleet

// Phase-5 tier-1 stress, extra interleaving X5: concurrent native
// register/unregister churn must leave the takeover mouse hook exactly
// balanced — every install matched by an uninstall once the last native
// handle is gone, and the hook must still be installable afterwards. A leaked
// WH_MOUSE_LL hook is machine-wide (it lags the human's real mouse), so this
// is the one seam where "roughly cleaned up" is not good enough.

import (
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestStress_TakeoverHookInstallBalance_RegisterChurn(t *testing.T) {
	snap := leakSnapshot()
	var installs, uninstalls atomic.Int32
	prev := installMouseHook
	t.Cleanup(func() { installMouseHook = prev })
	installMouseHook = func(func()) (func(), error) {
		installs.Add(1)
		return func() { uninstalls.Add(1) }, nil
	}

	f := newTestFleet(t)
	f.Start()

	var wg sync.WaitGroup
	for g := 0; g < 4; g++ {
		wg.Add(1)
		go func(g int) {
			defer wg.Done()
			for i := 0; i < 50; i++ {
				id := fmt.Sprintf("wd-%d-%d", g, i)
				err := registerFake(f, id, true)
				if err != nil {
					// The cap can legitimately reject when 4 goroutines hold
					// slots at once; anything else is a real failure.
					if !errors.Is(err, ErrNativeCapacityExceeded) {
						t.Errorf("register %s: %v", id, err)
					}
					continue
				}
				if err := f.Unregister(id); err != nil {
					t.Errorf("unregister %s: %v", id, err)
				}
			}
		}(g)
	}
	wg.Wait()

	// All native handles are gone → reconcile must have converged on
	// "uninstalled", with every install matched.
	waitFor(t, 5*time.Second, "hook install/uninstall balance after churn", func() bool {
		return installs.Load() == uninstalls.Load()
	})
	if f.Snapshot().HasNative {
		t.Fatal("Snapshot reports native handles after full churn drain")
	}

	// The reconcile path must not be wedged: one more native register must
	// install exactly one more hook, and its unregister must remove it.
	if err := registerFake(f, "wd-final", true); err != nil {
		t.Fatalf("post-churn register: %v", err)
	}
	waitFor(t, 2*time.Second, "hook reinstalled for the post-churn handle", func() bool {
		return installs.Load() == uninstalls.Load()+1
	})
	if err := f.Unregister("wd-final"); err != nil {
		t.Fatalf("post-churn unregister: %v", err)
	}
	waitFor(t, 2*time.Second, "hook removed with the last native handle", func() bool {
		return installs.Load() == uninstalls.Load()
	})
	stopAndDrain(t, f)
	verifyNoNewGoroutines(t, snap) // watchdog run-loop goroutines must not survive the churn
}
