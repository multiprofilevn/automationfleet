package automationfleet

import (
	"context"
	"time"
)

// DialogType enumerates the JS dialog kinds both kits report. Underlying
// string values match chromekit.DialogType / firefoxkit.DialogType exactly
// (verified by both kits' own parity tests) — adapters convert by simple
// string cast, no lookup table needed.
type DialogType string

const (
	DialogAlert        DialogType = "alert"
	DialogConfirm      DialogType = "confirm"
	DialogPrompt       DialogType = "prompt"
	DialogBeforeUnload DialogType = "beforeunload"
)

// DialogEvent describes a JS dialog the page just opened. Fleet-level copy of
// chromekit.DialogEvent / firefoxkit.DialogEvent — the two kit types are
// distinct named types with an identical field shape, so OnDialog can't take
// either one directly without leaking a kit type into Driver; adapters
// translate field-by-field instead.
type DialogEvent struct {
	Type          DialogType
	Message       string
	DefaultPrompt string // only meaningful for DialogPrompt
}

// DialogAction tells the kit how to dismiss the dialog. Zero value
// (Accept=false) dismisses/cancels.
type DialogAction struct {
	Accept     bool
	PromptText string // only used when Accept=true on a prompt dialog
}

// Page is the subset of a kit Page the dispatcher drives. Widened selectively
// per phase 8: navigation, element query/wait, eval, screenshot, upload, and
// dialog handling. Deliberately NOT wrapped: Cookies, Downloads, Storage,
// WebExtension (fleet doesn't coordinate any of that state — see
// unwrappedPageMethods for the full accounting, one reason per method).
type Page interface {
	Mouse() Mouse
	Keyboard() Keyboard

	// -- eval --
	Evaluate(expr string, out any) error
	// EvalSelector evaluates fnBody with the element matching sel bound as the
	// function's argument.
	EvalSelector(sel, fnBody string, out any) error
	// EvalSelectorAll is the []element variant of EvalSelector.
	EvalSelectorAll(sel, jsFn string, out any) error
	// EvaluateCtx is the ctx-cancelable variant of Evaluate.
	EvaluateCtx(ctx context.Context, expression string, out any) error

	// -- navigation --
	Navigate(url string, timeout time.Duration) error
	Reload(timeout time.Duration) error
	GoBack(timeout time.Duration) error
	GoForward(timeout time.Duration) error
	GetURL() (string, error)
	GetTitle() (string, error)
	GetOrigin() (string, error)
	URL() string
	Title() string

	// -- query --
	GetText(selector string) (string, error)
	GetAttribute(selector, attribute string) (string, error)
	ScrollIntoView(selector string) error
	Focus(selector string) error
	// WaitForSelector blocks until the element appears (or timeout). Abstracts
	// the kits' differing QuerySelector return types (*cdp.Node vs *ElementHandle).
	WaitForSelector(selector string, timeout time.Duration) error
	// ElementCenter scrolls the element into view, re-queries its box, and
	// returns the center in CSS pixels (viewport-relative). Adapters hide the
	// per-kit scrollIntoView + bounding-box dance.
	ElementCenter(selector string) (x, y float64, err error)
	// ReadValue returns the current value of an input-like element, read over
	// the protocol. Reading never needs the keyboard, which is what makes it
	// safe to call after foreground loss — it is the ground truth the
	// verify-then-repair path compares against, so a blind retype (and the
	// duplicated content it causes) is never needed.
	ReadValue(selector string) (string, error)

	// -- wait --
	WaitForReady(timeout time.Duration) error
	WaitForChangeURL(timeout time.Duration) error
	WaitForFunction(expr string, timeout time.Duration) error
	WaitForSelectorGone(selector string, timeout time.Duration) error

	// -- screenshot --
	Screenshot() ([]byte, error)
	ScreenshotClip(x, y, w, h float64) ([]byte, error)
	ScreenshotElement(sel string) ([]byte, error)
	FullScreenshot(quality int) ([]byte, error)

	// -- upload --
	SetInputFiles(sel string, paths ...string) error

	// -- dialog --
	// OnDialog registers a handler for every alert/confirm/prompt the page
	// raises. Wrapped (unlike OnConsole) because an unhandled dialog can hang
	// a page indefinitely — that is an operational risk the fleet must be able
	// to close, not a diagnostic nicety.
	OnDialog(handler func(DialogEvent) DialogAction)
}

// pageUnwrappedMethod documents one chromekit/firefoxkit Page method the
// fleet's Page interface deliberately does not wrap, with the reason it was
// left out. TestConformance_PageSubsetDeclared asserts this list plus the
// wrapped set above exactly accounts for every exported method on both kits'
// Page type — wrapping too much or declaring a stale/nonexistent entry here
// both fail the test, same as wrapping too little.
type pageUnwrappedMethod struct {
	method string
	reason string
}

