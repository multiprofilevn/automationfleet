package bingsearch

import (
	"encoding/json"
	"fmt"
	stdlog "log"
	"strconv"
	"time"

	"github.com/multiprofilevn/automationfleet"
)

// ConsentProbeTimeout bounds how long RunQuery waits to see whether ONE
// candidate in ConsentDialogSelectors showed up before moving on to the next
// candidate (or deciding none are there).
//
// It was 3s, which is plenty for a single browser and too short for 25 sharing
// a machine: the banner's anchor had not hydrated in time on 5 of 25 instances,
// the probe gave up, and the run failed on a candidate that would have appeared
// a moment later. Sized for the contended case, since that is the case these
// examples exist to demonstrate.
const ConsentProbeTimeout = 10 * time.Second

// ConsentDismissTimeout bounds how long RunQuery waits, after clicking a
// matched consent button, for that selector to actually disappear. Clicking
// is not proof of dismissal — a stale/duplicate selector could match an
// element that isn't the real control — so dismissal is verified before the
// caller is allowed to proceed.
const ConsentDismissTimeout = 10 * time.Second

// consentVisibilityJSFmt measures one candidate. It returns a JSON STRING
// rather than an object so the result survives both kits' Evaluate decoding
// identically (BiDi object serialization depth is not something this example
// should be betting on) — the caller unmarshals it.
//
// %s is the selector, injected as a quoted JS string literal.
const consentVisibilityJSFmt = `(() => {
  const el = document.querySelector(%s);
  if (!el) return JSON.stringify({found: false});

  // Scroll into view BEFORE measuring. The click paths themselves scroll
  // (chromekit/firefoxkit ElementCenter both do), so measuring the unscrolled
  // position asks a different question than the one that matters and answers
  // it wrongly. In a tiled 640x360 window the content viewport is only
  // 624x267, and a real run failed with
  //
  //	"#b_results .b_algo h2 a" never became clickable:
  //	click point (292, 771) outside viewport (624x267)
  //
  // for a result that was perfectly clickable one scroll away.
  el.scrollIntoView({block: "center", inline: "center"});

  const r = el.getBoundingClientRect();
  const s = window.getComputedStyle(el);
  const op = parseFloat(s.opacity);
  const cx = (r.left + r.right) / 2, cy = (r.top + r.bottom) / 2;

  // What is ACTUALLY on top at the point we would click. An element can be
  // visible, correctly sized and inside the viewport while a modal, cookie
  // wall or notification overlay sits above it — the click then lands on the
  // overlay. Reporting the topmost element lets the caller refuse rather than
  // click blind; see consentElementMetrics.clickable.
  let occluded = false, topDesc = "";
  if (cx >= 0 && cy >= 0 && cx < window.innerWidth && cy < window.innerHeight) {
    const top = document.elementFromPoint(cx, cy);
    if (!top) {
      occluded = true; topDesc = "nothing at click point";
    } else if (top !== el && !el.contains(top) && !top.contains(el)) {
      occluded = true;
      topDesc = top.tagName.toLowerCase() +
        (top.id ? "#" + top.id : "") +
        (top.className && typeof top.className === "string"
          ? "." + top.className.trim().split(/\s+/).slice(0, 2).join(".") : "");
    }
  }

  return JSON.stringify({
    found: true,
    width: r.width, height: r.height,
    left: r.left, top: r.top, right: r.right, bottom: r.bottom,
    viewportWidth: window.innerWidth, viewportHeight: window.innerHeight,
    display: s.display, visibility: s.visibility,
    opacity: isNaN(op) ? 1 : op,
    occluded: occluded, topElement: topDesc
  });
})()`

// consentElementMetrics is what consentVisibilityJSFmt reports back. All
// geometry is in CSS pixels relative to the viewport, i.e. the same frame the
// drivers use to compute a click point.
type consentElementMetrics struct {
	Found          bool    `json:"found"`
	Width          float64 `json:"width"`
	Height         float64 `json:"height"`
	Left           float64 `json:"left"`
	Top            float64 `json:"top"`
	Right          float64 `json:"right"`
	Bottom         float64 `json:"bottom"`
	ViewportWidth  float64 `json:"viewportWidth"`
	ViewportHeight float64 `json:"viewportHeight"`
	Display        string  `json:"display"`
	Visibility     string  `json:"visibility"`
	Opacity        float64 `json:"opacity"`
	Occluded       bool    `json:"occluded"`
	TopElement     string  `json:"topElement"`
}

