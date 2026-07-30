package main

import (
	"context"
	"fmt"
	stdlog "log"

	"github.com/tuwibu/automationfleet"
	"github.com/tuwibu/automationfleet/examples/bingsearch"
	"github.com/tuwibu/firefoxkit"
)

// launchAndRegister spawns and registers every remote (BiDi) Firefox
// instance. Visible (not headless) so the operator can watch the run; BiDi
// input never touches the real screen either way, tiled via the grid layout
// below so 25 windows don't stack directly on top of each other.
//
// Each browser is tracked in sd the moment it launches (not batched at the
// end) so a Ctrl+C mid-loop closes exactly what's been opened so far, not
// nothing. ctx is checked at the top of each iteration so an interrupt stops
// new launches promptly instead of finishing all 25 first.
func launchAndRegister(ctx context.Context, fleet *automationfleet.Fleet, firefoxPath string, sd *bingsearch.Shutdown) ([]*firefoxkit.Browser, []string, error) {
	browsers := make([]*firefoxkit.Browser, 0, instances)
	ids := make([]string, 0, instances)
	layout := bingsearch.NewGridLayout(instances)

	for i := 0; i < instances; i++ {
		if err := ctx.Err(); err != nil {
			return browsers, ids, fmt.Errorf("launch interrupted after %d/%d instances: %w", len(ids), instances, err)
		}

		id := fmt.Sprintf("firefox-remote-%d", i)
		cell := layout.Cell(i, id)

		opts := []firefoxkit.Option{
			firefoxkit.WithHeadless(false), // default input backend is BackendBiDi already
			// Firefox equivalent of Chrome's occluded/background-timer
			// throttling flags (verified against Mozilla bug 1362322 and
			// the widget.windows occlusion-tracking pref, not invented) —
			// see the matching comment in bing_firefox_native/launch.go.
			firefoxkit.WithPref("dom.timeout.enable_budget_timer_throttling", false),
			firefoxkit.WithPref("widget.windows.window_occlusion_tracking.enabled", false),
		}
		if cell.Scale < 1.0 {
			// The cell is smaller than Firefox's own minimum window size —
			// shrink the whole browser UI proportionally (its enforced
			// minimum shrinks in physical pixels right along with it), or
			// Firefox clamps the window up past its cell and windows
			// overlap. See grid_layout_windows.go's Cell doc.
			opts = append(opts, firefoxkit.WithScaleFactor(cell.Scale))
		}

		b, err := firefoxkit.Launch(firefoxPath, opts...)
		if err != nil {
			return browsers, ids, fmt.Errorf("launch %s: %w", id, err)
		}
		browsers = append(browsers, b)
		sd.Track(id, func() { _ = b.Close() })

		// Firefox has no --window-position CLI flag — position/size after
		// launch instead. Best-effort/cosmetic: log and keep going rather
		// than failing the whole run if the HWND isn't resolvable (e.g.
		// non-Windows host).
		if err := b.SetWindowBounds(cell.X, cell.Y, cell.Width, cell.Height); err != nil {
			stdlog.Printf("tile %s: %v (window stays at Firefox's default position)", id, err)
		}

		if err := fleet.RegisterFirefox(id, b, automationfleet.ExpectRemote()); err != nil {
			return browsers, ids, fmt.Errorf("register %s: %w", id, err)
		}
		ids = append(ids, id)
	}
	stdlog.Printf("launched+registered %d remote firefox instances", len(ids))
	return browsers, ids, nil
}
