package automationfleet

import (
	"context"
	"fmt"
	"time"

	"github.com/tuwibu/firefoxkit"
)

// WrapFirefox adapts a firefoxkit.Browser into a fleet Driver.
//
// Native input: firefoxkit measures the live content origin via
// MeasureContentOffset(HWND) and carries it as an absolute ContentOffsetX/Y
// (page_input_backend.go nativeWindow/buildNativeWindow), so CSS (0,0) maps onto
// the rendered content, not the tab/omnibox chrome. Both native=true and
// native=false (BiDi/Remote) are supported; native requires the browser's HWND
// to be resolvable (proc-backed Launch, or SetHWND on the Connect path).
func WrapFirefox(b *firefoxkit.Browser) Driver { return firefoxDriver{b} }

type firefoxDriver struct{ b *firefoxkit.Browser }

func (d firefoxDriver) Current() Page {
	p := d.b.Current()
	if p == nil {
		return nil
	}
	return firefoxPage{p}
}

func (d firefoxDriver) Focus(ctx context.Context) error { return d.b.Focus(ctx) }

func (d firefoxDriver) ContentOffset() (int, int, error) { return d.b.ContentOffset() }

// HWND delegates to firefoxkit's own resolution (SetHWND override → PID +
// window-class enumeration). Errors on the Connect path, where the guard then
// skips rather than failing every job.
func (d firefoxDriver) HWND() (uintptr, error) { return d.b.HWND() }

// WindowScale reports the DPI scale firefoxkit measures from the live HWND,
// which is the number its native backend actually converts CSS pixels with.
func (d firefoxDriver) WindowScale() (float64, error) { return d.b.WindowScale() }

// InvalidateNativeWindow makes firefoxkit re-measure the content origin + scale
// and push them into the live input backends in place.
func (d firefoxDriver) InvalidateNativeWindow() error { return d.b.InvalidateNativeWindow() }

// WindowRect converts firefoxkit's out-of-band window rectangle into the
// fleet's kit-agnostic Rect.
func (d firefoxDriver) WindowRect() (Rect, error) {
	r, err := d.b.WindowRect()
	if err != nil {
		return Rect{}, err
	}
	return Rect{Left: r.Left, Top: r.Top, Right: r.Right, Bottom: r.Bottom}, nil
}

func (d firefoxDriver) InputBackend() Backend {
	if d.b.InputBackend() == firefoxkit.BackendNative {
		return BackendNative
	}
	return BackendRemote // BackendBiDi → Remote
}

type firefoxPage struct{ p *firefoxkit.Page }

func (p firefoxPage) Mouse() Mouse       { return firefoxMouse{p.p.Mouse()} }
func (p firefoxPage) Keyboard() Keyboard { return firefoxKeyboard{p.p.Keyboard()} }

func (p firefoxPage) Evaluate(expr string, out any) error { return p.p.Evaluate(expr, out) }

func (p firefoxPage) Navigate(url string, timeout time.Duration) error {
	return p.p.Navigate(url, timeout)
}

func (p firefoxPage) WaitForSelector(selector string, timeout time.Duration) error {
	return p.p.WaitForSelector(selector, timeout)
}

// ElementCenter uses firefoxkit's selector-based ScrollIntoView + BoundingBox.
// firefox BoundingBox X/Y is the top-left origin (CSS px, viewport-relative).
func (p firefoxPage) ElementCenter(selector string) (float64, float64, error) {
	if err := p.p.ScrollIntoView(selector); err != nil {
		return 0, 0, err
	}
	bb, err := p.p.BoundingBox(selector)
	if err != nil {
		return 0, 0, err
	}
	if bb.Width <= 0 || bb.Height <= 0 {
		return 0, 0, fmt.Errorf("zero-size element %q", selector)
	}
	return bb.X + bb.Width/2, bb.Y + bb.Height/2, nil
}

// ReadValue reads the element's current value over BiDi. Fails loud when the
// element is missing rather than returning "" — see chromePage.ReadValue.
func (p firefoxPage) ReadValue(selector string) (string, error) {
	var v *string
	if err := p.p.Evaluate(readValueExpr(selector), &v); err != nil {
		return "", err
	}
	if v == nil {
		return "", fmt.Errorf("readValue: element %q not found or has no value", selector)
	}
	return *v, nil
}
