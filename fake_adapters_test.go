package automationfleet

// ---------------------------------------------------------------------------
// faketest → fleet interface adapter.
//
// internal/faketest cannot import automationfleet (these tests are white-box,
// so that would be an import cycle). All behaviour lives in the fake; this is
// pure delegation. Split from foreground_lease_test.go (phase 8) once the
// widened Driver/Page/Mouse/Keyboard surface made the block its own concern.
// ---------------------------------------------------------------------------

import (
	"context"
	"time"

	"github.com/tuwibu/automationfleet/internal/faketest"
)

type fakeDriver struct{ d *faketest.Driver }

func (f fakeDriver) Current() Page {
	if f.d.HasNoPage() {
		return nil
	}
	return fakePage{f.d}
}
func (f fakeDriver) Focus(ctx context.Context) error  { return f.d.Focus(ctx) }
func (f fakeDriver) ContentOffset() (int, int, error) { return f.d.ContentOffset() }
func (f fakeDriver) HWND() (uintptr, error)           { return f.d.HWND() }
func (f fakeDriver) WindowScale() (float64, error)    { return f.d.WindowScale() }
func (f fakeDriver) InvalidateNativeWindow() error    { return f.d.InvalidateNativeWindow() }
func (f fakeDriver) WindowRect() (Rect, error) {
	r, err := f.d.WindowRect()
	return Rect{Left: r.Left, Top: r.Top, Right: r.Right, Bottom: r.Bottom}, err
}
func (f fakeDriver) InputBackend() Backend {
	if f.d.IsNative() {
		return BackendNative
	}
	return BackendRemote
}

type fakePage struct{ d *faketest.Driver }

func (p fakePage) Mouse() Mouse                         { return fakeMouse{p.d} }
func (p fakePage) Keyboard() Keyboard                   { return fakeKeyboard{p.d} }
func (p fakePage) Evaluate(expr string, out any) error  { return p.d.Evaluate(expr, out) }
func (p fakePage) ReadValue(sel string) (string, error) { return p.d.ReadValue(sel) }
func (p fakePage) Navigate(url string, timeout time.Duration) error {
	return p.d.Navigate(url, timeout)
}
func (p fakePage) WaitForSelector(sel string, timeout time.Duration) error {
	return p.d.WaitForSelector(sel, timeout)
}
func (p fakePage) ElementCenter(sel string) (float64, float64, error) {
	return p.d.ElementCenter(sel)
}

// -- widened (phase 8): pure delegation, same as the original five above.

func (p fakePage) EvalSelector(sel, fnBody string, out any) error {
	return p.d.EvalSelector(sel, fnBody, out)
}
func (p fakePage) EvalSelectorAll(sel, jsFn string, out any) error {
	return p.d.EvalSelectorAll(sel, jsFn, out)
}
func (p fakePage) EvaluateCtx(ctx context.Context, expression string, out any) error {
	return p.d.EvaluateCtx(ctx, expression, out)
}
func (p fakePage) Reload(timeout time.Duration) error    { return p.d.Reload(timeout) }
func (p fakePage) GoBack(timeout time.Duration) error    { return p.d.GoBack(timeout) }
func (p fakePage) GoForward(timeout time.Duration) error { return p.d.GoForward(timeout) }
func (p fakePage) GetURL() (string, error)               { return p.d.GetURL() }
func (p fakePage) GetTitle() (string, error)             { return p.d.GetTitle() }
func (p fakePage) GetOrigin() (string, error)            { return p.d.GetOrigin() }
func (p fakePage) URL() string                           { return p.d.URL() }
func (p fakePage) Title() string                         { return p.d.Title() }
func (p fakePage) GetText(sel string) (string, error)    { return p.d.GetText(sel) }
func (p fakePage) GetAttribute(sel, attribute string) (string, error) {
	return p.d.GetAttribute(sel, attribute)
}
func (p fakePage) ScrollIntoView(sel string) error { return p.d.ScrollIntoView(sel) }

