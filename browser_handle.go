package automationfleet

import "errors"

// BrowserHandle binds a Driver (chromekit or firefoxkit, via WrapChrome /
// WrapFirefox) to a fleet-stable ID.
//
// Native is DERIVED, never declared: Register overwrites it with what
// Driver.InputBackend() reports, so the fleet and the browser can never
// disagree about how input is dispatched. Callers assert what they believe they
// launched with ExpectNative() / ExpectRemote(); a mismatch is an error.
//
// The window origin and DPI scale are no longer part of this struct. The kit
// owns the coordinate frame (pinFrame measures it live), so a number the runner
// wrote down before launch is at best a cross-check and at worst a stale value
// the drift guard would compute a wrong expectation from.
type BrowserHandle struct {
	ID     string
	Driver Driver
	Native bool
}

func (h *BrowserHandle) validate() error {
	if h == nil {
		return errors.New("BrowserHandle: nil")
	}
	if h.ID == "" {
		return errors.New("BrowserHandle: ID required")
	}
	if h.Driver == nil {
		return errors.New("BrowserHandle: Driver required")
	}
	return nil
}
