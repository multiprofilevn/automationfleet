package automationfleet

import "fmt"

// RegisterOption tunes a single Register / RegisterChrome / RegisterFirefox
// call. It exists so the caller can ASSERT the input mode without being able to
// DECLARE it: the mode itself is always derived from the kit
// (Driver.InputBackend), never from a parameter, so no call site can put the
// fleet and the browser into disagreement.
type RegisterOption func(*registerConfig)

// registerConfig is the resolved option set for one registration. A nil expect
// means "no expectation" — derive from the kit and register whatever it says.
type registerConfig struct {
	expect *InputMode
}

func newRegisterConfig(opts ...RegisterOption) *registerConfig {
	rc := &registerConfig{}
	for _, opt := range opts {
		if opt != nil {
			opt(rc)
		}
	}
	return rc
}

// ExpectNative asserts the browser was launched with the native input backend.
// A remote browser registered under this option is REJECTED — it is never
// quietly accepted as remote. This replaces the two-way check the old
// BrowserHandle.validate() did, and it exists for anti-bot reasons rather than
// convenience: a caller who believes a profile is driving a real cursor while
// it is actually driving CDP leaves automation traces it never agreed to.
func ExpectNative() RegisterOption {
	return func(rc *registerConfig) {
		m := InputNative
		rc.expect = &m
	}
}

// ExpectRemote asserts the browser was launched with a remote (CDP / BiDi)
// input backend. A native browser registered under this option is rejected —
// it would silently consume one of the scarce native capacity slots.
func ExpectRemote() RegisterOption {
	return func(rc *registerConfig) {
		m := InputRemote
		rc.expect = &m
	}
}

// check compares the mode derived from the kit against the caller's stated
// expectation. Returns nil when no expectation was given.
//
// The message names both sides on purpose: "expected native, kit reports
// remote" tells the operator the browser was launched wrong, whereas a bare
// sentinel would leave them guessing which half is at fault.
func (rc *registerConfig) check(actual InputMode) error {
	if rc.expect == nil || *rc.expect == actual {
		return nil
	}
	return fmt.Errorf("%w: expected %s, kit reports %s", ErrExpectationMismatch, *rc.expect, actual)
}
