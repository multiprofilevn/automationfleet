package bingsearch

import (
	"path/filepath"
	"sort"
	"testing"
)

// TestLessVersion_NumericNotLexicographic pins the reason lessVersion exists:
// a plain string sort ranks "7444.9" above "7444.163" and would hand back an
// older browser than the newest one installed.
func TestLessVersion_NumericNotLexicographic(t *testing.T) {
	cases := []struct {
		lo, hi string // lo must sort below hi
	}{
		{`gpm_browser/142.0.7444.9/chrome.exe`, `gpm_browser/142.0.7444.163/chrome.exe`},
		{`gpm_browser/129.0.6533.73/chrome.exe`, `gpm_browser/129.0.6668.59/chrome.exe`},
		{`gpm_browser/139.0.7258.139/chrome.exe`, `gpm_browser/142.0.7444.163/chrome.exe`},
		{`gpm_browser/99.0.1.1/chrome.exe`, `gpm_browser/100.0.1.1/chrome.exe`},
		{`gpm_firefox_browser/FirefoxCore_v145/firefox.exe`, `gpm_firefox_browser/FirefoxCore_v149/firefox.exe`},
		{`gpm_firefox_browser/FirefoxCore_v99/firefox.exe`, `gpm_firefox_browser/FirefoxCore_v149/firefox.exe`},
	}
	for _, c := range cases {
		if !lessVersion(c.lo, c.hi) {
			t.Errorf("lessVersion(%q, %q) = false, want true", c.lo, c.hi)
		}
		if lessVersion(c.hi, c.lo) {
			t.Errorf("lessVersion(%q, %q) = true, want false (not symmetric)", c.hi, c.lo)
		}
	}
}

// TestNewestPick reproduces the builds actually installed on the dev machine
// and asserts the newest one wins.
func TestNewestPick(t *testing.T) {
	for _, tc := range []struct {
		name  string
		paths []string
		want  string
	}{
		{
			name: "chrome",
			paths: []string{
				`124.0.6367.29`, `127.0.6533.73`, `129.0.6533.73`, `129.0.6668.59`,
				`132.0.6834.84`, `134.0.6998.89`, `135.0.7049.42`, `137.0.7151.41`,
				`139.0.7258.139`, `142.0.7444.163`,
			},
			want: `142.0.7444.163`,
		},
		{
			name:  "firefox",
			paths: []string{`FirefoxCore_v145`, `FirefoxCore_v149`},
			want:  `FirefoxCore_v149`,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := append([]string(nil), tc.paths...)
			sort.Slice(got, func(i, j int) bool { return lessVersion(got[i], got[j]) })
			if newest := got[len(got)-1]; newest != tc.want {
				t.Errorf("newest = %q, want %q", newest, tc.want)
			}
		})
	}
}

// TestVersionChunks covers the shapes GPM produces plus the degenerate input.
func TestVersionChunks(t *testing.T) {
	for _, tc := range []struct {
		in   string
		want []int
	}{
		{`142.0.7444.163`, []int{142, 0, 7444, 163}},
		{`FirefoxCore_v149`, []int{149}},
		{`no-digits-here`, nil},
		{``, nil},
	} {
		got := versionChunks(tc.in)
		if len(got) != len(tc.want) {
			t.Errorf("versionChunks(%q) = %v, want %v", tc.in, got, tc.want)
			continue
		}
		for i := range got {
			if got[i] != tc.want[i] {
				t.Errorf("versionChunks(%q) = %v, want %v", tc.in, got, tc.want)
				break
			}
		}
	}
}

// TestNewestGPMBrowser_NoMatchIsEmpty keeps the fallback chain intact: an
// unresolvable glob must return "" so resolvePath moves on to the well-known
// install paths rather than handing back a bogus path.
func TestNewestGPMBrowser_NoMatchIsEmpty(t *testing.T) {
	if got := newestGPMBrowser(filepath.ToSlash(`this/path/does/not/exist/*/nope.exe`)); got != "" {
		t.Errorf("newestGPMBrowser(missing) = %q, want empty", got)
	}
}
