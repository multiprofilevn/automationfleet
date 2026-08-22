// bing_chrome_remote drives 25 Chrome instances on the REMOTE (CDP) input
// path: virtual mouse and keyboard dispatched over DevTools Protocol, no
// real cursor involved. Each instance i (0-based) navigates to Bing,
// searches "anime {i+1}", and clicks the first organic result.
//
// Windows run visible (not headless) and tiled via bingsearch.NewGridLayout
// so the operator can watch all 25 without them stacking on top of each
// other — see launchAndRegister.
//
// No foreground lease is needed here, unlike bing_chrome_native: CDP
// addresses input to a specific tab over the wire, so opening one browser
// can never steal input away from another — there is no shared OS
// foreground to contend over. All 25 instances run through
// WithRemoteWorkers' parallel pool instead of the single serialized native
// worker.
//
// This example does NOT read FLEET_ALLOW_REAL_INPUT — that gate exists
// because native input touches the real desktop; CDP does not, so there is
// nothing for it to protect here.
//
// Ctrl+C (SIGINT) or SIGTERM closes every browser launched so far before the
// process exits — see bingsearch.RunMain.
//
// Usage:
//
//	set CHROME_PATH=C:\...\chrome.exe   (optional — a couple of well-known
//	                                      install paths are probed too)
//	go run ./examples/bing_chrome_remote
package main

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/multiprofilevn/automationfleet"
	"github.com/multiprofilevn/automationfleet/examples/bingsearch"
)

const (
	instances     = 25
	remoteWorkers = 8 // parallel CDP pool; less than instances on purpose — see README "Capacity and worker pool"
	jobTimeout    = 25 * time.Second
)

func main() {
	bingsearch.RunMain(run)
}

func run(ctx context.Context, sd *bingsearch.Shutdown) error {
	chromePath, err := bingsearch.ResolveChromePath()
	if err != nil {
		return err
	}

	fleet := automationfleet.New(
		automationfleet.WithLogger(bingsearch.StdLogger{}),
		automationfleet.WithDefaultTimeout(jobTimeout),
		automationfleet.WithRemoteWorkers(remoteWorkers),
		automationfleet.WithStopHotkeyDisabled(),
	)
	fleet.Start()
	sd.Track("fleet", fleet.Stop)

	profileRoot, err := os.MkdirTemp("", "bing-chrome-remote-*")
	if err != nil {
		return fmt.Errorf("temp profile root: %w", err)
	}
	sd.Track("temp profile root "+profileRoot, func() { _ = os.RemoveAll(profileRoot) })

	browsers, ids, err := launchAndRegister(ctx, fleet, chromePath, profileRoot, sd)
	if err != nil {
		return err
	}

	return runQueries(fleet, browsers, ids)
}
