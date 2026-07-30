//go:build integration

// Overhead A/B benchmarks for what phases 1-2 added to the critical section,
// each measured on vs off so the delta is attributable to one thing:
//
//	foreground guard        -> BenchmarkTypeHumanPerChar (bench_native_stages_test.go)
//	WindowFromPoint gate     -> BenchmarkOverheadWindowFromPointGuard
//	ReadValue snapshot       -> BenchmarkOverheadReadValue
//	runtime.LockOSThread     -> BenchmarkOverheadLockOSThread
//	per-job frame re-measure -> BenchmarkOverheadFrameRemeasure
//
// WindowFromPoint / ReadValue / LockOSThread / frame re-measure are
// unconditional in production (dispatcher_critical_windows.go,
// dispatcher_type_repair_windows.go) — there is no launch-time flag to turn
// them off, unlike the kit's own foreground guard. So "off" here means the
// honest baseline: a loop doing nothing, to isolate exactly what the call
// itself costs. Only the foreground guard has a real two-process A/B via
// chromekit.WithForegroundGuard. Shared fixtures/replicas live in
// bench_native_guard_replica_test.go; false-rejection counting lives in
// bench_native_false_rejection_test.go.
package automationfleet_test

import (
	"runtime"
	"testing"

	"github.com/tuwibu/automationfleet"
)

// BenchmarkOverheadWindowFromPointGuard: click loop with vs without the
// click-target guard replica layered on top.
func BenchmarkOverheadWindowFromPointGuard(b *testing.B) {
	requireRealInput(b)

	b.Run("with_guard", func(b *testing.B) {
		br, _, page, x, y := oneNativeChrome(b)
		driver := automationfleet.WrapChrome(br)
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			_ = page.Mouse().MoveTo(x, y)
			_ = guardClickTargetReplica(driver)
			_ = page.Mouse().ClickAt(x, y)
		}
	})

	b.Run("without_guard", func(b *testing.B) {
		_, _, page, x, y := oneNativeChrome(b)
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			_ = page.Mouse().MoveTo(x, y)
			_ = page.Mouse().ClickAt(x, y)
		}
	})
}

// BenchmarkOverheadReadValue: the verify-then-repair snapshot read
// (typeWithRepair's page.ReadValue call), isolated against a no-op baseline.
func BenchmarkOverheadReadValue(b *testing.B) {
	requireRealInput(b)

	b.Run("call", func(b *testing.B) {
		_, _, page, _, _ := oneNativeChrome(b)
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			if _, err := page.ReadValue("#target"); err != nil {
				b.Fatalf("readValue: %v", err)
			}
		}
	})

	b.Run("noop", func(b *testing.B) {
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			_ = i
		}
	})
}

// BenchmarkOverheadLockOSThread: the runtime.LockOSThread/UnlockOSThread pair
// wrapped around every critical section (dispatcher_critical_windows.go),
// isolated against a no-op baseline. Pure stdlib — no browser needed, but
// still gated by requireRealInput per the phase's "everything behind the env
// gate" instruction.
func BenchmarkOverheadLockOSThread(b *testing.B) {
	requireRealInput(b)

	b.Run("call", func(b *testing.B) {
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			runtime.LockOSThread()
			runtime.UnlockOSThread()
		}
	})

	b.Run("noop", func(b *testing.B) {
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			_ = i
		}
	})
}

// BenchmarkOverheadFrameRemeasure: pinFrame's per-job re-measure
// (InvalidateNativeWindow + ContentOffset + WindowScale + WindowRect),
// isolated against a no-op baseline. Test Spec threshold 2: if the chrome
// side of this exceeds 50ms, per-job invalidate is not viable and a TTL is
// needed instead (see BenchmarkInvalidateNativeWindow for the single-call
// number this threshold is actually judged on).
func BenchmarkOverheadFrameRemeasure(b *testing.B) {
	requireRealInput(b)

	b.Run("call", func(b *testing.B) {
		br, _, _, _, _ := oneNativeChrome(b)
		driver := automationfleet.WrapChrome(br)
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			if err := pinFrameReplica(driver); err != nil {
				b.Fatalf("pinFrame: %v", err)
			}
		}
	})

	b.Run("noop", func(b *testing.B) {
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			_ = i
		}
	})
}
