package faketest

import (
	"strings"
	"time"
)

// ----------------------------------------------------------- transcript ----

// Count returns how many times op was invoked. "key" counts emitted characters.
func (d *Driver) Count(op string) int {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.counts[op]
}

// Events returns a copy of the full transcript.
func (d *Driver) Events() []Event {
	d.mu.Lock()
	defer d.mu.Unlock()
	out := make([]Event, len(d.events))
	copy(out, d.events)
	return out
}

// Keys returns every character emitted, in order, concatenated. Includes
// characters emitted before a simulated foreground loss — that is exactly the
// "what actually reached the OS" transcript a repair test needs.
func (d *Driver) Keys() string {
	d.mu.Lock()
	defer d.mu.Unlock()
	var b strings.Builder
	for _, e := range d.events {
		if e.Op == "key" {
			b.WriteString(e.Arg)
		}
	}
	return b.String()
}

// Ops returns the op names in order, keystrokes excluded.
func (d *Driver) Ops() []string {
	d.mu.Lock()
	defer d.mu.Unlock()
	out := make([]string, 0, len(d.events))
	for _, e := range d.events {
		if e.Op != "key" {
			out = append(out, e.Op)
		}
	}
	return out
}

// -------------------------------------------------------------- plumbing ----

// record appends an event, bumps the counter and returns the op latency. The
// onCall hook runs outside the lock so a hook may call back into the driver.
func (d *Driver) record(op, arg string, x, y float64) time.Duration {
	d.mu.Lock()
	d.counts[op]++
	d.events = append(d.events, Event{Op: op, Arg: arg, X: x, Y: y, At: time.Now()})
	lat, ok := d.perOp[op]
	if !ok {
		lat = d.latency
	}
	cb := d.onCall
	d.mu.Unlock()
	if cb != nil {
		cb(op)
	}
	d.waitGate(op)
	return lat
}

// extraErr reads the generic error injected via SetErr for an op with no
// dedicated field — every mirror added past the original small surface uses
// this instead of growing the Driver struct one field at a time.
func (d *Driver) extraErr(op string) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.extraErrs[op]
}

func sleep(d time.Duration) {
	if d > 0 {
		time.Sleep(d)
	}
}
