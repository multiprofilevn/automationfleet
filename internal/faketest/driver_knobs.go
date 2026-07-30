package faketest

import "time"

// ---------------------------------------------------------------- knobs ----

// SetNative controls what the adapter reports as the input backend.
func (d *Driver) SetNative(v bool) { d.mu.Lock(); d.native = v; d.mu.Unlock() }

// IsNative reports the configured backend.
func (d *Driver) IsNative() bool { d.mu.Lock(); defer d.mu.Unlock(); return d.native }

// SetHWND fixes the handle HWND() reports. Zero (the default) models "the kit
// could not resolve one", which the click guard must treat as "skip", never as
// "fail the job".
func (d *Driver) SetHWND(h uintptr) { d.mu.Lock(); d.hwnd = h; d.mu.Unlock() }

// SetHWNDErr makes HWND() fail, modelling the firefox Connect path.
func (d *Driver) SetHWNDErr(err error) { d.mu.Lock(); d.hwndErr = err; d.mu.Unlock() }

// HWND mirrors Driver.HWND.
func (d *Driver) HWND() (uintptr, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.hwnd, d.hwndErr
}

// SetNoPage makes Current() report "no live page".
func (d *Driver) SetNoPage(v bool) { d.mu.Lock(); d.noPage = v; d.mu.Unlock() }

// HasNoPage reports the no-page knob.
func (d *Driver) HasNoPage() bool { d.mu.Lock(); defer d.mu.Unlock(); return d.noPage }

// SetValue seeds the fake input field (the "pre-existing value" case).
func (d *Driver) SetValue(v string) { d.mu.Lock(); d.value = v; d.mu.Unlock() }

// Value returns the current field contents.
func (d *Driver) Value() string { d.mu.Lock(); defer d.mu.Unlock(); return d.value }

// SetCenter fixes what ElementCenter returns.
func (d *Driver) SetCenter(x, y float64) { d.mu.Lock(); d.centerX, d.centerY = x, y; d.mu.Unlock() }

// SetContentOffset fixes what ContentOffset returns.
func (d *Driver) SetContentOffset(x, y int, err error) {
	d.mu.Lock()
	d.offsetX, d.offsetY, d.offsetErr = x, y, err
	d.mu.Unlock()
}

// SetLatency makes every call sleep d before returning.
func (d *Driver) SetLatency(v time.Duration) { d.mu.Lock(); d.latency = v; d.mu.Unlock() }

// SetOpLatency overrides the latency of a single op ("focus", "key", …).
func (d *Driver) SetOpLatency(op string, v time.Duration) {
	d.mu.Lock()
	d.perOp[op] = v
	d.mu.Unlock()
}

// SetErr injects an error into one op. Ops with a dedicated field (focus,
// moveto, clickat, readvalue, navigate, waitforselector, evaluate,
// elementcenter, clearinput) use it; every other op name — including every
// mirror added past the original small surface — falls back to the generic
// extraErrs map, read by extraErr in driver_transcript.go.
func (d *Driver) SetErr(op string, err error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	switch op {
	case "focus":
		d.focusErr = err
	case "moveto":
		d.moveErr = err
	case "clickat", "click":
		d.clickErr = err
	case "readvalue":
		d.readErr = err
	case "navigate":
		d.navErr = err
	case "waitforselector":
		d.waitErr = err
	case "evaluate":
		d.evalErr = err
	case "elementcenter":
		d.centerErr = err
	case "clearinput":
		d.clearErr = err
	default:
		d.extraErrs[op] = err
	}
}

// LoseForegroundAt arms the foreground-loss simulation: TypeHuman emits n
// characters into the field and then fails with err. When always is false the
// simulation disarms after firing once, so a subsequent repair round succeeds.
// n <= 0 disarms.
func (d *Driver) LoseForegroundAt(n int, err error, always bool) {
	d.mu.Lock()
	d.loseAt, d.loseErr, d.loseAlways = n, err, always
	d.loseArmed = n > 0
	d.mu.Unlock()
}

// SetOnCall registers a hook fired (outside the driver lock) at the start of
// every op except individual keystrokes. Used to sample OS thread identity.
func (d *Driver) SetOnCall(fn func(op string)) { d.mu.Lock(); d.onCall = fn; d.mu.Unlock() }

// SetOnKey registers a hook fired after each emitted character, i being the
// 1-based index within the current TypeHuman call.
func (d *Driver) SetOnKey(fn func(i int)) { d.mu.Lock(); d.onKey = fn; d.mu.Unlock() }
