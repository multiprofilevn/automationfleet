//go:build integration

// False-rejection instrumentation: how often WindowFromPoint (phase 1) and
// the GetWindowRect clamp (phase 2) reject a click/move that was actually
// legitimate. Both guards default to a "degrade to allow" stance on any
// unresolved input specifically because a wrong rejection is destructive
// (native_guard.go, dispatcher_coordinate_frame_windows.go doc comments) —
// this is the only place real numbers exist to tune their tolerances.
package automationfleet_test

import (
	"testing"

	"github.com/tuwibu/automationfleet"
)

// BenchmarkFalseRejectionWindowFromPoint counts how often a LEGITIMATE click
// on the browser's own target gets rejected by the guard replica.
func BenchmarkFalseRejectionWindowFromPoint(b *testing.B) {
	requireRealInput(b)
	br, _, page, x, y := oneNativeChrome(b)
	driver := automationfleet.WrapChrome(br)

	var rejected int
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if err := page.Mouse().MoveTo(x, y); err != nil {
			b.Fatalf("moveTo: %v", err)
		}
		if guardClickTargetReplica(driver) {
			rejected++
		}
		if err := page.Mouse().ClickAt(x, y); err != nil {
			b.Fatalf("clickAt: %v", err)
		}
	}
	if b.N > 0 {
		b.ReportMetric(100*float64(rejected)/float64(b.N), "falseRejectPct")
	}
}

// BenchmarkFalseRejectionRectClamp counts how often WindowRect disagrees with
// itself across a legitimate MoveTo+ClickAt with no real window move —
// exactly the errWindowMoved false-positive risk documented in
// dispatcher_coordinate_frame_windows.go.
func BenchmarkFalseRejectionRectClamp(b *testing.B) {
	requireRealInput(b)
	br, _, page, x, y := oneNativeChrome(b)
	driver := automationfleet.WrapChrome(br)

	var rejected int
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		before, err := driver.WindowRect()
		if err != nil {
			continue // unresolved rect: production skips the clamp, so do we
		}
		if err := page.Mouse().MoveTo(x, y); err != nil {
			b.Fatalf("moveTo: %v", err)
		}
		after, err := driver.WindowRect()
		if err != nil {
			continue
		}
		if before != after {
			rejected++
		}
		if err := page.Mouse().ClickAt(x, y); err != nil {
			b.Fatalf("clickAt: %v", err)
		}
	}
	if b.N > 0 {
		b.ReportMetric(100*float64(rejected)/float64(b.N), "falseRejectPct")
	}
}
