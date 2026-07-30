//go:build integration_nativeclick && windows

// This file is behind its own build tag on purpose. It drives a REAL browser and
// moves a REAL window on whatever desktop it runs on, so it must never be part
// of `go test ./...`. Run it deliberately:
//
//	go test -tags integration_nativeclick -run TestNativeClick_AfterWindowMoved \
//	    -firefox "C:\Program Files\Mozilla Firefox\firefox.exe" ./...
//
// It is the end-to-end evidence for B4: everything else in this phase asserts
// against fixtures, and fixtures cannot tell you whether the pixel the OS
// actually clicked was the right one.

package automationfleet

import (
	"flag"
	"testing"
	"time"

	"github.com/tuwibu/firefoxkit"
)

var firefoxPath = flag.String("firefox", "", "path to firefox.exe for the native-click integration test")

// The page records every click it receives, so the assertion is what the OS
// delivered rather than what the fleet believes it sent.
const clickTargetPage = `data:text/html,<html><body style="margin:0">
<div id="target" style="position:absolute;left:300px;top:200px;width:120px;height:60px;background:#08f"></div>
<script>window.__hits=[];document.getElementById('target').addEventListener('click',()=>window.__hits.push(1))</script>
</body></html>`

// 19. TestNativeClick_AfterWindowMoved: click, move the window 200px, click
// again — the second click must still land inside the target.
//
// Before the coordinate-frame work the second click misses: the cached content
// offset still describes the window's old position, so the cursor is driven
// 200px away from the element and the click lands on the page background (or,
// with the window near a screen edge, outside the browser entirely).
func TestNativeClick_AfterWindowMoved(t *testing.T) {
	if *firefoxPath == "" {
		t.Skip("no -firefox path given")
	}

	b, err := firefoxkit.Launch(*firefoxPath,
		firefoxkit.WithInputBackend(firefoxkit.BackendNative),
	)
	if err != nil {
		t.Fatalf("launch firefox: %v", err)
	}
	t.Cleanup(func() { _ = b.Close() })

	if err := b.SetWindowBounds(100, 100, 1000, 800); err != nil {
		t.Fatalf("position window: %v", err)
	}
	page := b.Current()
	if page == nil {
		t.Fatal("no page after launch")
	}
	if err := page.Navigate(clickTargetPage, 10*time.Second); err != nil {
		t.Fatalf("navigate: %v", err)
	}

	f := New(WithPauseHotkeyDisabled(), WithResumeHotkeyDisabled())
	if err := f.RegisterFirefox("ff", b, true, 100, 100, 1.0); err != nil {
		t.Fatalf("register: %v", err)
	}
	f.Start()
	t.Cleanup(f.Stop)

	click := func(what string) {
		t.Helper()
		ch, err := f.Submit(Job{BrowserID: "ff", Action: ClickAction{Selector: "#target"}})
		if err != nil {
			t.Fatalf("submit %s click: %v", what, err)
		}
		select {
		case r := <-ch:
			if r.Status != StatusDone {
				t.Fatalf("%s click: status %s, err %v", what, r.Status, r.Err)
			}
		case <-time.After(30 * time.Second):
			t.Fatalf("%s click: timed out", what)
		}
	}

	hits := func() int {
		t.Helper()
		var n int
		if err := page.Evaluate(`window.__hits.length`, &n); err != nil {
			t.Fatalf("read hit count: %v", err)
		}
		return n
	}

	click("first")
	if got := hits(); got != 1 {
		t.Fatalf("first click landed %d times on the target, want 1 — the baseline is already broken", got)
	}

	// Move the window out from under the cached coordinate frame.
	if err := b.SetWindowBounds(300, 300, 1000, 800); err != nil {
		t.Fatalf("move window: %v", err)
	}
	// The window manager repositions asynchronously; poll for the move to have
	// actually happened rather than sleeping a guessed interval.
	deadline := time.Now().Add(5 * time.Second)
	for {
		r, err := b.WindowRect()
		if err == nil && r.Left >= 300 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("window never reported its new position")
		}
		time.Sleep(50 * time.Millisecond)
	}

	click("second")
	if got := hits(); got != 2 {
		t.Fatalf("after the window moved, the target was hit %d times total, want 2 — the click went somewhere else "+
			"on the desktop", got)
	}
}
