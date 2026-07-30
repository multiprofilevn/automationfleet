//go:build windows

package automationfleet

import (
	"context"
	"fmt"
)

// executeCritical runs the atomic native sequence for a single job:
//
//	focus → scrollIntoView → re-query bbox → MoveTo → cursor drift guard
//	→ Click  [→ IME guard → Type → restore]
//
// The whole sequence holds the OS focus and runs on the dispatcher's single
// native worker — chromekit's package-level mutex is a defense-in-depth net,
// the contract here is "this function is the sole native input source".
func executeCritical(ctx context.Context, f *Fleet, h *BrowserHandle, a Action) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	// ForceENUSLayout acts on the CALLING OS THREAD, not on the foreground
	// window, and Go may move a goroutine between threads at any preemption
	// point inside TypeHuman. Unpinned, the layout can be forced on one thread
	// while the typing runs on another (IME mitigation silently disabled), and
	// the deferred RestoreLayout can pin an unrelated runtime thread to en-US
	// forever. The pin covers the whole section, layout switch included.
	lockOSThread()
	defer unlockOSThread()

	if err := h.Driver.Focus(ctx); err != nil {
		return fmt.Errorf("focus: %w", err)
	}

	page := h.Driver.Current()
	if page == nil {
		return fmt.Errorf("no active page on browser %s", h.ID)
	}

	switch act := a.(type) {
	case NavigateAction:
		// Browser already focused above; Page.Navigate handles the
		// omnibox keystroke sequence. Native input mutex inside chromekit
		// is defense-in-depth; the dispatcher's single native worker is
		// the real guarantee.
		timeout := act.Timeout
		if timeout <= 0 {
			timeout = f.cfg.defaultTimeout
		}
		return page.Navigate(act.URL, timeout)

	case ClickAction:
		if err := ctx.Err(); err != nil {
			return err
		}
		x, y, err := page.ElementCenter(act.Selector)
		if err != nil {
			return err
		}
		return clickAt(ctx, f, h, page, x, y)

	case TypeAction:
		if err := ctx.Err(); err != nil {
			return err
		}
		return typeWithRepair(ctx, f, h, page, act)

	default:
		return fmt.Errorf("executeCritical: unsupported action %s", a.kind())
	}
}

// clickAt drives Page.MoveTo + ClickAt against a coordinate frame pinned before
// the glide starts, guarding against two distinct failures: the window moving
// out from under the pinned frame (errWindowMoved) and the cursor ending up
// somewhere nobody asked for (errCursorDrift). Both feed the caller's retry
// path, which re-enters executeCritical and therefore re-pins the frame.
func clickAt(ctx context.Context, f *Fleet, h *BrowserHandle, page Page, cssX, cssY float64) error {
	frame, err := pinFrame(h)
	if err != nil {
		return err
	}

	if err := page.Mouse().MoveTo(cssX, cssY); err != nil {
		return fmt.Errorf("moveTo: %w", err)
	}

	// Ask the window manager again BEFORE judging the cursor. A window that
	// moved mid-glide leaves the cursor exactly where the pinned frame aimed
	// it — nothing drifted, the target did — and calling that drift would send
	// the retry loop back through the same stale frame.
	if frame.haveRect {
		if rect, rerr := h.Driver.WindowRect(); rerr == nil && rect != frame.rect {
			return fmt.Errorf("%w: pinned %+v, now %+v", errWindowMoved, frame.rect, rect)
		}
	}

	expectedX, expectedY := frame.screenFor(cssX, cssY)
	// cursorPos is the seam over winapi.GetCursorPos (see native_guard.go) so
	// tests can drive this path without depending on the real OS cursor.
	gotX, gotY, err := cursorPos()
	if err == nil {
		if abs(gotX-expectedX) > f.cfg.driftThreshold || abs(gotY-expectedY) > f.cfg.driftThreshold {
			return fmt.Errorf("%w: expected (%d,%d) got (%d,%d)", errCursorDrift,
				expectedX, expectedY, gotX, gotY)
		}
	}

	if err := ctx.Err(); err != nil {
		return err
	}
	if err := guardClickTarget(f, h); err != nil {
		return err
	}
	if err := page.Mouse().ClickAt(cssX, cssY); err != nil {
		return fmt.Errorf("click: %w", err)
	}
	return nil
}

// guardClickTarget refuses a click that would land in someone else's window.
//
// The drift guard above compares COORDINATES only, and the mouse goes to the
// window under the cursor rather than to the foreground window. A second
// browser opening on top of the target at the same screen position therefore
// passes the drift check perfectly while the click lands in the wrong browser.
//
// Every unknown degrades to "allow": an unresolvable HWND, a cursor position
// that cannot be read, or no window at that point must not fail a job for a
// reason unrelated to the foreground.
func guardClickTarget(f *Fleet, h *BrowserHandle) error {
	target, err := h.Driver.HWND()
	if err != nil || target == 0 {
		return nil
	}
	x, y, err := cursorPos()
	if err != nil {
		return nil
	}
	under, err := windowFromPoint(x, y)
	if err != nil || under == 0 {
		return nil
	}
	// Compare ROOTS: Chrome renders page content in a child HWND, so a
	// legitimate click reports the child and a raw handle comparison would
	// reject every real click.
	if rootWindow(under) == rootWindow(target) {
		return nil
	}
	f.log.Warnf("automationfleet: click aborted on browser=%s — window under cursor 0x%x is not 0x%x",
		h.ID, under, target)
	return &ForegroundLostError{Op: "click"}
}

func abs(x int) int {
	if x < 0 {
		return -x
	}
	return x
}
