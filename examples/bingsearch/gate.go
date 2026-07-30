package bingsearch

import (
	"fmt"
	"os"
)

// AllowRealInputEnv gates the two native examples (bing_chrome_native,
// bing_firefox_native). They drive the real OS mouse/keyboard, so nothing in
// those two examples launches a browser without this set to "1".
//
// The remote examples (bing_chrome_remote, bing_firefox_remote) do NOT read
// this — CDP/BiDi never touch the OS cursor or keyboard, so there is nothing
// for this gate to protect there.
const AllowRealInputEnv = "FLEET_ALLOW_REAL_INPUT"

// AllowRealInput reports whether the operator opted into a native run.
func AllowRealInput() bool { return os.Getenv(AllowRealInputEnv) == "1" }

// NativeGateMessage is printed instead of running when AllowRealInput is
// false — it explains the scenario without opening a single window.
func NativeGateMessage(browser string, n int) string {
	return fmt.Sprintf(
		"this example would launch %d %s window(s) on the NATIVE input path.\n"+
			"each launch takes over the real OS mouse and keyboard for the duration\n"+
			"of the fleet.AcquireForeground lease around it, and every search after\n"+
			"that runs through the single native worker with the real cursor.\n\n"+
			"nothing has been launched. to actually run it:\n\n"+
			"  set %s=1\n"+
			"  go run ./examples/bing_%s_native\n",
		n, browser, AllowRealInputEnv, browser,
	)
}
