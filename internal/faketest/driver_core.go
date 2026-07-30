package faketest

import "context"

// ---------------------------------------------------------------- driver ----

// Focus mirrors Driver.Focus.
func (d *Driver) Focus(_ context.Context) error {
	sleep(d.record("focus", "", 0, 0))
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.focusErr
}

// ContentOffset mirrors Driver.ContentOffset.
func (d *Driver) ContentOffset() (int, int, error) {
	sleep(d.record("contentoffset", "", 0, 0))
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.offsetX, d.offsetY, d.offsetErr
}

// SetWindowScale fixes what WindowScale reports — the scale the KIT measured,
// which a spec sets independently of BrowserHandle.Scale.
func (d *Driver) SetWindowScale(s float64, err error) {
	d.mu.Lock()
	d.scale, d.scaleErr = s, err
	d.mu.Unlock()
}

// WindowScale mirrors Driver.WindowScale.
func (d *Driver) WindowScale() (float64, error) {
	sleep(d.record("windowscale", "", 0, 0))
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.scale, d.scaleErr
}

// SetWindowRect fixes what WindowRect reports. Safe to call mid-test (e.g. from
// an onCall hook) to model a window that moves during an operation.
func (d *Driver) SetWindowRect(r Rect, err error) {
	d.mu.Lock()
	d.rect, d.rectErr = r, err
	d.mu.Unlock()
}

// WindowRect mirrors Driver.WindowRect.
func (d *Driver) WindowRect() (Rect, error) {
	sleep(d.record("windowrect", "", 0, 0))
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.rect, d.rectErr
}

// SetInvalidateErr makes InvalidateNativeWindow fail.
func (d *Driver) SetInvalidateErr(err error) {
	d.mu.Lock()
	d.invalidateErr = err
	d.mu.Unlock()
}

// InvalidateNativeWindow mirrors Driver.InvalidateNativeWindow. It records the
// call so a spec can assert the frame was re-measured (or that it was not).
func (d *Driver) InvalidateNativeWindow() error {
	sleep(d.record("invalidatenativewindow", "", 0, 0))
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.invalidateErr
}