// Focus delegates to the fake's FocusSelector — named distinctly from the
// fake's Driver-level Focus(ctx) since Go forbids two methods sharing a name
// on one receiver. See faketest/driver_page.go.
func (p fakePage) Focus(sel string) error { return p.d.FocusSelector(sel) }
func (p fakePage) WaitForReady(timeout time.Duration) error {
	return p.d.WaitForReady(timeout)
}
func (p fakePage) WaitForChangeURL(timeout time.Duration) error {
	return p.d.WaitForChangeURL(timeout)
}
func (p fakePage) WaitForFunction(expr string, timeout time.Duration) error {
	return p.d.WaitForFunction(expr, timeout)
}
func (p fakePage) WaitForSelectorGone(sel string, timeout time.Duration) error {
	return p.d.WaitForSelectorGone(sel, timeout)
}
func (p fakePage) Screenshot() ([]byte, error) { return p.d.Screenshot() }
func (p fakePage) ScreenshotClip(x, y, w, h float64) ([]byte, error) {
	return p.d.ScreenshotClip(x, y, w, h)
}
func (p fakePage) ScreenshotElement(sel string) ([]byte, error) { return p.d.ScreenshotElement(sel) }
func (p fakePage) FullScreenshot(quality int) ([]byte, error)   { return p.d.FullScreenshot(quality) }
func (p fakePage) SetInputFiles(sel string, paths ...string) error {
	return p.d.SetInputFiles(sel, paths...)
}
func (p fakePage) OnDialog(handler func(DialogEvent) DialogAction) {
	if handler == nil {
		p.d.OnDialog(nil)
		return
	}
	p.d.OnDialog(func(ev faketest.DialogEvent) faketest.DialogAction {
		a := handler(DialogEvent{
			Type:          DialogType(ev.Type),
			Message:       ev.Message,
			DefaultPrompt: ev.DefaultPrompt,
		})
		return faketest.DialogAction{Accept: a.Accept, PromptText: a.PromptText}
	})
}

type fakeMouse struct{ d *faketest.Driver }

func (m fakeMouse) MoveTo(x, y float64) error      { return m.d.MoveTo(x, y) }
func (m fakeMouse) ClickAt(x, y float64) error     { return m.d.ClickAt(x, y) }
func (m fakeMouse) Click(sel string) error         { return m.d.Click(sel) }
func (m fakeMouse) FocusElement(sel string) error  { return m.d.FocusElement(sel) }
func (m fakeMouse) RightClick(sel string) error    { return m.d.RightClick(sel) }
func (m fakeMouse) MiddleClick(x, y float64) error { return m.d.MiddleClick(x, y) }
func (m fakeMouse) DoubleClick(sel string) error   { return m.d.DoubleClick(sel) }
func (m fakeMouse) Hover(sel string) error         { return m.d.Hover(sel) }
func (m fakeMouse) Scroll(dx, dy float64) error    { return m.d.Scroll(dx, dy) }
func (m fakeMouse) Drag(fx, fy, tx, ty float64) error {
	return m.d.Drag(fx, fy, tx, ty)
}
func (m fakeMouse) DragAndDrop(fromSel, toSel string) error {
	return m.d.DragAndDrop(fromSel, toSel)
}
func (m fakeMouse) Tap(x, y float64) error { return m.d.Tap(x, y) }

type fakeKeyboard struct{ d *faketest.Driver }

func (k fakeKeyboard) ClearInput(sel ...string) error { return k.d.ClearInput(sel...) }
func (k fakeKeyboard) TypeHuman(text string) error    { return k.d.TypeHuman(text) }
func (k fakeKeyboard) Down(key string) error          { return k.d.Down(key) }
func (k fakeKeyboard) Up(key string) error            { return k.d.Up(key) }
func (k fakeKeyboard) Press(key string) error         { return k.d.Press(key) }
func (k fakeKeyboard) Type(text string) error         { return k.d.Type(text) }
func (k fakeKeyboard) TypeFast(text string) error     { return k.d.TypeFast(text) }
func (k fakeKeyboard) TypeSelector(selector, text string) error {
	return k.d.TypeSelector(selector, text)
}
