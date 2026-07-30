// Package bingsearch holds the code shared by the four bing_* examples
// (bing_chrome_native, bing_firefox_native, bing_chrome_remote,
// bing_firefox_remote): browser-path resolution (CHROME_PATH / FIREFOX_PATH,
// same convention as stress_fleet_real_helpers_test.go), the native-input
// gate (FLEET_ALLOW_REAL_INPUT), Bing's selectors, and the search-and-click
// script itself.
//
// Each example only differs in HOW it launches and registers its browsers
// (native vs remote, chrome vs firefox, 9 vs 25 instances) — the workflow it
// drives against Bing is this package's RunQuery, so the four mains stay
// small and the actual "what does the script do" logic lives in one place.
package bingsearch
