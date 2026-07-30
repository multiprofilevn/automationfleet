package main

import (
	"context"
	"fmt"
	stdlog "log"
	"path/filepath"
	"strconv"

	"github.com/tuwibu/automationfleet"
	"github.com/tuwibu/automationfleet/examples/bingsearch"
	"github.com/tuwibu/chromekit"
)

// launchAndRegister spawns and registers every remote (CDP) Chrome instance.
// Visible (not headless) so the operator can watch the run; CDP input never
// touches the real screen either way, so visibility here is purely for
// observing the run, tiled via the grid layout below so 25 windows don't
// stack directly on top of each other.
//
// Each browser is tracked in sd the moment it launches (not batched at the
// end) so a Ctrl+C mid-loop closes exactly what's been opened so far, not
// nothing. ctx is checked at the top of each iteration so an interrupt stops
// new launches promptly instead of finishing all 25 first.
func launchAndRegister(ctx context.Context, fleet *automationfleet.Fleet, chromePath, profileRoot string, sd *bingsearch.Shutdown) ([]*chromekit.Browser, []string, error) {
	browsers := make([]*chromekit.Browser, 0, instances)
	ids := make([]string, 0, instances)
	layout := bingsearch.NewGridLayout(instances)

	for i := 0; i < instances; i++ {
		if err := ctx.Err(); err != nil {
			return browsers, ids, fmt.Errorf("launch interrupted after %d/%d instances: %w", len(ids), instances, err)
		}

		id := fmt.Sprintf("chrome-remote-%d", i)
		cell := layout.Cell(i, id)
		profile := filepath.Join(profileRoot, id)

		// --window-size stays at the full logical viewport and NEVER shrinks to
		// the cell: Chrome multiplies both --window-size and --window-position
		// by the forced device scale factor, so a cell-sized request lands under
		// Chrome's minimum window width, gets clamped back up, and the windows
		// overlap. Shrinking is the scale factor's job; the page keeps a full
		// 1920x1080 viewport. --window-position is pre-divided by the same
		// factor so it lands where the tiling intends. See bingsearch.Cell.
		posX, posY := cell.ChromeWindowPosition()
		opts := []chromekit.Option{
			chromekit.WithHeadless(false), // default input backend is BackendCDP already
			chromekit.WithUserDataDir(profile),
			chromekit.WithFlag("window-position", fmt.Sprintf("%d,%d", posX, posY)),
			chromekit.WithFlag("window-size", fmt.Sprintf("%d,%d",
				bingsearch.LogicalViewportWidth, bingsearch.LogicalViewportHeight)),
			// 25 visible windows means most sit unfocused or occluded behind
			// a sibling most of the time; Chrome's background-tab/occluded-
			// window throttling would stall their timers and rAF, so it is
			// disabled for every window regardless of focus state.
			chromekit.WithFlag("disable-background-timer-throttling", true),
			chromekit.WithFlag("disable-backgrounding-occluded-windows", true),
			chromekit.WithFlag("disable-renderer-backgrounding", true),
		}
		if cell.Scale < 1.0 {
			// Formatted as a string: chromedp only renders string and bool flag
			// values and rejects everything else with "invalid exec pool flag"
			// (chromedp/allocate.go Allocate). A float64 here fails at launch.
			opts = append(opts, chromekit.WithFlag("force-device-scale-factor",
				strconv.FormatFloat(cell.Scale, 'f', -1, 64)))
		}

		b, err := chromekit.Launch(chromePath, opts...)
		if err != nil {
			return browsers, ids, fmt.Errorf("launch %s: %w", id, err)
		}
		browsers = append(browsers, b)
		sd.Track(id, func() { _ = b.Close() })

		if err := fleet.RegisterChrome(id, b, automationfleet.ExpectRemote()); err != nil {
			return browsers, ids, fmt.Errorf("register %s: %w", id, err)
		}
		ids = append(ids, id)
	}
	stdlog.Printf("launched+registered %d remote chrome instances", len(ids))
	return browsers, ids, nil
}
