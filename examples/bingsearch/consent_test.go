package bingsearch

import (
	"encoding/json"
	"strings"
	"testing"
)

// visible is a candidate that should always pass the gate: a normal 120x40
// button sitting near the top-left of a 1280x940 viewport.
func visible() consentElementMetrics {
	return consentElementMetrics{
		Found: true, Width: 120, Height: 40,
		Left: 100, Top: 200, Right: 220, Bottom: 240,
		ViewportWidth: 1280, ViewportHeight: 940,
		Display: "block", Visibility: "visible", Opacity: 1,
	}
}

func TestConsentClickable(t *testing.T) {
	// The measured failure: click point (-0.17, 0.52) against a
	// 1280x940 viewport. Reconstructed here so a regression that stops
	// rejecting it fails this test by name.
	measuredFailure := consentElementMetrics{
		Found: true,
		// A box whose center lands just left of the origin.
		Width: 0.34, Height: 1.05,
		Left: -0.34, Top: 0, Right: 0, Bottom: 1.05,
		ViewportWidth: 1280, ViewportHeight: 940,
		Display: "block", Visibility: "visible", Opacity: 1,
	}

	cases := []struct {
		name    string
		m       consentElementMetrics
		wantOK  bool
		wantWhy string // substring
	}{
		{"plain visible button", visible(), true, ""},
		{"no element matched", consentElementMetrics{Found: false}, false, "no element matched"},
		{"display none", withDisplay(visible(), "none"), false, "display:none"},
		{"visibility hidden", withVisibility(visible(), "hidden"), false, "visibility:hidden"},
		{"visibility collapse", withVisibility(visible(), "collapse"), false, "visibility:collapse"},
		{"opacity zero", withOpacity(visible(), 0), false, "opacity:0"},
		{"negative opacity", withOpacity(visible(), -1), false, "opacity:0"},
		{"zero width", withBox(visible(), 0, 40), false, "zero-size"},
		{"zero height", withBox(visible(), 120, 0), false, "zero-size"},
		// The center check — not the zero-size check — is what catches the
		// real failure: the box has non-zero (sub-pixel) width, so only the
		// click point being left of the origin rejects it.
		{"measured out-of-bounds failure", measuredFailure, false, "outside viewport"},
		{"center left of viewport", atCenter(visible(), -5, 400), false, "outside viewport"},
		{"center above viewport", atCenter(visible(), 400, -5), false, "outside viewport"},
		{"center right of viewport", atCenter(visible(), 1280, 400), false, "outside viewport"},
		{"center below viewport", atCenter(visible(), 400, 940), false, "outside viewport"},
		{"center on last in-bounds pixel", atCenter(visible(), 1279, 939), true, ""},
		{"unmeasurable viewport is not a rejection", withViewport(visible(), 0, 0), true, ""},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ok, why := tc.m.clickable()
			if ok != tc.wantOK {
				t.Fatalf("clickable() ok = %v (why=%q), want %v", ok, why, tc.wantOK)
			}
			if tc.wantWhy == "" {
				if why != "" {
					t.Fatalf("clickable() why = %q, want empty", why)
				}
				return
			}
			if !strings.Contains(why, tc.wantWhy) {
				t.Fatalf("clickable() why = %q, want it to contain %q", why, tc.wantWhy)
			}
		})
	}
}

// The gate is only as good as the JSON contract between the JS probe and the
// Go struct — a renamed field would silently decode as a zero value and
// reject every candidate.
func TestConsentElementMetricsDecodesProbeShape(t *testing.T) {
	raw := `{"found":true,"width":120,"height":40,"left":100,"top":200,` +
		`"right":220,"bottom":240,"viewportWidth":1280,"viewportHeight":940,` +
		`"display":"block","visibility":"visible","opacity":1}`
	var m consentElementMetrics
	if err := json.Unmarshal([]byte(raw), &m); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if m != visible() {
		t.Fatalf("decoded %+v, want %+v", m, visible())
	}
	if ok, why := m.clickable(); !ok {
		t.Fatalf("decoded probe output rejected: %s", why)
	}
}

func withDisplay(m consentElementMetrics, v string) consentElementMetrics {
	m.Display = v
	return m
}

func withVisibility(m consentElementMetrics, v string) consentElementMetrics {
	m.Visibility = v
	return m
}

func withOpacity(m consentElementMetrics, v float64) consentElementMetrics {
	m.Opacity = v
	return m
}

func withViewport(m consentElementMetrics, w, h float64) consentElementMetrics {
	m.ViewportWidth, m.ViewportHeight = w, h
	return m
}

func withBox(m consentElementMetrics, w, h float64) consentElementMetrics {
	m.Width, m.Height = w, h
	m.Right, m.Bottom = m.Left+w, m.Top+h
	return m
}

// atCenter moves the box so its center sits exactly at (cx, cy), keeping size.
func atCenter(m consentElementMetrics, cx, cy float64) consentElementMetrics {
	m.Left, m.Right = cx-m.Width/2, cx+m.Width/2
	m.Top, m.Bottom = cy-m.Height/2, cy+m.Height/2
	return m
}