// clickable reports whether this candidate can actually be clicked, and if
// not, why — the reason is logged so a skipped candidate is never silent.
//
// WHY THIS GATE EXISTS: dismissConsentIfPresent used to treat WaitForSelector
// as proof of clickability. WaitForSelector only proves the node EXISTS. A
// real 25-window run matched one of the defensive candidates in
// ConsentDialogSelectors against a hidden element and clicked it anyway,
// failing with:
//
//	bidi: move target out of bounds: Move target
//	(-0.17166046823965417, 0.5231548389437894) is out of bounds of viewport
//	dimensions (1280, 940)
//
// Fractional coordinates next to the origin are the signature of a zero/near-
// zero bounding box. Every check below is aimed at that class of element;
// do not "simplify" one away without a run that proves it redundant.
//
// The viewport test is on the element's CENTER rather than on any overlap,
// because the center is exactly the point the drivers aim at — an element
// half off-screen whose center is outside the viewport produces the very
// out-of-bounds error above.
func (m consentElementMetrics) clickable() (bool, string) {
	if !m.Found {
		return false, "no element matched"
	}
	if m.Display == "none" {
		return false, "display:none"
	}
	if m.Visibility == "hidden" || m.Visibility == "collapse" {
		return false, "visibility:" + m.Visibility
	}
	if m.Opacity <= 0 {
		return false, "opacity:0"
	}
	if m.Width <= 0 || m.Height <= 0 {
		return false, fmt.Sprintf("zero-size box (%gx%g)", m.Width, m.Height)
	}
	// A viewport we could not measure is not evidence of anything, so it
	// cannot be grounds for rejection — the geometry checks above already ran.
	if m.ViewportWidth <= 0 || m.ViewportHeight <= 0 {
		return true, ""
	}
	cx, cy := (m.Left+m.Right)/2, (m.Top+m.Bottom)/2
	if cx < 0 || cy < 0 || cx >= m.ViewportWidth || cy >= m.ViewportHeight {
		return false, fmt.Sprintf("click point (%g, %g) outside viewport (%gx%g)",
			cx, cy, m.ViewportWidth, m.ViewportHeight)
	}
	// Something else is on top at the click point. The fleet's WindowFromPoint
	// gate (phase 1) already refuses a click whose OS window is not the target,
	// but an overlay INSIDE the right window shares that window's HWND and
	// sails straight through it — a modal, cookie wall or notification banner
	// covering the element is exactly that case. Clicking anyway means
	// dismissing, accepting or navigating something nobody asked for.
	if m.Occluded {
		where := m.TopElement
		if where == "" {
			where = "unknown element"
		}
		return false, fmt.Sprintf("click point (%g, %g) is covered by %s", cx, cy, where)
	}
	return true, ""
}

// measureConsentCandidate runs the visibility probe for one selector.
func measureConsentCandidate(page PageReader, sel string) (consentElementMetrics, error) {
	var raw string
	expr := fmt.Sprintf(consentVisibilityJSFmt, strconv.Quote(sel))
	if err := page.Evaluate(expr, &raw); err != nil {
		return consentElementMetrics{}, fmt.Errorf("evaluate: %w", err)
	}
	var m consentElementMetrics
	if err := json.Unmarshal([]byte(raw), &m); err != nil {
		return consentElementMetrics{}, fmt.Errorf("decode metrics %q: %w", raw, err)
	}
	return m, nil
}

