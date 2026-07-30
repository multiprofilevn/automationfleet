package bingsearch

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
)

// Well-known Windows install paths, probed only when the env var is unset —
// mirrors the CHROME_PATH / FIREFOX_PATH convention already used by
// automationfleet's own tier-2 stress helpers
// (stress_fleet_real_helpers_test.go: resolveChromeExe / resolveFirefoxExe).
var (
	defaultChromePaths = []string{
		`C:\Program Files\Google\Chrome\Application\chrome.exe`,
		`C:\Program Files (x86)\Google\Chrome\Application\chrome.exe`,
	}
	defaultFirefoxPaths = []string{
		`C:\Program Files\Mozilla Firefox\firefox.exe`,
	}
)

// GPM (multiprofile) keeps its managed browsers under the user's home in a
// version-named directory, e.g.
//
//	~/.multiprofile/resources/browser/gpm_browser/142.0.7444.163/chrome.exe
//	~/.multiprofile/resources/browser/gpm_firefox_browser/FirefoxCore_v149/firefox.exe
//
// The version segment changes whenever GPM updates, so these are globbed
// rather than hardcoded — a pinned version would silently stop resolving after
// the next update and send the reader hunting for a path that no longer exists.
const (
	gpmChromeGlob  = `.multiprofile/resources/browser/gpm_browser/*/chrome.exe`
	gpmFirefoxGlob = `.multiprofile/resources/browser/gpm_firefox_browser/*/firefox.exe`
)

// ResolveChromePath returns the Chrome binary path from CHROME_PATH, else the
// newest GPM-managed build, else the first well-known install path that
// exists, else an error naming the env var to set.
func ResolveChromePath() (string, error) {
	return resolvePath("CHROME_PATH", gpmChromeGlob, defaultChromePaths)
}

// ResolveFirefoxPath is the Firefox counterpart of ResolveChromePath.
func ResolveFirefoxPath() (string, error) {
	return resolvePath("FIREFOX_PATH", gpmFirefoxGlob, defaultFirefoxPaths)
}

func resolvePath(env, gpmGlob string, candidates []string) (string, error) {
	if p := os.Getenv(env); p != "" {
		return p, nil
	}
	if p := newestGPMBrowser(gpmGlob); p != "" {
		return p, nil
	}
	for _, c := range candidates {
		if _, err := os.Stat(c); err == nil {
			return c, nil
		}
	}
	return "", fmt.Errorf("no binary found at the well-known install paths and %s is unset — set %s=<path to the binary> and re-run", env, env)
}

// newestGPMBrowser returns the newest GPM build directory matching glob, or ""
// when none exist. Set CHROME_PATH / FIREFOX_PATH to pin an exact build.
func newestGPMBrowser(glob string) string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	matches, err := filepath.Glob(filepath.Join(home, filepath.FromSlash(glob)))
	if err != nil || len(matches) == 0 {
		return ""
	}
	sort.Slice(matches, func(i, j int) bool {
		return lessVersion(matches[i], matches[j])
	})
	return matches[len(matches)-1]
}

// lessVersion orders two paths by their embedded version, comparing digit runs
// numerically so "7444.9" ranks below "7444.163". A plain string sort gets that
// backwards, which would silently pick an older browser than the one installed.
// Handles both shapes GPM uses: dotted Chrome builds ("142.0.7444.163") and
// Firefox's "FirefoxCore_v149".
func lessVersion(a, b string) bool {
	an, bn := versionChunks(a), versionChunks(b)
	for i := 0; i < len(an) && i < len(bn); i++ {
		if an[i] != bn[i] {
			return an[i] < bn[i]
		}
	}
	if len(an) != len(bn) {
		return len(an) < len(bn)
	}
	return a < b // identical version numbers — fall back to path order
}

// versionChunks extracts every run of digits in s, left to right.
func versionChunks(s string) []int {
	var out []int
	for i := 0; i < len(s); {
		if s[i] < '0' || s[i] > '9' {
			i++
			continue
		}
		n := 0
		for i < len(s) && s[i] >= '0' && s[i] <= '9' {
			n = n*10 + int(s[i]-'0')
			i++
		}
		out = append(out, n)
	}
	return out
}
