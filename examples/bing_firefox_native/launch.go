package main

import (
	"context"
	"errors"
	"fmt"
	stdlog "log"

	"github.com/multiprofilevn/automationfleet"
	"github.com/multiprofilevn/automationfleet/examples/bingsearch"
	"github.com/multiprofilevn/firefoxkit"
)

// launchAndRegister spawns and registers every native Firefox instance,
// wrapping EACH launch in fleet.AcquireForeground/Release. That lease is
// what stops window N+1 popping up and stealing the foreground mid-keystroke
// from whatever native job the dispatcher might otherwise be running — see
// the AcquireForeground doc comment in foreground_lease.go. All launches
// happen here, before any search runs, so no further foreground contention
// exists once runQueries starts.
//
// ctx is passed straight into AcquireForeground: if it's cancelled (Ctrl+C)
// while waiting for the lease, AcquireForeground returns immediately with
// ctx.Err() and no lease is held — so a cancellation here never leaves a
// lease outstanding with nothing left to release it. Once a lease IS
// acquired, the loop always reaches lease.Release() before the next
// ctx check, so a lease is never held across iterations.
//
// Each browser is tracked in sd the moment it launches (not batched at the
// end) so a Ctrl+C mid-loop closes exactly what's been opened so far.
func launchAndRegister(ctx context.Context, fleet *automationfleet.Fleet, firefoxPath string, sd *bingsearch.Shutdown) ([]*firefoxkit.Browser, []string, error) {
	browsers := make([]*firefoxkit.Browser, 0, instances)
	ids := make([]string, 0, instances)
	layout := bingsearch.NewGridLayout(instances)

	for i := 0; i < instances; i++ {
		id := fmt.Sprintf("firefox-native-%d", i)
		cell := layout.Cell(i, id)

		lease, err := fleet.AcquireForeground(ctx, "launch-"+id)
		if err != nil {
			return browsers, ids, fmt.Errorf("acquire foreground for %s: %w", id, err)
		}

		opts := []firefoxkit.Option{
			firefoxkit.WithInputBackend(firefoxkit.BackendNative),
			firefoxkit.WithHeadless(false),
			// WithNativeWindow is a fallback only: firefoxkit measures its own
			// frame from the live window HWND once resolved, and that
			// measurement wins over this. cell.Scale is what the measurement
			// will converge on, so launch starts from the right frame instead
			// of a wrong one.
			firefoxkit.WithNativeWindow(cell.X, cell.Y, cell.Scale),
			// Firefox has no --disable-background-timer-throttling /
			// --disable-backgrounding-occluded-windows flags. These two
			// prefs are the closest real equivalents (verified against
			// Mozilla bug 1362322 and the widget.windows occlusion-tracking
			// pref, not invented): the first turns off budget-based
			// background timer throttling, the second turns off window-
			// occlusion tracking, which is what would otherwise throttle
			// rendering for the windows sitting behind a sibling.
			firefoxkit.WithPref("dom.timeout.enable_budget_timer_throttling", false),
			firefoxkit.WithPref("widget.windows.window_occlusion_tracking.enabled", false),
		}
		if cell.Scale < 1.0 {
			// layout.css.devPixelsPerPx. The native coordinate frame honours
			// this now: firefoxkit's WindowScale reads the page's
			// devicePixelRatio — which IS the CSS-to-physical ratio this pref
			// produces, unlike the OS DPI ratio it used to read — and only
			// believes it once GetWindowRect independently agrees.
			//
			// Before that, a 640px window with a 1872px CSS viewport had every
			// CSS x multiplied by 1.0 instead of 0.333, and the drift guard
			// caught the cursor being aimed off the far edge of the screen:
			//
			//	cursor drift detected: expected (2197,243) got (1919,243)
			//
			// Unlike Chrome, this does NOT move the window: SetWindowBounds
			// below still sizes the tile in screen pixels. The pref only
			// changes how much page fits inside it.
			opts = append(opts, firefoxkit.WithScaleFactor(cell.Scale))
		}

		b, err := firefoxkit.Launch(firefoxPath, opts...)
		if err != nil {
			lease.Release()
			return browsers, ids, fmt.Errorf("launch %s: %w", id, err)
		}
		browsers = append(browsers, b)
		sd.Track(id, func() { _ = b.Close() })

		// Firefox has no --window-position CLI flag (see the doc comment on
		// firefoxkit's Browser.SetWindowBounds) — position/size the window
		// after launch instead. Best-effort/cosmetic: log and keep going
		// rather than failing the whole run if the HWND isn't resolvable
		// yet.
		if err := b.SetWindowBounds(cell.X, cell.Y, cell.Width, cell.Height); err != nil {
			stdlog.Printf("tile %s: %v (window stays at Firefox's default position)", id, err)
		}

		if err := fleet.RegisterFirefox(id, b, automationfleet.ExpectNative()); err != nil {
			lease.Release()
			if errors.Is(err, automationfleet.ErrNativeCapacityExceeded) {
				return browsers, ids, fmt.Errorf("register %s: %w — this example is sized to exactly the native cap (%d); "+
					"lower `instances` or raise WithNativeCapacity if you deliberately need more", id, err, instances)
			}
			return browsers, ids, fmt.Errorf("register %s: %w", id, err)
		}
		ids = append(ids, id)

		// Release only after Register returns — the window must be up AND
		// counted before the next launch is allowed to steal foreground.
		lease.Release()
		stdlog.Printf("launched+registered %s", id)
	}
	return browsers, ids, nil
}
