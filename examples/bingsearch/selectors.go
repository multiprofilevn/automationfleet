package bingsearch

// Bing selectors. bing.com's markup changes over redesigns — these were
// picked for how long they've stayed put historically, not because they are
// guaranteed to keep working. Re-check them (browser devtools) before
// relying on this example after any gap in usage.
const (
	// BingURL is the search homepage every instance starts from.
	BingURL = "https://www.bing.com"

	// SearchBoxSelector is Bing's main search input. #sb_form_q has been the
	// id of the search box since the "sb_form" redesign and has survived
	// several visual refreshes since.
	SearchBoxSelector = "#sb_form_q"

	// The magnifier "go" button (#sb_form_go) used to be clicked here, chosen
	// so the whole script stayed expressible with the fleet's 5 Action types.
	// A real 25-window run failed every instance on it with
	// `zero-size element "#sb_form_go" (rect=0x0)` — the button is in the DOM
	// but has no rendered box, so there is nothing to click. Submitting with
	// Enter instead removes the dependency on that button entirely; see
	// submitSearch in search.go.

	// FirstResultLinkSelector targets the anchor inside the first organic
	// result's heading: #b_results is the results list container, .b_algo
	// is each organic result's wrapping <li>.
	FirstResultLinkSelector = "#b_results .b_algo h2 a"
)

// ConsentDialogSelectors dismisses Bing's cookie-consent interstitial ("bnp"
// = the Bing notification-panel widget).
//
// These are MEASURED, not guessed. A live Firefox against bing.com dumped the
// container's markup:
//
//	<div id="bnp_btn_accept" class="bnp_btn_accept">
//	  <a href="javascript: void(0)" h="ID=Bnp,5007.1">Accept</a>
//	</div>
//	<div id="bnp_btn_reject" class="bnp_btn_reject">
//	  <a href="javascript: void(0)" h="ID=Bnp,5008.1">Reject</a>
//	</div>
//
// Two things that cost a full 25-window run each are encoded here:
//
//  1. #bnp_btn_accept is the WRAPPER div; the control that carries the
//     handler is the <a> inside it. Clicking the div does nothing — the banner
//     stays up. The bare div ids were kept as a "fallback" for one run and made
//     things worse rather than safer: on 5 of 25 instances the anchor had not
//     hydrated within the probe window, so the code fell through to the
//     wrapper, clicked it, and failed. A fallback that cannot work is not a
//     safety net, it is a second failure mode. Only the anchors are listed.
//  2. The same probe found NO <button>, <a role=button> or <input> anywhere
//     on the page matching accept/reject/consent text. Earlier guesses at
//     button[aria-label='Accept all'] and friends matched nothing — they are
//     removed rather than left in to look thorough.
//
// dismissConsentIfPresent (consent.go) tries these in order and ACCEPTS
// EITHER accept or reject — the goal is dismissing the interstitial, not
// consenting one way or the other. Each candidate must also pass a visibility
// gate before being clicked (an earlier run clicked a hidden, sub-pixel
// element and the driver rejected it as an out-of-bounds move — see
// consentElementMetrics.clickable), and the selector is verified gone
// afterwards.
//
// Re-run the probe if Bing's markup shifts; do not re-guess.
var ConsentDialogSelectors = []string{
	"#bnp_btn_accept a",
	"#bnp_btn_reject a",
}
