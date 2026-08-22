//go:build integration

// Shared helpers for bench_native_overhead_test.go and
// bench_native_false_rejection_test.go: a one-browser fixture, plus
// replicas of the fleet's own internal guards built from EXPORTED surface
// only (Driver/Page methods + internal/winapi, both legally reachable from
// this external test package since it lives under the module root).
//
// These are replicas, not the production code path — see the "Known scope
// decisions" section of benchmark-native-throughput.md for why this package
// stays black-box instead of white-boxing into `package automationfleet`.
package automationfleet_test

import (
	"testing"

	"github.com/multiprofilevn/automationfleet"
	"github.com/multiprofilevn/automationfleet/internal/winapi"
	"github.com/multiprofilevn/chromekit"
)

// oneNativeChrome is a small helper shared by the overhead and
// false-rejection benchmarks: one native chrome browser, registered,
// navigated to the bench page, with #btn's center resolved.
func oneNativeChrome(b *testing.B) (*chromekit.Browser, *automationfleet.Fleet, automationfleet.Page, float64, float64) {
	b.Helper()
	browsers := launchChromeFleet(b, 1, true, true)
	f, ids := newNativeChromeFleet(b, browsers)
	navigateAll(b, f, ids, benchPageURL(b))
	page := automationfleet.WrapChrome(browsers[0]).Current()
	x, y, err := page.ElementCenter("#btn")
	if err != nil {
		b.Fatalf("elementCenter: %v", err)
	}
	b.Cleanup(func() {
		f.Stop()
		closeChrome(browsers)
	})
	return browsers[0], f, page, x, y
}

// guardClickTargetReplica mirrors guardClickTarget in
// dispatcher_critical_windows.go. Returns true when the guard would REJECT
// the click — every unresolved input degrades to "allow", exactly like
// production.
func guardClickTargetReplica(driver automationfleet.Driver) bool {
	target, err := driver.HWND()
	if err != nil || target == 0 {
		return false
	}
	x, y, err := winapi.GetCursorPos()
	if err != nil {
		return false
	}
	under, err := winapi.WindowFromPoint(x, y)
	if err != nil || under == 0 {
		return false
	}
	return winapi.RootWindow(under) != winapi.RootWindow(target)
}

// pinFrameReplica mirrors pinFrame in dispatcher_coordinate_frame_windows.go
// — re-measure the coordinate frame once, using only exported Driver methods.
func pinFrameReplica(driver automationfleet.Driver) error {
	if err := driver.InvalidateNativeWindow(); err != nil {
		return err
	}
	if _, _, err := driver.ContentOffset(); err != nil {
		return err
	}
	if _, err := driver.WindowScale(); err != nil {
		return err
	}
	_, _ = driver.WindowRect() // best-effort, same as production (haveRect optional)
	return nil
}
