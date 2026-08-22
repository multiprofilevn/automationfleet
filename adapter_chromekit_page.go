package automationfleet

import (
	"context"
	"time"

	"github.com/multiprofilevn/chromekit"
)

func (p chromePage) EvalSelector(sel, fnBody string, out any) error {
	return p.p.EvalSelector(sel, fnBody, out)
}

func (p chromePage) EvalSelectorAll(sel, jsFn string, out any) error {
	return p.p.EvalSelectorAll(sel, jsFn, out)
}

func (p chromePage) EvaluateCtx(ctx context.Context, expression string, out any) error {
	return p.p.EvaluateCtx(ctx, expression, out)
}

func (p chromePage) Reload(timeout time.Duration) error    { return p.p.Reload(timeout) }
func (p chromePage) GoBack(timeout time.Duration) error    { return p.p.GoBack(timeout) }
func (p chromePage) GoForward(timeout time.Duration) error { return p.p.GoForward(timeout) }
func (p chromePage) GetURL() (string, error)               { return p.p.GetURL() }
func (p chromePage) GetTitle() (string, error)             { return p.p.GetTitle() }
func (p chromePage) GetOrigin() (string, error)            { return p.p.GetOrigin() }
func (p chromePage) URL() string                           { return p.p.URL() }
func (p chromePage) Title() string                         { return p.p.Title() }

func (p chromePage) GetText(selector string) (string, error) { return p.p.GetText(selector) }
func (p chromePage) GetAttribute(selector, attribute string) (string, error) {
	return p.p.GetAttribute(selector, attribute)
}
func (p chromePage) ScrollIntoView(selector string) error { return p.p.ScrollIntoView(selector) }
func (p chromePage) Focus(selector string) error          { return p.p.Focus(selector) }

func (p chromePage) WaitForReady(timeout time.Duration) error { return p.p.WaitForReady(timeout) }
func (p chromePage) WaitForChangeURL(timeout time.Duration) error {
	return p.p.WaitForChangeURL(timeout)
}
func (p chromePage) WaitForFunction(expr string, timeout time.Duration) error {
	return p.p.WaitForFunction(expr, timeout)
}
func (p chromePage) WaitForSelectorGone(selector string, timeout time.Duration) error {
	return p.p.WaitForSelectorGone(selector, timeout)
}

func (p chromePage) Screenshot() ([]byte, error) { return p.p.Screenshot() }
func (p chromePage) ScreenshotClip(x, y, w, h float64) ([]byte, error) {
	return p.p.ScreenshotClip(x, y, w, h)
}
func (p chromePage) ScreenshotElement(sel string) ([]byte, error) { return p.p.ScreenshotElement(sel) }
func (p chromePage) FullScreenshot(quality int) ([]byte, error)   { return p.p.FullScreenshot(quality) }

func (p chromePage) SetInputFiles(sel string, paths ...string) error {
	return p.p.SetInputFiles(sel, paths...)
}

// OnDialog translates between the fleet's kit-agnostic DialogEvent/DialogAction
// and chromekit's own (identically shaped but distinctly named) types.
func (p chromePage) OnDialog(handler func(DialogEvent) DialogAction) {
	if handler == nil {
		p.p.OnDialog(nil)
		return
	}
	p.p.OnDialog(func(ev chromekit.DialogEvent) chromekit.DialogAction {
		a := handler(DialogEvent{
			Type:          DialogType(ev.Type),
			Message:       ev.Message,
			DefaultPrompt: ev.DefaultPrompt,
		})
		return chromekit.DialogAction{Accept: a.Accept, PromptText: a.PromptText}
	})
}
