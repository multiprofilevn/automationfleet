package bingsearch

import (
	"fmt"
	stdlog "log"
	"time"

	"github.com/tuwibu/automationfleet"
)

// PageReader is the subset of *chromekit.Page / *firefoxkit.Page that
// RunQuery reads directly — outside the fleet's job queue — to decide what
// to do next (e.g. "did the consent banner show up", "did the page settle
// after the click"). None of these calls touch the mouse or keyboard, so
// they're safe to run whether the handle is native or remote. Both kits'
// Page type satisfy this structurally; no adapter needed.
type PageReader interface {
	WaitForReady(timeout time.Duration) error
	WaitForSelector(selector string, timeout time.Duration) error
	WaitForSelectorGone(selector string, timeout time.Duration) error
	WaitForChangeURL(timeout time.Duration) error
	Evaluate(expr string, out any) error
}

// stripResultTargetsJS removes target="_blank" from every result link so the
// click navigates the CURRENT tab. Without it a real run clicked the first
// result successfully and then failed 25/25 on `wait navigation after result
// click: timeout` — Bing opens results in a new tab, so the original page's
// URL never changed and there was nothing for WaitForChangeURL to see.
//
// Returns the number of links it altered, which the caller logs: zero means
// Bing changed its markup (or stopped using target) and the wait below is
// resting on an assumption that no longer holds.
const stripResultTargetsJS = `(() => {
  const links = document.querySelectorAll('#b_results a[target]');
  links.forEach(a => a.removeAttribute('target'));
  return links.length;
})()`

// RunQuery drives one browser through the shared workflow: navigate to
// Bing, dismiss the consent interstitial if present, search "anime {n}",
// and click the first organic result.
//
// id is both the fleet.Submit BrowserID and the label used in every wrapped
// error, so a failure names which instance and which step it happened at.
// n is 1-based (instance 0 in the caller's loop searches "anime 1").
// submitSearch presses Enter in the focused search box. It is a callback
// rather than a method on PageReader because both kits spell it
// Press(key string, mods ...input.Modifier) with their OWN input.Modifier
// type, so no structural interface can cover the variadic. Each example
// builds it from the concrete browser it already holds.
//
// Enter replaced clicking Bing's magnifier button after a real 25-window run
// failed every instance with `zero-size element "#sb_form_go" (rect=0x0)`:
// the button is in the DOM but has no rendered box, so there was nothing to
// click. Pressing Enter is also what a person does.
type submitSearch func() error

func RunQuery(fleet *automationfleet.Fleet, id string, page PageReader, submit submitSearch, n int, timeout time.Duration) error {
	query := fmt.Sprintf("anime %d", n)

	if err := submitWait(fleet, id, automationfleet.NavigateAction{URL: BingURL, Timeout: timeout}); err != nil {
		return fmt.Errorf("[%s] navigate to bing: %w", id, err)
	}
	if err := page.WaitForReady(timeout); err != nil {
		return fmt.Errorf("[%s] wait page ready after navigate: %w", id, err)
	}

	// Consent/cookie interstitial — locale- and session-dependent, so this
	// probes for it rather than assuming it's always there; see
	// dismissConsentIfPresent for why it can also fail the instance outright.
	if err := dismissConsentIfPresent(fleet, id, page); err != nil {
		return fmt.Errorf("[%s] dismiss consent interstitial: %w", id, err)
	}

	// Confirm the search box exists before TypeAction tries to query its
	// bounding box. TypeAction itself clicks the element first (native:
	// clickAt; remote: FocusElement), so it never types into an unfocused
	// field — this WaitAction only guards against the box not being there
	// yet at all.
	if err := submitWait(fleet, id, automationfleet.WaitAction{Selector: SearchBoxSelector, Timeout: timeout}); err != nil {
		return fmt.Errorf("[%s] wait for search box: %w", id, err)
	}
	// Presence is not enough — see WaitElementClickable. TypeAction clicks the
	// box before typing, and a box that exists but has not been laid out yet
	// yields a sub-pixel click point the driver rejects.
	if err := WaitElementClickable(page, SearchBoxSelector, timeout); err != nil {
		return fmt.Errorf("[%s] search box never became clickable: %w", id, err)
	}
	if err := submitWait(fleet, id, automationfleet.TypeAction{Selector: SearchBoxSelector, Text: query, ClearFirst: true}); err != nil {
		return fmt.Errorf("[%s] type query %q: %w", id, query, err)
	}
	// TypeAction already focused the box (native: clickAt; remote:
	// FocusElement), so Enter submits the form from wherever the caret is.
	if err := submit(); err != nil {
		return fmt.Errorf("[%s] submit search (Enter): %w", id, err)
	}

	// The results page can take a moment to settle after submit — wait for
	// the first result link rather than assuming it's already there.
	if err := submitWait(fleet, id, automationfleet.WaitAction{Selector: FirstResultLinkSelector, Timeout: timeout}); err != nil {
		return fmt.Errorf("[%s] wait for results (query %q): %w", id, query, err)
	}
	// Force same-tab navigation before clicking — see stripResultTargetsJS.
	var stripped int
	if err := page.Evaluate(stripResultTargetsJS, &stripped); err != nil {
		return fmt.Errorf("[%s] strip result link targets: %w", id, err)
	}
	if stripped == 0 {
		stdlog.Printf("[%s] warning: no result link carried target= — if the "+
			"navigation wait below times out, Bing's markup has moved and "+
			"stripResultTargetsJS needs re-checking", id)
	}

	if err := WaitElementClickable(page, FirstResultLinkSelector, timeout); err != nil {
		return fmt.Errorf("[%s] first result never became clickable: %w", id, err)
	}
	if err := submitWait(fleet, id, automationfleet.ClickAction{Selector: FirstResultLinkSelector}); err != nil {
		return fmt.Errorf("[%s] click first organic result: %w", id, err)
	}
	if err := page.WaitForChangeURL(timeout); err != nil {
		return fmt.Errorf("[%s] wait navigation after result click: %w", id, err)
	}
	return nil
}

func submitWait(fleet *automationfleet.Fleet, id string, action automationfleet.Action) error {
	ch, err := fleet.Submit(automationfleet.Job{BrowserID: id, Action: action, Priority: 5})
	if err != nil {
		return fmt.Errorf("submit: %w", err)
	}
	r := <-ch
	if r.Status != automationfleet.StatusDone {
		return fmt.Errorf("status=%s err=%v", r.Status, r.Err)
	}
	return nil
}
