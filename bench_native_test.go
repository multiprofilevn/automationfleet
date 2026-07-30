//go:build integration

// !!! THESE BENCHMARKS DRIVE THE REAL OS MOUSE AND KEYBOARD !!!
//
// Running any Benchmark* below takes over the machine's cursor and keyboard
// for tens of minutes. A stray automated run — CI, a background `go test`,
// anything not started deliberately by a human sitting in front of the
// machine — WILL type into and click on whatever window currently has focus.
//
// Two independent guards exist because either one failing alone must not be
// enough to run this for real:
//  1. build tag `integration` — excluded from the default `go test ./...`.
//  2. requireRealInput(b) — every Benchmark* calls this first and skips
//     unless FLEET_ALLOW_REAL_INPUT=1 is set in the environment.
//
// Manual run (see benchmark-native-throughput.md in the plan directory for
// the full report template and the machine-spec fields to fill in):
//
//	FLEET_ALLOW_REAL_INPUT=1 go test -tags integration -run='^$' \
//	  -bench='Benchmark' -benchtime=20x ./... | tee bench.out
//
// The compile-only gate (safe to automate) is:
//
//	go vet -tags=integration ./... && \
//	  go test -tags=integration -run='^$' -bench='^$' ./...
package automationfleet_test

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/tuwibu/chromekit"
	"github.com/tuwibu/firefoxkit"
)

// requireRealInput is the mandatory env gate described in the package doc
// comment above. Every Benchmark* in this file set calls this first.
func requireRealInput(b *testing.B) {
	b.Helper()
	if os.Getenv("FLEET_ALLOW_REAL_INPUT") != "1" {
		b.Skip("set FLEET_ALLOW_REAL_INPUT=1 — this drives the real mouse and keyboard")
	}
}

// resolveChromePath mirrors chromekit's own stress-suite helper
// (chromekit/stress_helpers_test.go resolveChromePath): env override, else
// the standard Windows install dirs, else skip.
func resolveChromePath(b *testing.B) string {
	b.Helper()
	if p := os.Getenv("CHROME_PATH"); p != "" {
		return p
	}
	if runtime.GOOS != "windows" {
		b.Skip("expects Windows chrome.exe — set CHROME_PATH to override")
	}
	for _, c := range []string{
		`C:\Program Files\Google\Chrome\Application\chrome.exe`,
		`C:\Program Files (x86)\Google\Chrome\Application\chrome.exe`,
	} {
		if _, err := os.Stat(c); err == nil {
			return c
		}
	}
	b.Skip("no chrome.exe found in standard locations; set CHROME_PATH to run this benchmark")
	return ""
}

// resolveFirefoxPath mirrors firefoxkit's own stress-suite convention
// (firefoxkit/integration_stress_test.go stressFirefox): FIREFOXKIT_FIREFOX_PATH
// env var, or skip.
func resolveFirefoxPath(b *testing.B) string {
	b.Helper()
	p := os.Getenv("FIREFOXKIT_FIREFOX_PATH")
	if p == "" {
		b.Skip("FIREFOXKIT_FIREFOX_PATH not set; skipping firefox benchmark")
	}
	if _, err := os.Stat(p); err != nil {
		b.Skipf("firefox binary missing at %s: %v", p, err)
	}
	return p
}

// benchPageURL is the fixed local target — file://, no network, no
// animation, no web fonts (see testdata/bench-page.html).
func benchPageURL(b *testing.B) string {
	b.Helper()
	abs, err := filepath.Abs(filepath.Join("testdata", "bench-page.html"))
	if err != nil {
		b.Fatalf("bench page path: %v", err)
	}
	return "file://" + filepath.ToSlash(abs)
}

// cascadeXY spreads window i across the screen so headed windows don't fully
// occlude each other — same reasoning as chromekit's stress_fleet_test.go.
func cascadeXY(i int) (int, int) {
	return (i * 32) % 1400, (i * 28) % 700
}

// launchChromeFleet launches n separate Chrome processes, optionally on the
// native input backend, cascaded so headed windows stay visible.
func launchChromeFleet(b *testing.B, n int, native, headed bool) []*chromekit.Browser {
	b.Helper()
	path := resolveChromePath(b)
	browsers := make([]*chromekit.Browser, 0, n)
	for i := 0; i < n; i++ {
		x, y := cascadeXY(i)
		opts := []chromekit.Option{
			chromekit.WithHeadless(!headed),
			chromekit.WithFlag("no-first-run", true),
			chromekit.WithFlag("no-default-browser-check", true),
			chromekit.WithFlag("window-size", "480,360"),
			chromekit.WithFlag("window-position", fmt.Sprintf("%d,%d", x, y)),
		}
		if native {
			opts = append(opts, chromekit.WithInputBackend(chromekit.BackendNative))
		}
		br, err := chromekit.Launch(path, opts...)
		if err != nil {
			closeChrome(browsers)
			b.Fatalf("chrome launch %d: %v", i, err)
		}
		browsers = append(browsers, br)
	}
	return browsers
}

// launchFirefoxFleet launches n separate Firefox processes on the native
// input backend, cascaded the same way.
func launchFirefoxFleet(b *testing.B, n int, headed bool) []*firefoxkit.Browser {
	b.Helper()
	path := resolveFirefoxPath(b)
	browsers := make([]*firefoxkit.Browser, 0, n)
	for i := 0; i < n; i++ {
		br, err := firefoxkit.Launch(path,
			firefoxkit.WithHeadless(!headed),
			firefoxkit.WithInputBackend(firefoxkit.BackendNative),
			firefoxkit.WithCursorProfile(firefoxkit.CursorFast),
		)
		if err != nil {
			closeFirefox(browsers)
			b.Fatalf("firefox launch %d: %v", i, err)
		}
		if headed {
			x, y := cascadeXY(i)
			_ = br.SetWindowBounds(x, y, 480, 360)
		}
		browsers = append(browsers, br)
	}
	return browsers
}

func closeChrome(browsers []*chromekit.Browser) {
	for _, br := range browsers {
		br.Close()
	}
}

func closeFirefox(browsers []*firefoxkit.Browser) {
	for _, br := range browsers {
		_ = br.Close()
	}
}
