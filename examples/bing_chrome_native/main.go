// bing_chrome_native drives automationfleet.DefaultNativeCapacity (9) Chrome
// instances on the NATIVE input path: real OS mouse, real keyboard. Each
// instance i (0-based) navigates to Bing, searches "anime {i+1}", and clicks
// the first organic result.
//
// Native capacity is a machine-wide budget shared across BOTH kits
// (fleet.go: nativeCount is counted across chrome + firefox together) — this
// example alone claims all 9 slots on purpose, so raising `instances` past
// automationfleet.DefaultNativeCapacity fails loudly at Register with
// ErrNativeCapacityExceeded instead of silently degrading.
//
// Windows are tiled via bingsearch.NewGridLayout so the 9 instances don't
// stack directly on top of each other — see launchAndRegister.
//
// FOREGROUND LEASE — the one thing this example exists to demonstrate:
// opening a new browser window steals OS foreground. If instance 3's
// keystrokes are mid-flight when instance 4's Chrome window pops up, the
// tail of instance 3's string lands in instance 4 instead. Every launch
// below is wrapped in fleet.AcquireForeground/Release for exactly that
// reason — see runLaunchPhase. bing_chrome_remote does NOT need this: CDP
// addresses input to a specific tab over the wire, so opening one browser
// there can never steal input away from another.
//
// SAFETY: this example drives the REAL mouse and keyboard. Without
// FLEET_ALLOW_REAL_INPUT=1 it prints what it would do and exits 0 — nothing
// is launched.
//
// Ctrl+C (SIGINT) or SIGTERM closes every browser launched so far before the
// process exits — see bingsearch.RunMain. The foreground lease held during
// each launch iteration is always released before the next AcquireForeground
// call (or the loop exits on ctx cancellation before acquiring one), so an
// interrupt can never leave a lease held with nothing left to release it.
//
// Usage:
//
//	set CHROME_PATH=C:\...\chrome.exe   (optional — a couple of well-known
//	                                      install paths are probed too)
//	set FLEET_ALLOW_REAL_INPUT=1
//	go run ./examples/bing_chrome_native
package main

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/tuwibu/automationfleet"
	"github.com/tuwibu/automationfleet/examples/bingsearch"
)

const (
	instances  = automationfleet.DefaultNativeCapacity // 9 — sits exactly at the native cap, on purpose
	jobTimeout = 25 * time.Second
)

func main() {
	bingsearch.RunMain(run)
}

func run(ctx context.Context, sd *bingsearch.Shutdown) error {
	if !bingsearch.AllowRealInput() {
		fmt.Print(bingsearch.NativeGateMessage("chrome", instances))
		return nil
	}

	chromePath, err := bingsearch.ResolveChromePath()
	if err != nil {
		return err
	}

	fleet := automationfleet.New(
		automationfleet.WithLogger(bingsearch.StdLogger{}),
		automationfleet.WithDefaultTimeout(jobTimeout),
		automationfleet.WithNativeCapacity(instances), // explicit: this example alone claims the whole budget
		automationfleet.WithStopHotkeyDisabled(),
	)
	fleet.Start()
	sd.Track("fleet", fleet.Stop)

	profileRoot, err := os.MkdirTemp("", "bing-chrome-native-*")
	if err != nil {
		return fmt.Errorf("temp profile root: %w", err)
	}
	sd.Track("temp profile root "+profileRoot, func() { _ = os.RemoveAll(profileRoot) })

	browsers, ids, err := launchAndRegister(ctx, fleet, chromePath, profileRoot, sd)
	if err != nil {
		return err
	}

	return runQueries(ctx, fleet, browsers, ids)
}
