package automationfleet

import "github.com/tuwibu/automationfleet/internal/winapi"

// installMouseHook is the OS seam for the takeover hook. Package-level so tests
// can pin it: registering a native handle goes through reconcileTakeover, and a
// real WH_MOUSE_LL hook would make every test that registers one hostage to
// whatever the human's mouse is doing.
var installMouseHook = winapi.InstallMouseHook

// reconcileTakeover installs or uninstalls the mouse-takeover watchdog to match
// the current native-handle count. Idempotent and order-independent: it reads
// the authoritative count rather than trusting a single transition edge, so
// racing Register/Unregister calls converge on the correct state. BiDi-only
// fleets (nativeCount==0) pay zero hook overhead.
func (f *Fleet) reconcileTakeover() {
	f.takeoverMu.Lock()
	defer f.takeoverMu.Unlock()

	f.mu.RLock()
	want := f.nativeCount > 0 && !f.stopped
	f.mu.RUnlock()

	installed := f.watchdog != nil
	switch {
	case want && !installed:
		f.startTakeoverLocked()
	case !want && installed:
		f.stopTakeoverLocked()
	}
}

// startTakeoverLocked spins up the watchdog and installs the low-level mouse
// hook. Caller must hold f.takeoverMu. If the hook fails to install, the
// watchdog is torn down (without a hook it can never fire) and the fleet keeps
// running without takeover — logged, never fatal.
func (f *Fleet) startTakeoverLocked() {
	w := newTakeoverWatchdog(takeoverIdle,
		func() { f.Pause(ReasonUserTakeover) },
		func() { f.Resume(ReasonUserTakeover) })
	w.start()
	uninstall, err := installMouseHook(w.notify)
	if err != nil {
		f.log.Warnf("automationfleet: mouse takeover watchdog disabled: %v", err)
		w.stop()
		return
	}
	f.watchdog = w
	f.hookUninstall = uninstall
	f.log.Infof("automationfleet: mouse takeover watchdog installed")
}

// stopTakeoverLocked uninstalls the hook and stops the watchdog (which clears
// any lingering "user-takeover" pause). Caller must hold f.takeoverMu.
//
// Ordering matters: hookUninstall() first blocks until the message-pump thread
// exits (no more notify() calls can arrive), THEN watchdog.stop() blocks until
// the run loop drains — and stop() resumes before it closes its done channel,
// so once this returns the ReasonUserTakeover pause is guaranteed cleared. A
// concurrent Snapshot()/hasReason() may still observe the pause momentarily
// while teardown is in flight; that is benign (it clears within one tick).
func (f *Fleet) stopTakeoverLocked() {
	if f.hookUninstall != nil {
		f.hookUninstall()
		f.hookUninstall = nil
	}
	if f.watchdog != nil {
		f.watchdog.stop()
		f.watchdog = nil
	}
	f.log.Infof("automationfleet: mouse takeover watchdog removed")
}
