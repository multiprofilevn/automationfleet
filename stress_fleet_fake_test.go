//go:build windows

package automationfleet

// Phase-5 tier-1 stress: mixed fleet under -race, goroutine leaks, and
// absolute-count growth. Test Spec items 1-3. Fake driver only — CI-safe.

import (
	"fmt"
	"math/rand"
	"sync"
	"testing"
	"time"

	"github.com/tuwibu/automationfleet/internal/faketest"
)

// 1. 9 native + 91 remote handles, 5000 jobs submitted from 8 racing
// goroutines. No job lost, no job reported twice, every native click really
// went through the single native worker.
func TestStress_MixedFleet_100Handles(t *testing.T) {
	snap := leakSnapshot()
	f := newTestFleet(t, WithRemoteWorkers(16))
	natives := make([]*faketest.Driver, 9)
	for i := range natives {
		natives[i] = addFake(f, fmt.Sprintf("n%d", i), true)
	}
	for i := 0; i < 91; i++ {
		addFake(f, fmt.Sprintf("r%d", i), false)
	}
	f.Start()

	const total, submitters = 5000, 8
	chunk := total / submitters
	results := make([]<-chan JobResult, total)
	var wg sync.WaitGroup
	for s := 0; s < submitters; s++ {
		wg.Add(1)
		go func(s int) {
			defer wg.Done()
			rng := rand.New(rand.NewSource(int64(s)))
			for i := 0; i < chunk; i++ {
				idx := s*chunk + i
				var j Job
				if idx%10 == 0 { // 500 native-critical clicks, serialized
					j = Job{BrowserID: fmt.Sprintf("n%d", rng.Intn(9)), Priority: rng.Intn(3),
						Action: ClickAction{Selector: "#s"}}
				} else {
					id := fmt.Sprintf("r%d", rng.Intn(91))
					switch idx % 4 {
					case 0:
						j = Job{BrowserID: id, Action: WaitAction{Selector: "#w", Timeout: time.Second}}
					case 1:
						j = Job{BrowserID: id, Action: ClickAction{Selector: "#c"}}
					case 2:
						j = Job{BrowserID: id, Action: TypeAction{Selector: "#t", Text: "x"}}
					default:
						j = Job{BrowserID: id, Action: ScrollAction{DeltaY: 5}}
					}
				}
				ch, err := f.Submit(j)
				if err != nil {
					t.Errorf("submitter %d job %d: %v", s, idx, err)
					return
				}
				results[idx] = ch
			}
		}(s)
	}
	wg.Wait()
	if t.Failed() {
		t.FailNow()
	}
	waitDrained(t, f, 60*time.Second, "5000 mixed jobs")

	for i, ch := range results {
		r := oneResult(t, ch, 5*time.Second, fmt.Sprintf("job %d", i))
		if r.Status != StatusDone {
			t.Fatalf("job %d: status=%s err=%v, want done", i, r.Status, r.Err)
		}
	}
	nativeClicks := 0
	for _, d := range natives {
		nativeClicks += d.Count("clickat")
	}
	if nativeClicks != total/10 {
		t.Fatalf("native clickat total = %d, want %d — native jobs were lost or misrouted", nativeClicks, total/10)
	}
	stopAndDrain(t, f)
	verifyNoNewGoroutines(t, snap)
}

// stressRound runs one full fleet lifecycle: start, mixed jobs, a pool resize
// in BOTH directions, drain, stop. Shared by tests 2 and 3 so the two leak
// gates measure the identical workload.
func stressRound(t *testing.T) {
	t.Helper()
	f := New(WithPauseHotkeyDisabled(), WithResumeHotkeyDisabled(), WithRemoteWorkers(4))
	addFake(f, "n1", true)
	addFake(f, "r1", false)
	f.Start()
	chans := make([]<-chan JobResult, 0, 30)
	for i := 0; i < 30; i++ {
		j := remoteJob("r1")
		if i%5 == 0 {
			j = Job{BrowserID: "n1", Action: ClickAction{Selector: "#x"}}
		}
		ch, err := f.Submit(j)
		if err != nil {
			t.Fatalf("round submit %d: %v", i, err)
		}
		chans = append(chans, ch)
	}
	if err := f.SetRemoteWorkers(9); err != nil {
		t.Fatalf("grow: %v", err)
	}
	if err := f.SetRemoteWorkers(2); err != nil {
		t.Fatalf("shrink: %v", err)
	}
	waitDrained(t, f, 15*time.Second, "round drain")
	for i, ch := range chans {
		if r := oneResult(t, ch, 5*time.Second, fmt.Sprintf("round job %d", i)); r.Status != StatusDone {
			t.Fatalf("round job %d: status=%s err=%v", i, r.Status, r.Err)
		}
	}
	stopAndDrain(t, f)
}

// 2. Ten start/submit/resize/stop cycles, goleak.VerifyNone after EVERY round
// — a worker or watchdog goroutine that outlives its fleet fails the round it
// leaked in, not a diffuse end-of-test count.
func TestStress_GoroutineLeak(t *testing.T) {
	for round := 1; round <= 10; round++ {
		snap := leakSnapshot()
		stressRound(t)
		verifyNoNewGoroutines(t, snap)
	}
}

// 3. Absolute counts after GC: goroutines and live heap objects must return to
// the round-2 level (±10%) on every later round. The old ×1.5 ratio gate let
// small linear leaks through and was itself flaky — this is the replacement,
// with a polling deadline instead of a fixed post-round sleep.
func TestStress_NoUnboundedGrowth(t *testing.T) {
	type sample struct {
		goroutines  int
		heapObjects uint64
	}
	var samples [10]sample
	for round := 0; round < 10; round++ {
		stressRound(t)
		g, h := settledSample(3 * time.Second)
		samples[round] = sample{g, h}
	}
	base := samples[1] // round 2: round 1 pays one-time init (lazy allocs, pools)
	gSlack := base.goroutines / 10
	if gSlack < 1 {
		gSlack = 1 // 10% of a single-digit count truncates to 0; ±10% must not mean "exactly equal"
	}
	hSlack := base.heapObjects / 10
	for round := 2; round < 10; round++ {
		s := samples[round]
		if s.goroutines > base.goroutines+gSlack || s.goroutines < base.goroutines-gSlack {
			t.Errorf("round %d: goroutines = %d, want %d ±%d — linear goroutine growth",
				round+1, s.goroutines, base.goroutines, gSlack)
		}
		if s.heapObjects > base.heapObjects+hSlack || s.heapObjects+hSlack < base.heapObjects {
			t.Errorf("round %d: live heap objects = %d, want %d ±%d — memory not returning to baseline",
				round+1, s.heapObjects, base.heapObjects, hSlack)
		}
	}
}
