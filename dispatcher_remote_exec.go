package automationfleet

// The remote (CDP / BiDi) execution path. Split out of dispatcher.go, which
// owns lifecycle, queueing and the native critical section.

import (
	"context"
	"errors"
	"fmt"
)

// executeCDPOnly handles parallel-safe actions through CDP — Scroll, Wait,
// plus Click/Type/Navigate when the handle is CDP-backed (Native=false).
// CDP click/type drives Chromium's internal input pipeline, not the OS
// cursor/keyboard — no drift check needed, multiple browsers run in
// parallel.
func (d *Dispatcher) executeCDPOnly(ctx context.Context, h *BrowserHandle, a Action) error {
	page := h.Driver.Current()
	if page == nil {
		return errors.New("automationfleet: no active page")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	switch act := a.(type) {
	case ScrollAction:
		return scrollViewport(ctx, page, act)
	case WaitAction:
		return page.WaitForSelector(act.Selector, act.Timeout)
	case ClickAction:
		// Mouse().Click = bezier glide + dwell + click. page.Click =
		// chromedp.Click instant teleport → anti-bot detect.
		return page.Mouse().Click(act.Selector)
	case TypeAction:
		// FocusElement: bezier glide → click → verify activeElement.
		// Sau đó TypeHuman gõ với 80-220ms/char + 5% typo.
		if err := page.Mouse().FocusElement(act.Selector); err != nil {
			return err
		}
		if act.ClearFirst {
			if err := page.Keyboard().ClearInput(); err != nil {
				return err
			}
		}
		return page.Keyboard().TypeHuman(act.Text)
	case NavigateAction:
		timeout := act.Timeout
		if timeout <= 0 {
			timeout = d.fleet.cfg.defaultTimeout
		}
		return page.Navigate(act.URL, timeout)
	default:
		return fmt.Errorf("automationfleet: cdp path got unexpected action %s", a.kind())
	}
}

// scrollViewport scrolls either the viewport or a specific element.
func scrollViewport(_ context.Context, page Page, a ScrollAction) error {
	if a.Selector != "" {
		expr := fmt.Sprintf(`document.querySelector(%q)?.scrollBy(0, %f)`, a.Selector, a.DeltaY)
		return page.Evaluate(expr, nil)
	}
	expr := fmt.Sprintf(`window.scrollBy(0, %f)`, a.DeltaY)
	return page.Evaluate(expr, nil)
}