var unwrappedPageMethods = []pageUnwrappedMethod{
	// Frame-scoped — kit-specific *Frame type, out of the seven wrapped
	// categories (nav/query/wait/eval/screenshot/upload/dialog).
	{"AllFrames", "returns a kit-specific *Frame slice; frame-scoped callers use the kit's Browser/Page directly instead of through Driver"},
	{"Frames", "returns a kit-specific *Frame slice with a shape that already diverges between kits (see parity_test signatureExemptions); not one of the seven wrapped categories"},
	{"MainFrame", "returns a kit-specific *Frame; chromekit's version can fail (CDP round-trip) while firefoxkit's cannot, a divergence Driver doesn't paper over"},
	{"FrameByURL", "returns a kit-specific *Frame; same divergence as MainFrame/Frames"},
	{"FrameByName", "returns a kit-specific *Frame; same divergence as MainFrame/Frames"},

	// Explicitly out of scope per the phase spec.
	{"Cookies", "cookie-jar state isn't coordinated by the fleet dispatcher; callers needing cookies use the kit's own CookiesAPI directly"},
	{"Downloads", "download tracking isn't coordinated by the fleet dispatcher; callers use the kit's own DownloadsAPI directly"},
	{"LocalStorage", "storage state is explicitly out of scope for the fleet; callers use the kit directly"},
	{"SessionStorage", "storage state is explicitly out of scope for the fleet; callers use the kit directly"},

	// Page identity/lifecycle — owned by the kit's Browser, not by Driver
	// (Driver wraps an already-current page).
	{"ID", "internal page identity string; the fleet's own handle ID is the identity callers use, not the kit's page ID"},
	{"Ctx", "leaks the kit's internal page context; Driver methods already take or scope their own context, and exposing the raw one invites bypassing the fleet's lifecycle"},
	{"Index", "tab-position bookkeeping the kit's Browser owns; the fleet doesn't reorder tabs"},
	{"Close", "page lifecycle (open/close) belongs to the kit's Browser, already driven via WrapChrome/WrapFirefox; Driver wraps an already-current page, not its lifecycle"},
	{"BringToFront", "tab-focus bookkeeping the kit's Browser owns; Driver.Focus already gives the fleet OS-level foreground control, which is the operation the dispatcher actually needs"},

	// Page-level shortcuts that duplicate a Mouse/Keyboard primitive under a
	// second name — wrapping both doubles the interface for the same capability.
	{"Click", "duplicate shortcut for Mouse().Click(selector) — wrapping both doubles the interface for the same primitive"},
	{"ClickElement", "duplicate shortcut for Mouse().Click(selector) under a second name — same primitive, doubled"},
	{"Type", "duplicate shortcut for Keyboard().Type/TypeHuman — same primitive already reachable via Page.Keyboard()"},
	{"ClickAt", "duplicate shortcut for Mouse().ClickAt(x, y) — same primitive already reachable via Page.Mouse()"},
	{"HoverAt", "duplicate shortcut for a coordinate-based Mouse hover; no fleet caller needs a Page-level alias for a Mouse primitive"},
	{"Hover", "duplicate shortcut for Mouse().Hover(selector) — same primitive already reachable via Page.Mouse()"},
	{"Wait", "plain time.Sleep wrapper; the fleet already times operations via context deadlines, wrapping a sleep adds no capability"},

	// Node-handle-returning query methods — kit-specific return type, already
	// abstracted by ElementCenter/WaitForSelector for what the fleet needs.
	{"QuerySelector", "returns a kit-specific node handle (*cdp.Node vs *ElementHandle, exempted in parity_test signatureExemptions); ElementCenter and WaitForSelector already cover the fleet's element-location needs without exposing the divergent type"},
	{"QuerySelectorAll", "same divergent-node-handle reasoning as QuerySelector, just the []T variant"},
	{"BoundingBox", "takes the same divergent node/selector input QuerySelector produces; ElementCenter already returns the one derived value (center point) the fleet needs from it"},

	// Console listener — no operational requirement, unlike dialogs.
	{"OnConsole", "console-event listener isn't an operational requirement for the fleet — OnDialog is wrapped because an unhandled JS dialog can hang a page, console output carries no such correctness risk"},

	// Narrow/experimental query.
	{"FirstIframeRectPierce", "iframe-piercing geometry probe is a narrow, page-specific query outside the seven wrapped categories; frame-aware callers use the kit directly"},

	// Form-value / dropdown mutation — action verbs outside the seven
	// categories; Mouse+Keyboard already cover the fleet's input actions.
	{"SetValue", "form-value mutation is an action verb, not a read (query) or one of the other six categories; Keyboard().TypeHuman + Mouse().Click already cover the fleet's input actions without a third, redundant path"},
	{"CheckBox", "same action-verb reasoning as SetValue — mutates page state outside the seven wrapped categories"},
	{"SetSelectValue", "dropdown-selection helper is an action verb outside the seven wrapped categories; no current fleet caller drives a <select> element, and Mouse/Keyboard already cover the automation surface those callers would use"},
	{"SelectByValue", "same dropdown-selection reasoning as SetSelectValue"},
	{"SelectByIndex", "same dropdown-selection reasoning as SetSelectValue"},
	{"SelectByText", "same dropdown-selection reasoning as SetSelectValue"},

	// Network configuration.
	{"SetExtraHeaders", "network/request-header configuration; the fleet doesn't manage request headers per page, and this isn't a nav/query/wait/eval/screenshot/upload/dialog verb"},

	// Redundant ctx-cancelable read variants — GetURL/GetTitle already cover
	// the read, and no fleet caller needs external cancellation for it.
	{"URLLive", "ctx-cancelable variant of GetURL; the fleet reads through the page's own context uniformly and has no caller needing external cancellation for a URL read"},
	{"TitleLive", "ctx-cancelable variant of GetTitle; same reasoning as URLLive"},
}
