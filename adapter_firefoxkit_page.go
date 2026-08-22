package automationfleet

import (
	"context"
	"time"

	"github.com/multiprofilevn/firefoxkit"
)

func (p firefoxPage) EvalSelector(sel, fnBody string, out any) error {
	return p.p.EvalSelector(sel, fnBody, out)
}

func (p firefoxPage) EvalSelectorAll(sel, jsFn string, out any) error {
	return p.p.EvalSelectorAll(sel, jsFn, out)
}

func (p firefoxPage) EvaluateCtx(ctx context.Context, expression string, out any) error {
	return p.p.EvaluateCtx(ctx, expression, out)
}

func (p firefoxPage) Reload(timeout time.Duration) error    { return p.p.Reload(timeout) }
func (p firefoxPage) GoBack(timeout time.Duration) error    { return p.p.GoBack(timeout) }
func (p firefoxPage) GoForward(timeout time.Duration) error { return p.p.GoForward(timeout) }
func (p firefoxPage) GetURL() (string, error)               { return p.p.GetURL() }
func (p firefoxPage) GetTitle() (string, error)             { return p.p.GetTitle() }
func (p firefoxPage) GetOrigin() (string, error)            { return p.p.GetOrigin() }
func (p firefoxPage) URL() string                           { return p.p.URL() }
func (p firefoxPage) Title() string                         { return p.p.Title() }

func (p firefoxPage) GetText(selector string) (string, error) { return p.p.GetText(selector) }
func (p firefoxPage) GetAttribute(selector, attribute string) (string, error) {
	return p.p.GetAttribute(selector, attribute)
}
func (p firefoxPage) ScrollIntoView(selector string) error { return p.p.ScrollIntoView(selector) }
func (p firefoxPage) Focus(selector string) error          { return p.p.Focus(selector) }

func (p firefoxPage) WaitForReady(timeout time.Duration) error { return p.p.WaitForReady(timeout) }
func (p firefoxPage) WaitForChangeURL(timeout time.Duration) error {
	return p.p.WaitForChangeURL(timeout)
}
func (p firefoxPage) WaitForFunction(expr string, timeout time.Duration) error {
	return p.p.WaitForFunction(expr, timeout)
}
func (p firefoxPage) WaitForSelectorGone(selector string, timeout time.Duration) error {
	return p.p.WaitForSelectorGone(selector, timeout)
}

func (p firefoxPage) Screenshot() ([]byte, error) { return p.p.Screenshot() }
func (p firefoxPage) ScreenshotClip(x, y, w, h float64) ([]byte, error) {
	return p.p.ScreenshotClip(x, y, w, h)
}
func (p firefoxPage) ScreenshotElement(sel string) ([]byte, error) {
	return p.p.ScreenshotElement(sel)
}
func (p firefoxPage) FullScreenshot(quality int) ([]byte, error) { return p.p.FullScreenshot(quality) }

func (p firefoxPage) SetInputFiles(sel string, paths ...string) error {
	return p.p.SetInputFiles(sel, paths...)
}

// OnDialog translates between the fleet's kit-agnostic DialogEvent/DialogAction
// and firefoxkit's own (identically shaped but distinctly named) types.
func (p firefoxPage) OnDialog(handler func(DialogEvent) DialogAction) {
	if handler == nil {
		p.p.OnDialog(nil)
		return
	}
	p.p.OnDialog(func(ev firefoxkit.DialogEvent) firefoxkit.DialogAction {
		a := handler(DialogEvent{
			Type:          DialogType(ev.Type),
			Message:       ev.Message,
			DefaultPrompt: ev.DefaultPrompt,
		})
		return firefoxkit.DialogAction{Accept: a.Accept, PromptText: a.PromptText}
	})
}
