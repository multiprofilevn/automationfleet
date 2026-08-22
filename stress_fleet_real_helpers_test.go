//go:build integration && windows

package automationfleet

// Helpers for the phase-5 tier-2 REAL stress suite. See the warning at the top
// of stress_fleet_real_test.go — nothing here runs without
// FLEET_ALLOW_REAL_INPUT=1.

import (
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/multiprofilevn/automationfleet/internal/winapi"
	"github.com/multiprofilevn/chromekit"
	"github.com/multiprofilevn/firefoxkit"
)

// requireRealInput is the hard gate: without the env var every tier-2 test
// skips. This is what keeps an automated `go test -tags=integration` run from
// typing into whatever window the operator has open.
func requireRealInput(t *testing.T) {
	t.Helper()
	if os.Getenv("FLEET_ALLOW_REAL_INPUT") != "1" {
		t.Skip("tier-2 stress drives the REAL mouse and keyboard; set FLEET_ALLOW_REAL_INPUT=1 to run")
	}
}

// useRealOSSeams undoes the TestMain pinning (cursorPos / windowFromPoint /
// rootWindow / installMouseHook are faked for the whole package) for the
// duration of one tier-2 test. Without this the drift guard and click guard
// would run against the fake seams and prove nothing about the real OS.
func useRealOSSeams(t *testing.T) {
	t.Helper()
	pc, pw, pr, ph := cursorPos, windowFromPoint, rootWindow, installMouseHook
	cursorPos = winapi.GetCursorPos
	windowFromPoint = winapi.WindowFromPoint
	rootWindow = winapi.RootWindow
	installMouseHook = winapi.InstallMouseHook
	t.Cleanup(func() { cursorPos, windowFromPoint, rootWindow, installMouseHook = pc, pw, pr, ph })
}

func resolveChromeExe(t *testing.T) string {
	t.Helper()
	if p := os.Getenv("CHROME_PATH"); p != "" {
		return p
	}
	for _, c := range []string{
		`C:\Program Files\Google\Chrome\Application\chrome.exe`,
		`C:\Program Files (x86)\Google\Chrome\Application\chrome.exe`,
	} {
		if _, err := os.Stat(c); err == nil {
			return c
		}
	}
	t.Skip("no chrome.exe found; set CHROME_PATH")
	return ""
}

func resolveFirefoxExe(t *testing.T) string {
	t.Helper()
	if p := os.Getenv("FIREFOX_PATH"); p != "" {
		return p
	}
	c := `C:\Program Files\Mozilla Firefox\firefox.exe`
	if _, err := os.Stat(c); err == nil {
		return c
	}
	t.Skip("no firefox.exe found; set FIREFOX_PATH")
	return ""
}

// launchChromeNative launches a visible Chrome on the native input backend at
// the given screen position and registers it. Closed via t.Cleanup.
func launchChromeNative(t *testing.T, f *Fleet, id string, x, y int) *chromekit.Browser {
	t.Helper()
	b, err := chromekit.Launch(resolveChromeExe(t),
		chromekit.WithInputBackend(chromekit.BackendNative),
		chromekit.WithHeadless(false),
		chromekit.WithUserDataDir(t.TempDir()),
		chromekit.WithNativeWindow(x, y, 1.0),
		chromekit.WithFlag("window-position", fmt.Sprintf("%d,%d", x, y)),
	)
	if err != nil {
		t.Fatalf("launch chrome native %s: %v", id, err)
	}
	t.Cleanup(func() { _ = b.Close() })
	if err := f.RegisterChrome(id, b, ExpectNative()); err != nil {
		t.Fatalf("register chrome %s: %v", id, err)
	}
	return b
}

// launchFirefoxRemote launches a headless Firefox on the BiDi backend and
// registers it. Closed via t.Cleanup.
func launchFirefoxRemote(t *testing.T, f *Fleet, id string) *firefoxkit.Browser {
	t.Helper()
	b, err := firefoxkit.Launch(resolveFirefoxExe(t), firefoxkit.WithHeadless(true))
	if err != nil {
		t.Fatalf("launch firefox remote %s: %v", id, err)
	}
	t.Cleanup(func() { _ = b.Close() })
	if err := f.RegisterFirefox(id, b, ExpectRemote()); err != nil {
		t.Fatalf("register firefox %s: %v", id, err)
	}
	return b
}

// navigateHandle drives the kit's protocol navigation directly (NOT the
// omnibox NavigateAction) so page setup never depends on native input working.
func navigateHandle(t *testing.T, f *Fleet, id, url string) Page {
	t.Helper()
	h := f.handle(id)
	if h == nil {
		t.Fatalf("handle %s not registered", id)
	}
	page := h.Driver.Current()
	if page == nil {
		t.Fatalf("handle %s has no live page", id)
	}
	if err := page.Navigate(url, 20*time.Second); err != nil {
		t.Fatalf("navigate %s: %v", id, err)
	}
	return page
}

// clickRecorderPage counts clicks into plain <input> values so assertions go
// through ReadValue (protocol-side ground truth), never through the keyboard.
const clickRecorderPage = `data:text/html,<html><body style="margin:0">` +
	`<input id="count" value="0" style="position:absolute;top:0;left:0">` +
	`<div id="target" style="position:absolute;left:300px;top:200px;width:140px;height:70px;background:%2308f"></div>` +
	`<script>document.getElementById('target').addEventListener('click',()=>{` +
	`const c=document.getElementById('count');c.value=String(Number(c.value)+1)})</script></body></html>`

const typingPage = `data:text/html,<html><body><input id="field" style="width:400px;font-size:20px"></body></html>`
