package main

import (
	"context"
	"errors"
	"fmt"
	stdlog "log"
	"path/filepath"
	"strconv"

	"github.com/tuwibu/automationfleet"
	"github.com/tuwibu/automationfleet/examples/bingsearch"
	"github.com/tuwibu/chromekit"
)

// launchAndRegister spawns and registers every native Chrome instance,
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
func launchAndRegister(ctx context.Context, fleet *automationfleet.Fleet, chromePath, profileRoot string, sd *bingsearch.Shutdown) ([]*chromekit.Browser, []string, error) {
	browsers := make([]*chromekit.Browser, 0, instances)
	ids := make([]string, 0, instances)
	layout := bingsearch.NewGridLayout(instances)

	for i := 0; i < instances; i++ {
		id := fmt.Sprintf("chrome-native-%d", i)
		cell := layout.Cell(i, id)
		profile := filepath.Join(profileRoot, id)

		lease, err := fleet.AcquireForeground(ctx, "launch-"+id)
		if err != nil {
			return browsers, ids, fmt.Errorf("acquire foreground for %s: %w", id, err)
		}

		// Same --window-size / --window-position arrangement as the REMOTE
		// example, and for the same reason: Chrome multiplies BOTH flags by the
		// forced device scale factor. --window-size stays at the full logical
		// viewport so the physical window lands well clear of Chrome's own
		// minimum width (a cell-sized request would be scaled below it, clamped
		// back up, and the windows would overlap); --window-position is
		// pre-divided by the same factor so the tile lands where the grid
		// intends. Shrinking is the scale factor's job.
		posX, posY := cell.ChromeWindowPosition()
		opts := []chromekit.Option{
			chromekit.WithInputBackend(chromekit.BackendNative),
			chromekit.WithHeadless(false),
			chromekit.WithUserDataDir(profile),
			// WithNativeWindow is a fallback only: chromekit measures its own
			// frame from the live window HWND and that measurement wins once it
			// lands. cell.Scale is what the measurement will converge on, so
			// launch starts from the right frame instead of a wrong one.
			chromekit.WithNativeWindow(cell.X, cell.Y, cell.Scale),
			chromekit.WithFlag("window-position", fmt.Sprintf("%d,%d", posX, posY)),
			chromekit.WithFlag("window-size", fmt.Sprintf("%d,%d",
				bingsearch.LogicalViewportWidth, bingsearch.LogicalViewportHeight)),
			// Most of these windows sit unfocused or occluded behind a
			// sibling most of the time; Chrome's background-tab/occluded-
			// window throttling would stall their timers and rAF, so it is
			// disabled for every window regardless of focus state.
			chromekit.WithFlag("disable-background-timer-throttling", true),
			chromekit.WithFlag("disable-backgrounding-occluded-windows", true),
			chromekit.WithFlag("disable-renderer-backgrounding", true),
		}
		if cell.Scale < 1.0 {
			// The native coordinate frame honours this now. chromekit's
			// WindowScale reads the page's devicePixelRatio — which IS the
			// CSS-to-physical ratio a forced factor produces, unlike the OS DPI
			// ratio it used to read — and only believes it once GetWindowRect
			// independently agrees. Before that it multiplied a CSS x of ~917
			// by 1.0 instead of 0.3333 and the drift guard caught the cursor
			// being aimed at x=2197 on a 1920-wide screen.
			//
			// Formatted as a string: chromedp only renders string and bool flag
			// values and rejects everything else with "invalid exec pool flag"
			// (chromedp/allocate.go Allocate). A float64 here fails at launch.
			opts = append(opts, chromekit.WithFlag("force-device-scale-factor",
				strconv.FormatFloat(cell.Scale, 'f', -1, 64)))
		}

		b, err := chromekit.Launch(chromePath, opts...)
		if err != nil {
			lease.Release()
			return browsers, ids, fmt.Errorf("launch %s: %w", id, err)
		}
		browsers = append(browsers, b)
		sd.Track(id, func() { _ = b.Close() })

		if err := fleet.RegisterChrome(id, b, automationfleet.ExpectNative()); err != nil {
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
		stdlog.Printf("launched+registered %s at (%d,%d)", id, cell.X, cell.Y)
	}
	return browsers, ids, nil
}
