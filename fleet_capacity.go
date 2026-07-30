package automationfleet

// Lookup + runtime-tuning surface. Split out of fleet.go, which owns
// construction, registration and lifecycle.

// InputMode reports how the registered browser dispatches input. The value is
// the one derived from the kit at Register time, so it never disagrees with the
// browser. Returns ErrUnknownBrowser when id was never registered.
func (f *Fleet) InputMode(id string) (InputMode, error) {
	f.mu.RLock()
	defer f.mu.RUnlock()
	h, ok := f.handles[id]
	if !ok {
		return InputRemote, ErrUnknownBrowser
	}
	return inputModeOf(h), nil
}

// NativeCapacity reports how many native slots are taken and how many exist.
// The budget is machine-wide, not per-kit: the limit is one mouse and one
// keyboard, so nine Chrome windows and nine Firefox windows are the same nine
// slots.
func (f *Fleet) NativeCapacity() (used, max int) {
	f.mu.RLock()
	defer f.mu.RUnlock()
	return f.nativeCount, f.cfg.nativeCap
}

// SetRemoteWorkers resizes the remote (CDP / BiDi) worker pool while the fleet
// runs. Growing spawns immediately; shrinking lets the surplus workers exit
// AFTER the job they are on, so a resize never cancels work in flight.
//
// Returns ErrFleetStopped after Stop / AbortAll — and spawns nothing in that
// case — and an error for n <= 0.
func (f *Fleet) SetRemoteWorkers(n int) error { return f.dispatcher.setRemoteWorkers(n) }
