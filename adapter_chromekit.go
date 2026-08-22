package automationfleet

import (
	"context"
	"fmt"
	"time"

	"github.com/multiprofilevn/chromekit"
)

// WrapChrome adapts a chromekit.Browser into a fleet Driver.
func WrapChrome(b *chromekit.Browser) Driver { return chromeDriver{b} }

type chromeDriver struct{ b *chromekit.Browser }

func (d chromeDriver) Current() Page {
	p := d.b.Current()
	if p == nil {
		return nil // never wrap nil — chromePage{nil} would be a non-nil Page interface
	}
	return chromePage{p}
}

func (d chromeDriver) Focus(ctx context.Context) error { return d.b.Focus(ctx) }

func (d chromeDriver) ContentOffset() (int, int, error) { return d.b.ContentOffset() }

// HWND delegates to chromekit's own resolution order (explicit SetHWND → PID
// lookup → title fallback), which also re-validates a cached handle before
// returning it.
func (d chromeDriver) HWND() (uintptr, error) { return d.b.HWND() }

// WindowScale reports the DPI scale chromekit measures from the live HWND,
// which is the number its native backend actually converts CSS pixels with.
func (d chromeDriver) WindowScale() (float64, error) { return d.b.WindowScale() }

// InvalidateNativeWindow makes chromekit re-measure the content origin + scale
// and push them into the live input backends in place.
func (d chromeDriver) InvalidateNativeWindow() error { return d.b.InvalidateNativeWindow() }

// WindowRect converts chromekit's out-of-band window rectangle into the fleet's
// kit-agnostic Rect.
func (d chromeDriver) WindowRect() (Rect, error) {
	r, err := d.b.WindowRect()
	if err != nil {
		return Rect{}, err
	}
	return Rect{Left: r.Left, Top: r.Top, Right: r.Right, Bottom: r.Bottom}, nil
}

func (d chromeDriver) InputBackend() Backend {
	if d.b.InputBackend() == chromekit.BackendNative {
		return BackendNative
	}
	return BackendRemote
}

type chromePage struct{ p *chromekit.Page }

func (p chromePage) Mouse() Mouse       { return chromeMouse{p.p.Mouse()} }
func (p chromePage) Keyboard() Keyboard { return chromeKeyboard{p.p.Keyboard()} }

func (p chromePage) Evaluate(expr string, out any) error { return p.p.Evaluate(expr, out) }

func (p chromePage) Navigate(url string, timeout time.Duration) error {
	return p.p.Navigate(url, timeout)
}

func (p chromePage) WaitForSelector(selector string, timeout time.Duration) error {
	return p.p.WaitForSelector(selector, timeout)
}

// ElementCenter mirrors the original native scrollAndQueryCenter flow: scroll
// the element to viewport center, let layout settle, re-query the box (it can
// move after scroll), and return its center in CSS pixels. chromekit's
// BoundingBox sets X/Y == Left/Top, so X/Y is the top-left origin.
func (p chromePage) ElementCenter(selector string) (float64, float64, error) {
	scrollExpr := fmt.Sprintf(
		`(()=>{const el=document.querySelector(%q); if(!el) return false; el.scrollIntoView({block:'center', inline:'center'}); return true})()`,
		selector,
	)
	var ok bool
	if err := p.p.Evaluate(scrollExpr, &ok); err != nil {
		return 0, 0, fmt.Errorf("scrollIntoView: %w", err)
	}
	if !ok {
		return 0, 0, fmt.Errorf("element %q not found", selector)
	}
	time.Sleep(60 * time.Millisecond)

	node, err := p.p.QuerySelector(selector, 2*time.Second)
	if err != nil {
		return 0, 0, err
	}
	box, err := p.p.BoundingBox(node)
	if err != nil {
		return 0, 0, err
	}
	if box.Width <= 0 || box.Height <= 0 {
		return 0, 0, fmt.Errorf("zero-size element %q", selector)
	}
	return box.X + box.Width/2, box.Y + box.Height/2, nil
}

// ReadValue reads the element's current value over CDP. Fails loud when the
// element is missing rather than returning "": verify-then-repair must never
// mistake an unread field for an empty one and clear + retype on top of it.
func (p chromePage) ReadValue(selector string) (string, error) {
	var v *string
	if err := p.p.Evaluate(readValueExpr(selector), &v); err != nil {
		return "", err
	}
	if v == nil {
		return "", fmt.Errorf("readValue: element %q not found or has no value", selector)
	}
	return *v, nil
}
