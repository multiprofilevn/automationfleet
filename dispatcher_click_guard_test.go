//go:build windows

package automationfleet

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"

	"github.com/multiprofilevn/automationfleet/internal/faketest"
)

// The cursor-drift guard only compares COORDINATES. If a second browser opens
// on top of the first at the very same screen position, drift passes perfectly
// and the click still lands in the wrong window — the mouse goes to the window
// under the cursor, not to the foreground window. These two specs pin the
// WindowFromPoint gate that closes that hole.

// clickGuardFixture builds a fleet + handle whose expected cursor position is
// exactly (0,0), which the package-wide cursorPos seam reports — so the drift
// guard always passes and the only thing under test is the window check.
//
// The target HWND is set on the Driver, not on the handle: the fleet asks the
// kit for the handle every time (Driver.HWND) so there is a single source of
// truth, rather than a copy the caller supplied once and that Windows may since
// have recycled onto an unrelated window.
func clickGuardFixture(t *testing.T, targetHWND uintptr) (*Fleet, *BrowserHandle, Page, *faketest.Driver) {
	t.Helper()
	f := New(WithPauseHotkeyDisabled(), WithResumeHotkeyDisabled())
	d := faketest.New()
	d.SetHWND(targetHWND)
	drv := fakeDriver{d}
	h := &BrowserHandle{ID: "n1", Driver: drv, Native: true}
	return f, h, drv.Current(), d
}

// 15. A foreign window under the cursor must abort BEFORE the click is emitted.
// Detecting it afterwards is worthless — the click already landed elsewhere.
func TestClickAt_RejectsWhenWindowUnderCursorIsForeign(t *testing.T) {
	prevWFP, prevRoot := windowFromPoint, rootWindow
	t.Cleanup(func() { windowFromPoint, rootWindow = prevWFP, prevRoot })

	var wfpCalls atomic.Int32
	const targetHWND = uintptr(0xCAFE)
	const foreignHWND = uintptr(0xBEEF)
	windowFromPoint = func(int, int) (uintptr, error) {
		wfpCalls.Add(1)
		return foreignHWND, nil
	}
	rootWindow = func(h uintptr) uintptr { return h }

	f, h, page, d := clickGuardFixture(t, targetHWND)

	err := clickAt(context.Background(), f, h, page, 0, 0)

	if wfpCalls.Load() == 0 {
		t.Fatal("clickAt never consulted WindowFromPoint — the click guard is missing")
	}
	if !errors.Is(err, ErrForegroundLost) {
		t.Fatalf("err = %v, want ErrForegroundLost", err)
	}
	if got := d.Count("clickat"); got != 0 {
		t.Fatalf("click was emitted into a foreign window: clickat = %d, want 0", got)
	}
}

// 16. A CHILD of the target window is legitimate — Chrome renders page content
// in a child HWND, so a guard comparing raw handles would reject every real
// click. The comparison must be on the root window (GetAncestor GA_ROOT).
func TestClickAt_AcceptsChildWindow(t *testing.T) {
	prevWFP, prevRoot := windowFromPoint, rootWindow
	t.Cleanup(func() { windowFromPoint, rootWindow = prevWFP, prevRoot })

	var wfpCalls atomic.Int32
	const targetHWND = uintptr(0xCAFE)
	const childHWND = uintptr(0xC41D)
	windowFromPoint = func(int, int) (uintptr, error) {
		wfpCalls.Add(1)
		return childHWND, nil
	}
	rootWindow = func(h uintptr) uintptr {
		if h == childHWND {
			return targetHWND
		}
		return h
	}

	f, h, page, d := clickGuardFixture(t, targetHWND)

	err := clickAt(context.Background(), f, h, page, 0, 0)

	if wfpCalls.Load() == 0 {
		t.Fatal("clickAt never consulted WindowFromPoint — the click guard is missing")
	}
	if err != nil {
		t.Fatalf("click into the target's own child window was rejected: %v", err)
	}
	if got := d.Count("clickat"); got != 1 {
		t.Fatalf("clickat = %d, want 1", got)
	}
}
