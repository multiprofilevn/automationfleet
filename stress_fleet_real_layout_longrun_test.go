//go:build integration && windows

package automationfleet

// Tier-2 items 16 and 18. Same warning as stress_fleet_real_test.go: real
// input, env-gated, never run automatically. Item 18 runs ~10 minutes with 21
// real browsers — run it alone, on an idle machine.

import (
	"fmt"
	"math/rand"
	"testing"
	"time"

	"github.com/tuwibu/automationfleet/internal/winapi"
)

// 16. ForceENUSLayout is THREAD-scoped, not window-scoped. A per-window
// assertion structurally cannot catch the leak this test hunts: a runtime
// thread left pinned to en-US after RestoreLayout ran on the wrong thread.
// So the check enumerates every thread of THIS process and compares
// GetKeyboardLayout(tid) before vs after 50 TypeActions.
func TestStressReal_KeyboardLayoutThreadRestored(t *testing.T) {
	requireRealInput(t)
	useRealOSSeams(t)

	before := processThreadLayouts(t)
	testThreadBefore := winapi.GetCurrentKeyboardLayout()

	f := newTestFleet(t)
	launchChromeNative(t, f, "A", 0, 0)
	pageA := navigateHandle(t, f, "A", typingPage)
	f.Start()
	for i := 0; i < 50; i++ {
		r := resultWithin(t, mustSubmit(t, f, Job{BrowserID: "A", Timeout: 30 * time.Second,
			Action: TypeAction{Selector: "#field", Text: fmt.Sprintf("t%02d", i), ClearFirst: true}}),
			40*time.Second, fmt.Sprintf("layout type %d", i))
		if r.Status != StatusDone {
			t.Fatalf("type %d: status=%s err=%v", i, r.Status, r.Err)
		}
	}
	if v, err := pageA.ReadValue("#field"); err != nil || v != "t49" {
		t.Fatalf("last typed value = %q (err %v), want t49", v, err)
	}
	stopAndDrain(t, f) // native worker thread unpins before we sample

	if got := winapi.GetCurrentKeyboardLayout(); got != testThreadBefore {
		t.Fatalf("test thread keyboard layout drifted: 0x%x → 0x%x", testThreadBefore, got)
	}
	after := processThreadLayouts(t)
	for tid, hkl := range after {
		if !isENUS(hkl) {
			continue
		}
		if prev, existed := before[tid]; existed && isENUS(prev) {
			continue // was en-US before the run too — not our leak
		}
		t.Errorf("thread %d is pinned to en-US (hkl 0x%x) after the run — RestoreLayout ran on the wrong OS thread", tid, hkl)
	}
}

// 18. Long-run soak: 9 native Chrome + 12 remote Firefox, mixed workload for
// 10 minutes. Pass = own-process Windows handle count drifts ≤5% and no
// browser process is left orphaned after Close.
func TestStressReal_LongRun(t *testing.T) {
	requireRealInput(t)
	useRealOSSeams(t)
	if testing.Short() {
		t.Skip("long-run soak skipped in -short mode")
	}

	f := newTestFleet(t, WithRemoteWorkers(12), WithDefaultTimeout(30*time.Second))
	pids := make([]uint32, 0, 21)
	for i := 0; i < 9; i++ {
		b := launchChromeNative(t, f, fmt.Sprintf("c%d", i), (i%3)*640, (i/3)*360)
		pids = append(pids, b.PID())
		navigateHandle(t, f, fmt.Sprintf("c%d", i), clickRecorderPage)
	}
	for i := 0; i < 12; i++ {
		b := launchFirefoxRemote(t, f, fmt.Sprintf("f%d", i))
		pids = append(pids, b.PID())
		navigateHandle(t, f, fmt.Sprintf("f%d", i), clickRecorderPage)
	}
	f.Start()

	handlesBefore := ownProcessHandleCount(t)
	rng := rand.New(rand.NewSource(18))
	deadline := time.Now().Add(10 * time.Minute)
	jobs, failures := 0, 0
	for time.Now().Before(deadline) {
		var j Job
		if rng.Intn(3) == 0 { // native chrome share
			id := fmt.Sprintf("c%d", rng.Intn(9))
			switch rng.Intn(3) {
			case 0:
				j = Job{BrowserID: id, Action: ClickAction{Selector: "#target"}}
			case 1:
				j = Job{BrowserID: id, Action: TypeAction{Selector: "#count", Text: "9", ClearFirst: true}}
			default:
				j = Job{BrowserID: id, Action: ScrollAction{DeltaY: 40}}
			}
		} else { // remote firefox share
			id := fmt.Sprintf("f%d", rng.Intn(12))
			switch rng.Intn(3) {
			case 0:
				j = Job{BrowserID: id, Action: ClickAction{Selector: "#target"}}
			case 1:
				j = Job{BrowserID: id, Action: WaitAction{Selector: "#target", Timeout: 5 * time.Second}}
			default:
				j = Job{BrowserID: id, Action: ScrollAction{DeltaY: -40}}
			}
		}
		ch, err := f.Submit(j)
		if err != nil {
			t.Fatalf("submit after %d jobs: %v", jobs, err)
		}
		jobs++
		if jobs%50 == 0 { // periodic backpressure: drain before piling on
			r := resultWithin(t, ch, 60*time.Second, "long-run checkpoint job")
			if r.Status != StatusDone {
				failures++
			}
			waitDrained(t, f, 2*time.Minute, "long-run periodic drain")
		}
	}
	waitDrained(t, f, 5*time.Minute, "long-run final drain")
	t.Logf("long-run: %d jobs, %d checkpoint failures", jobs, failures)

	handlesAfter := ownProcessHandleCount(t)
	limit := handlesBefore + handlesBefore/20 // +5%
	if handlesAfter > limit {
		t.Errorf("own-process handle count %d → %d (> +5%% of baseline) — Windows handle leak", handlesBefore, handlesAfter)
	}

	stopAndDrain(t, f) // t.Cleanup closes every browser afterwards; orphan check below re-polls
	t.Cleanup(func() {
		deadline := time.Now().Add(30 * time.Second)
		for _, pid := range pids {
			for processAlive(pid) {
				if !time.Now().Before(deadline) {
					t.Errorf("browser pid %d still alive after Close — orphaned process tree", pid)
					break
				}
				time.Sleep(500 * time.Millisecond)
			}
		}
	})
}
