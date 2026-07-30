// bing_firefox_remote drives 25 Firefox instances on the REMOTE (BiDi)
// input path: virtual mouse and keyboard dispatched over WebDriver BiDi, no
// real cursor involved. Each instance i (0-based) navigates to Bing,
// searches "anime {i+1}", and clicks the first organic result.
//
// Windows run visible (not headless) and tiled via bingsearch.NewGridLayout
// so the operator can watch all 25 without them stacking on top of each
// other — see launchAndRegister.
//
// No foreground lease is needed here, unlike bing_firefox_native: BiDi
// addresses input to a specific browsing context over the wire, so opening
// one browser can never steal input away from another — there is no shared
// OS foreground to contend over. All 25 instances run through
// WithRemoteWorkers' parallel pool instead of the single serialized native
// worker.
//
// This example does NOT read FLEET_ALLOW_REAL_INPUT — that gate exists
// because native input touches the real desktop; BiDi does not, so there is
// nothing for it to protect here.
//
// Ctrl+C (SIGINT) or SIGTERM closes every browser launched so far before the
// process exits — see bingsearch.RunMain.
//
// Usage:
//
//	set FIREFOX_PATH=C:\...\firefox.exe   (optional — a well-known install
//	                                        path is probed too)
//	go run ./examples/bing_firefox_remote
package main

import (
	"context"
	"time"

	"github.com/tuwibu/automationfleet"
	"github.com/tuwibu/automationfleet/examples/bingsearch"
)

const (
	instances     = 25
	remoteWorkers = 8 // parallel BiDi pool; less than instances on purpose — see README "Capacity and worker pool"
	jobTimeout    = 25 * time.Second
)

func main() {
	bingsearch.RunMain(run)
}

func run(ctx context.Context, sd *bingsearch.Shutdown) error {
	firefoxPath, err := bingsearch.ResolveFirefoxPath()
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

	// No user-data-dir passed to Launch: firefoxkit allocates a temp profile
	// per instance and removes it on Close (browser_launch.go:
	// ensureProfileDir / cleanupProfile), so there is nothing here for this
	// example to clean up itself.
	browsers, ids, err := launchAndRegister(ctx, fleet, firefoxPath, sd)
	if err != nil {
		return err
	}

	return runQueries(fleet, browsers, ids)
}
