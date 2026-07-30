package automationfleet

import "errors"

// ErrNativeCapacityExceeded is returned by Register* when the fleet already
// holds the maximum number of native handles. The cap is counted across every
// kit because the limit is physical — one mouse, one keyboard — not per-browser.
//
// Register returns it BEFORE touching the registry: a rejected registration
// leaves no partial state behind.
var ErrNativeCapacityExceeded = errors.New("automationfleet: native capacity exceeded")

// ErrExpectationMismatch is returned when ExpectNative()/ExpectRemote() does
// not match the input mode the kit was actually launched with. It fails loud
// on purpose: silently registering a browser the caller believed was native as
// a remote one strips the anti-bot guarantees that browser was launched for,
// and nothing downstream would ever say so.
var ErrExpectationMismatch = errors.New("automationfleet: input mode does not match expectation")
