package faketest

// Op gate: a deterministic way to hold N driver calls inside the driver at the
// same time. Tests that need to observe concurrency (how many workers ran at
// once) or to shrink a worker pool while jobs are genuinely in flight cannot
// use a sleep for that — a sleep proves nothing about what overlapped. The gate
// blocks the op until the test releases it, so the test controls the overlap
// exactly.

// HoldOp makes every subsequent call to op block inside the driver until
// ReleaseOps is called. Calls already past the gate are unaffected.
func (d *Driver) HoldOp(op string) {
	d.mu.Lock()
	d.holdOp = op
	if d.holdCh == nil {
		d.holdCh = make(chan struct{})
	}
	d.mu.Unlock()
}

// ReleaseOps unblocks everything waiting at the gate and disarms it.
func (d *Driver) ReleaseOps() {
	d.mu.Lock()
	ch := d.holdCh
	d.holdCh = nil
	d.holdOp = ""
	d.mu.Unlock()
	if ch != nil {
		close(ch)
	}
}

// Held reports how many calls are currently blocked at the gate.
func (d *Driver) Held() int {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.held
}

// PeakHeld reports the largest number of calls that were blocked at the gate at
// the same moment — i.e. the observed concurrency of the worker pool.
func (d *Driver) PeakHeld() int {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.peakHeld
}

// waitGate blocks op at the gate when one is armed for it. Called from record,
// outside the driver lock.
func (d *Driver) waitGate(op string) {
	d.mu.Lock()
	if d.holdOp != op || d.holdCh == nil {
		d.mu.Unlock()
		return
	}
	ch := d.holdCh
	d.held++
	if d.held > d.peakHeld {
		d.peakHeld = d.held
	}
	d.mu.Unlock()

	<-ch

	d.mu.Lock()
	d.held--
	d.mu.Unlock()
}
