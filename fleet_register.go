package automationfleet

import (
	"errors"
	"fmt"

	"github.com/tuwibu/chromekit"
	"github.com/tuwibu/firefoxkit"
)

// Register adds a browser handle to the fleet. Must be called before Submit
// references the same BrowserID.
//
// The input mode is DERIVED from the Driver, never declared: h.Native is
// overwritten with what the kit reports. Pass ExpectNative() / ExpectRemote()
// to assert what the caller believed it launched — a mismatch is an error, not
// a silent downgrade.
func (f *Fleet) Register(h *BrowserHandle, opts ...RegisterOption) error {
	if h == nil {
		return errors.New("automationfleet: Register: nil handle")
	}
	if err := h.validate(); err != nil {
		return err
	}
	// Derive first, assert second. Nothing between these two lines may leave a
	// handle whose Native disagrees with the kit.
	h.Native = h.Driver.InputBackend() == InputNative
	if err := newRegisterConfig(opts...).check(inputModeOf(h)); err != nil {
		return err
	}

	// Duplicate check, capacity check and the increment share ONE critical
	// section. Counting outside it is how a cap of nine admits ten: two
	// goroutines both read eight, both decide there is room, both insert.
	f.mu.Lock()
	if _, exists := f.handles[h.ID]; exists {
		f.mu.Unlock()
		return errors.New("automationfleet: browser id already registered: " + h.ID)
	}
	if h.Native && f.nativeCount >= f.cfg.nativeCap {
		f.mu.Unlock()
		return fmt.Errorf("%w: %d/%d native slots in use, cannot register %q",
			ErrNativeCapacityExceeded, f.nativeCount, f.cfg.nativeCap, h.ID)
	}
	f.handles[h.ID] = h
	if h.Native {
		f.nativeCount++
	}
	f.mu.Unlock()

	f.reconcileTakeover()
	return nil
}

// RegisterChrome wraps a chromekit.Browser in a Driver and registers it under
// id. The input mode comes from the kit; the window origin and DPI scale come
// from the kit too (phase 2 moved the coordinate frame there), so neither is a
// parameter any more.
func (f *Fleet) RegisterChrome(id string, b *chromekit.Browser, opts ...RegisterOption) error {
	if b == nil {
		return errors.New("automationfleet: RegisterChrome: browser is nil")
	}
	return f.Register(&BrowserHandle{ID: id, Driver: WrapChrome(b)}, opts...)
}

// RegisterFirefox wraps a firefoxkit.Browser in a Driver and registers it under
// id. Prefer remote (BiDi) — native firefox input has a content-offset gap in
// firefoxkit (see WrapFirefox).
func (f *Fleet) RegisterFirefox(id string, b *firefoxkit.Browser, opts ...RegisterOption) error {
	if b == nil {
		return errors.New("automationfleet: RegisterFirefox: browser is nil")
	}
	return f.Register(&BrowserHandle{ID: id, Driver: WrapFirefox(b)}, opts...)
}

// inputModeOf reports the mode a handle resolves to.
func inputModeOf(h *BrowserHandle) InputMode {
	if h != nil && h.Native {
		return InputNative
	}
	return InputRemote
}

// AbortAll cancels every in-flight job, rejects the pending queue and tears the
// workers down. Same funnel as Stop — this is the name the hotkey path and the
// consumer's panic button use.
func (f *Fleet) AbortAll(reason string) { f.requestStop(reason) }

// Unregister removes a browser handle from the fleet. Returns ErrUnknownBrowser
// if the ID was never registered. Pending jobs targeting this BrowserID will
// fail with ErrUnknownBrowser when dispatched (dispatcher checks handle nil
// before executing). Symmetric to Register — required so callers can re-Register
// the same ID after the underlying Browser closes; without it the internal
// registry holds a stale handle forever and the next Register rejects.
func (f *Fleet) Unregister(id string) error {
	f.mu.Lock()
	h, exists := f.handles[id]
	if !exists {
		f.mu.Unlock()
		return ErrUnknownBrowser
	}
	delete(f.handles, id)
	// Read Native BEFORE the handle is gone so the count stays balanced —
	// decrementing on a non-native handle would corrupt the hook gate.
	if h.Native && f.nativeCount > 0 {
		f.nativeCount--
	}
	f.mu.Unlock()

	f.reconcileTakeover()
	return nil
}
