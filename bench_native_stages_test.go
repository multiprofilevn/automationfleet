//go:build integration

package automationfleet_test

import (
	"context"
	"testing"

	"github.com/tuwibu/automationfleet"
	"github.com/tuwibu/chromekit"
)

// These benchmarks isolate one chunk of the critical section at a time
// (Architecture section of phase-04-benchmark-throughput-native.md), driving
// the Driver/Page interfaces directly rather than through Fleet.Submit — the
// dispatcher's own overhead is measured separately by the E2E benchmarks.

// BenchmarkFocus times Driver.Focus alone.
func BenchmarkFocus(b *testing.B) {
	requireRealInput(b)
	browsers := launchChromeFleet(b, 1, true, true)
	defer closeChrome(browsers)
	driver := automationfleet.WrapChrome(browsers[0])
	ctx := context.Background()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if err := driver.Focus(ctx); err != nil {
			b.Fatalf("focus: %v", err)
		}
	}
}

// BenchmarkInvalidateNativeWindow compares the two kits' re-measure cost —
// chrome pays a JS eval, firefox pays syscalls (phase 2 Key Insight).
func BenchmarkInvalidateNativeWindow(b *testing.B) {
	requireRealInput(b)

	b.Run("chrome", func(b *testing.B) {
		browsers := launchChromeFleet(b, 1, true, true)
		defer closeChrome(browsers)
		driver := automationfleet.WrapChrome(browsers[0])
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			if err := driver.InvalidateNativeWindow(); err != nil {
				b.Fatalf("invalidate: %v", err)
			}
		}
	})

	b.Run("firefox", func(b *testing.B) {
		browsers := launchFirefoxFleet(b, 1, true)
		defer closeFirefox(browsers)
		driver := automationfleet.WrapFirefox(browsers[0])
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			if err := driver.InvalidateNativeWindow(); err != nil {
				b.Fatalf("invalidate: %v", err)
			}
		}
	})
}

// BenchmarkElementCenter times the scrollIntoView + re-query-bbox dance.
func BenchmarkElementCenter(b *testing.B) {
	requireRealInput(b)
	browsers := launchChromeFleet(b, 1, true, true)
	defer closeChrome(browsers)
	f, ids := newNativeChromeFleet(b, browsers)
	defer f.Stop()
	navigateAll(b, f, ids, benchPageURL(b))
	page := automationfleet.WrapChrome(browsers[0]).Current()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, _, err := page.ElementCenter("#target"); err != nil {
			b.Fatalf("elementCenter: %v", err)
		}
	}
}

// BenchmarkClickAt times MoveTo + ClickAt directly against the Page.Mouse
// surface. The dispatcher's own drift/window-moved re-check on top of this is
// measured by BenchmarkClickActionE2E and the overhead A/B benchmarks —
// isolating it here would require unexported access this external package
// deliberately does not have (see phase's Reuse strategy: measure, don't
// restructure).
func BenchmarkClickAt(b *testing.B) {
	requireRealInput(b)
	browsers := launchChromeFleet(b, 1, true, true)
	defer closeChrome(browsers)
	f, ids := newNativeChromeFleet(b, browsers)
	defer f.Stop()
	navigateAll(b, f, ids, benchPageURL(b))
	page := automationfleet.WrapChrome(browsers[0]).Current()
	x, y, err := page.ElementCenter("#btn")
	if err != nil {
		b.Fatalf("elementCenter: %v", err)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if err := page.Mouse().MoveTo(x, y); err != nil {
			b.Fatalf("moveTo: %v", err)
		}
		if err := page.Mouse().ClickAt(x, y); err != nil {
			b.Fatalf("clickAt: %v", err)
		}
	}
}

// BenchmarkTypeHumanPerChar compares TypeHuman with the kit's own foreground
// guard on vs off. Test Spec threshold 1: delta must be <= 2% — computed by
// hand from the p50 numbers each sub-benchmark logs (see
// benchmark-native-throughput.md), since go test does not correlate two
// separate -bench subtests' ns/op automatically.
func BenchmarkTypeHumanPerChar(b *testing.B) {
	requireRealInput(b)

	// Guard is on by default (chromekit.defaultConfig) — nothing to toggle
	// for the "on" side beyond a plain native launch.
	runGuardOn := func(b *testing.B) {
		browsers := launchChromeFleet(b, 1, true, true)
		defer closeChrome(browsers)
		f, ids := newNativeChromeFleet(b, browsers)
		defer f.Stop()
		navigateAll(b, f, ids, benchPageURL(b))
		page := automationfleet.WrapChrome(browsers[0]).Current()
		if _, _, err := page.ElementCenter("#target"); err != nil {
			b.Fatalf("elementCenter: %v", err)
		}
		if err := page.Mouse().FocusElement("#target"); err != nil {
			b.Fatalf("focusElement: %v", err)
		}

		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			if err := page.Keyboard().TypeHuman("x"); err != nil {
				b.Fatalf("typeHuman: %v", err)
			}
		}
	}

	// "off" needs a browser launched with the guard disabled — chromekit only
	// exposes that at Launch time (WithForegroundGuard), so the two sub-benches
	// necessarily run against two separate processes rather than one process
	// toggled mid-run. Documented as a decision, not an oversight.
	b.Run("guard_on", runGuardOn)
	b.Run("guard_off", func(b *testing.B) {
		requireRealInput(b)
		path := resolveChromePath(b)
		br, err := chromekit.Launch(path,
			chromekit.WithHeadless(false),
			chromekit.WithInputBackend(chromekit.BackendNative),
			chromekit.WithForegroundGuard(false),
		)
		if err != nil {
			b.Fatalf("chrome launch: %v", err)
		}
		defer br.Close()
		f, ids := newNativeChromeFleet(b, []*chromekit.Browser{br})
		defer f.Stop()
		navigateAll(b, f, ids, benchPageURL(b))
		page := automationfleet.WrapChrome(br).Current()
		if _, _, err := page.ElementCenter("#target"); err != nil {
			b.Fatalf("elementCenter: %v", err)
		}
		if err := page.Mouse().FocusElement("#target"); err != nil {
			b.Fatalf("focusElement: %v", err)
		}
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			if err := page.Keyboard().TypeHuman("x"); err != nil {
				b.Fatalf("typeHuman: %v", err)
			}
		}
	})
}
