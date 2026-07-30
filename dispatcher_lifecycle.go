package automationfleet

import (
	"fmt"
	"time"
)

// start spins up workers. No-op on second call.
func (d *Dispatcher) start() {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.started {
		return
	}
	d.started = true
	d.cdpJobs = make(chan *queuedJob, 64)

	d.wg.Add(1)
	go d.nativeWorker(d.cdpJobs, d.stopCh)

	// Every worker captures the channels at spawn time, under d.mu. Reading
	// d.cdpJobs from inside a worker is a data race against shutdown, and it
	// bought nothing.
	for i := 0; i < d.remoteTarget; i++ {
		d.spawnRemoteWorkerLocked()
	}
}

// enqueue places a job on the priority heap and wakes the native worker.
func (d *Dispatcher) enqueue(j Job) chan JobResult {
	resCh := make(chan JobResult, 1)
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.stopped {
		go deliver(resCh, JobResult{
			BrowserID: j.BrowserID,
			Status:    StatusRejected,
			Err:       ErrFleetStopped,
		})
		return resCh
	}
	d.insertSeq++
	qj := &queuedJob{
		id:        nextJobID(),
		insertSeq: d.insertSeq,
		job:       j,
		result:    resCh,
		enqueued:  time.Now(),
	}
	d.queue.push(qj)
	d.inflight.Add(1)
	d.cond.Signal()
	return resCh
}

// abortAll cancels every in-flight job and rejects pending ones.
func (d *Dispatcher) abortAll(reason string) {
	d.mu.Lock()
	d.stopped = true
	pending := d.queue.drain()
	d.cond.Broadcast()
	d.mu.Unlock()

	// Single funnel for BOTH stopped flags. They sit under different locks, so
	// the invariant that keeps Submit and enqueue agreeing is "exactly one place
	// assigns them" — nothing else in the package writes either field.
	d.fleet.mu.Lock()
	d.fleet.stopped = true
	d.fleet.mu.Unlock()

	for _, qj := range pending {
		deliver(qj.result, JobResult{
			ID:        qj.id,
			BrowserID: qj.job.BrowserID,
			Status:    StatusCancelled,
			Err:       fmt.Errorf("aborted: %s", reason),
		})
		d.inflight.Done()
	}
	// Broadcast shutdown instead of closing cdpJobs. Closing it forced abortAll
	// to nil the field, which nativeWorker then read without the lock.
	d.stopChOnce.Do(func() { close(d.stopCh) })
}

// waitDrained blocks until in-flight + queue both reach zero.
func (d *Dispatcher) waitDrained() { d.inflight.Wait() }

// pause adds reason to the active set. Returns true only on the running→paused
// transition (empty set → non-empty), so callers fire OnPause exactly once
// regardless of how many sources pause. Adding an already-present reason is a
// no-op. Add + transition-detection happen under one lock (same atomicity the
// old bool had).
func (d *Dispatcher) pause(reason string) bool {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.stopped {
		return false
	}
	was := d.isPaused()
	d.pauseReasons[reason] = struct{}{}
	return !was
}

// resume removes reason from the active set and wakes blocked workers only when
// the set becomes empty. Returns true only on the paused→running transition, so
// one source resuming cannot un-pause a fleet another source still holds.
// Removing an absent reason is a no-op.
func (d *Dispatcher) resume(reason string) bool {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.stopped {
		return false
	}
	if _, ok := d.pauseReasons[reason]; !ok {
		return false
	}
	delete(d.pauseReasons, reason)
	if d.isPaused() {
		return false
	}
	d.cond.Broadcast()
	return true
}
