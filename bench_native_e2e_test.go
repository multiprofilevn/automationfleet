//go:build integration

package automationfleet_test

import (
	"errors"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/multiprofilevn/automationfleet"
)

// nativeFleetSizes / remoteWorkerSizes are the configurations the Architecture
// section of phase-04-benchmark-throughput-native.md calls for.
var nativeFleetSizes = []int{1, 3, 6, 9}
var remoteWorkerSizes = []int{4, 9, 16, 32}

// failsCriticalSectionInvariant reports whether err is the one class of
// failure Test Spec threshold 3 forbids at N=9: cursor drift or foreground
// loss. A fail here means phase 1/2 is not done — record it as a compile-time
// blocking bug in the report, never as a throughput number.
//
// errCursorDrift itself is unexported (dispatcher.go) and cannot be
// errors.Is-compared from this external package, so drift is matched on its
// wrapped message text; foreground loss is exported (ErrForegroundLost) and
// checked properly.
func failsCriticalSectionInvariant(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, automationfleet.ErrForegroundLost) {
		return true
	}
	return strings.Contains(err.Error(), "cursor drift detected")
}

// BenchmarkTypeActionE2E runs the full TypeAction path — native focus, click,
// verify-then-repair typing — across 1/3/6/9 native Chrome browsers sharing
// the single native worker.
func BenchmarkTypeActionE2E(b *testing.B) {
	requireRealInput(b)
	for _, n := range nativeFleetSizes {
		n := n
		b.Run(strconv.Itoa(n), func(b *testing.B) {
			browsers := launchChromeFleet(b, n, true, true)
			defer closeChrome(browsers)
			f, ids := newNativeChromeFleet(b, browsers)
			defer f.Stop()
			navigateAll(b, f, ids, benchPageURL(b))

			rec := newLatencyRecorder()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				id := ids[i%len(ids)]
				start := time.Now()
				res := submitAndWait(b, f, id, automationfleet.TypeAction{
					Selector: "#target", Text: "throughput", ClearFirst: true,
				})
				rec.record("type", time.Since(start), res.Err)
				if res.Status != automationfleet.StatusDone {
					if failsCriticalSectionInvariant(res.Err) {
						b.Fatalf("N=%d: job failed on cursor drift / foreground loss (phase 1/2 not done): %v", n, res.Err)
					}
					b.Fatalf("N=%d: job status=%s err=%v", n, res.Status, res.Err)
				}
			}
			b.StopTimer()
			logPercentiles(b, rec)
		})
	}
}

// BenchmarkClickActionE2E is BenchmarkTypeActionE2E's click-only counterpart.
func BenchmarkClickActionE2E(b *testing.B) {
	requireRealInput(b)
	for _, n := range nativeFleetSizes {
		n := n
		b.Run(strconv.Itoa(n), func(b *testing.B) {
			browsers := launchChromeFleet(b, n, true, true)
			defer closeChrome(browsers)
			f, ids := newNativeChromeFleet(b, browsers)
			defer f.Stop()
			navigateAll(b, f, ids, benchPageURL(b))

			rec := newLatencyRecorder()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				id := ids[i%len(ids)]
				start := time.Now()
				res := submitAndWait(b, f, id, automationfleet.ClickAction{Selector: "#btn"})
				rec.record("click", time.Since(start), res.Err)
				if res.Status != automationfleet.StatusDone {
					if failsCriticalSectionInvariant(res.Err) {
						b.Fatalf("N=%d: job failed on cursor drift / foreground loss (phase 1/2 not done): %v", n, res.Err)
					}
					b.Fatalf("N=%d: job status=%s err=%v", n, res.Status, res.Err)
				}
			}
			b.StopTimer()
			logPercentiles(b, rec)
		})
	}
}

// BenchmarkRemoteE2E is the comparison point: the same actions over CDP
// (remote), which does not share a single OS cursor and so parallelises for
// real. Answers "does hybrid (few native + many remote) beat native alone".
func BenchmarkRemoteE2E(b *testing.B) {
	requireRealInput(b)
	for _, n := range remoteWorkerSizes {
		n := n
		b.Run(strconv.Itoa(n), func(b *testing.B) {
			browsers := launchChromeFleet(b, n, false, false)
			defer closeChrome(browsers)
			f, ids := newRemoteChromeFleet(b, browsers, n)
			defer f.Stop()
			navigateAll(b, f, ids, benchPageURL(b))

			rec := newLatencyRecorder()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				id := ids[i%len(ids)]
				var act automationfleet.Action = automationfleet.ClickAction{Selector: "#btn"}
				op := "click"
				if i%2 == 1 {
					act = automationfleet.TypeAction{Selector: "#target", Text: "remote", ClearFirst: true}
					op = "type"
				}
				start := time.Now()
				res := submitAndWait(b, f, id, act)
				rec.record(op, time.Since(start), res.Err)
				if res.Status != automationfleet.StatusDone {
					b.Fatalf("workers=%d: job status=%s err=%v", n, res.Status, res.Err)
				}
			}
			b.StopTimer()
			logPercentiles(b, rec)
		})
	}
}