// dismissConsentIfPresent probes ConsentDialogSelectors in order, clicks the
// first candidate that both matches AND is genuinely clickable, and confirms
// it actually disappeared before returning. It accepts EITHER an accept or a
// reject match — dismissing the interstitial is the goal, not which choice
// the user "made".
//
// A candidate that exists but fails the visibility gate (see
// consentElementMetrics.clickable) is SKIPPED, not clicked, and the probe
// moves on to the next candidate. Same for a candidate whose probe could not
// be run at all: not being able to prove it is clickable is not permission to
// click it.
//
// If a candidate is clicked but never disappears, that's a stale/duplicate
// selector or an interstitial this example doesn't know how to drive —
// returning nil and continuing would repeat the exact failure this function
// exists to fix (25 windows silently stuck on a blocked page), so it fails
// the instance instead, naming the selector that wouldn't go away. No
// candidate matching at all is NOT an error — the interstitial is
// locale/session-dependent and often just isn't shown.
func dismissConsentIfPresent(fleet *automationfleet.Fleet, id string, page PageReader) error {
	for _, sel := range ConsentDialogSelectors {
		if err := page.WaitForSelector(sel, ConsentProbeTimeout); err != nil {
			continue // this candidate isn't present — try the next one
		}
		m, err := measureConsentCandidate(page, sel)
		if err != nil {
			stdlog.Printf("[%s] consent candidate %q: visibility probe failed (%v) — skipping", id, sel, err)
			continue
		}
		if ok, why := m.clickable(); !ok {
			stdlog.Printf("[%s] consent candidate %q exists but is not clickable (%s) — skipping", id, sel, why)
			continue
		}
		if err := submitWait(fleet, id, automationfleet.ClickAction{Selector: sel}); err != nil {
			return fmt.Errorf("click consent control %q: %w", sel, err)
		}
		if err := waitConsentDismissed(page, sel); err != nil {
			return fmt.Errorf("consent dialog %q not dismissed within %s: %w", sel, ConsentDismissTimeout, err)
		}
		return nil
	}
	return nil
}

// consentDismissPollInterval paces waitConsentDismissed. Short enough that a
// normal dismissal costs one or two polls, long enough not to spin.
const consentDismissPollInterval = 200 * time.Millisecond

// waitConsentDismissed waits until the clicked control stops being clickable.
//
// It deliberately does NOT use WaitForSelectorGone, and that is the whole
// point of this function. Measured against live Bing on Firefox, clicking
// Accept leaves the banner in the DOM with display:block untouched — only its
// height collapses:
//
//	before click: {containerInDom:true, display:"block", height:244}
//	after  click: {containerInDom:true, display:"block", height:0}
//
// So "gone from the DOM" never happens and a display-based check never fires
// either; 25/25 instances failed with "still present 5s after click" while the
// banner was, visually, already dismissed. Size is the only honest signal, and
// clickable() already measures exactly that — reusing it keeps one definition
// of "this control is interactable" instead of two that can drift apart.
func waitConsentDismissed(page PageReader, sel string) error {
	deadline := time.Now().Add(ConsentDismissTimeout)
	var last string
	for {
		m, err := measureConsentCandidate(page, sel)
		if err != nil {
			// Probing failed (element detached mid-poll is the common case).
			// Treat as dismissed: there is nothing left to interact with.
			return nil
		}
		ok, why := m.clickable()
		if !ok {
			return nil
		}
		last = why
		if time.Now().After(deadline) {
			if last == "" {
				last = "still clickable"
			}
			return fmt.Errorf("control remains interactable (%s)", last)
		}
		time.Sleep(consentDismissPollInterval)
	}
}

// WaitElementClickable blocks until sel is not merely present but actually
// interactable: non-zero box, not hidden, click point inside the viewport.
//
// This generalises the gate consent dismissal already needed, because waiting
// on mere presence and then acting turned out to be a pattern rather than a
// one-off. The consent banner hit it first; the search box hit it next, failing
// a real run with:
//
//	move target out of bounds: Move target (0.53, -0.19) is out of bounds of
//	viewport dimensions (1840, 940)
//
// WaitAction had proved #sb_form_q existed while it was still a degenerate
// sub-pixel box. Anything this example is about to click or type into goes
// through here first.
func WaitElementClickable(page PageReader, sel string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	var last string
	for {
		m, err := measureConsentCandidate(page, sel)
		if err != nil {
			last = err.Error()
		} else if ok, why := m.clickable(); ok {
			return nil
		} else {
			last = why
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("%q never became clickable within %s (%s)", sel, timeout, last)
		}
		time.Sleep(consentDismissPollInterval)
	}
}
