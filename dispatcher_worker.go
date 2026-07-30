package automationfleet

// Worker loops and the queue-selection rules they run on. Split out of
// dispatcher.go, which owns lifecycle and job execution.

// nativeWorker pops jobs in priority order. Native-critical jobs execute
// here serially; CDP-only jobs are forwarded to the cdp pool.
func (d *Dispatcher) nativeWorker(jobs chan *queuedJob, stop <-chan struct{}) {
	defer d.wg.Done()
	for {
		d.mu.Lock()
		// Wait while: nothing poppable OR paused (and not stopped). "Poppable"
		// excludes native jobs while a foreground lease is held or pending.
		for !d.stopped && (d.pickable() == nil || d.isPaused()) {
			d.cond.Wait()
		}
		if d.stopped && d.queue.Len() == 0 {
			d.mu.Unlock()
			return
		}
		qj := d.popPickable()
		d.mu.Unlock()

		if qj == nil {
			// Stopped with a lease-blocked native job still queued: abortAll
			// has already drained and cancelled the heap, so there is nothing
			// left to do here.
			continue
		}

		// Route based on action class AND handle backend. Click/Type/Navigate
		// only need the native critical section when the handle is launched
		// with native backend — CDP-backed handles run those parallel via
		// the cdp pool (no OS cursor → no drift check needed).
		if d.isNativeJob(qj) {
			d.runJob(qj, true)
		} else {
			select {
			case jobs <- qj:
			case <-stop:
				d.completeJob(qj, JobResult{Status: StatusCancelled, Err: ErrFleetStopped})
			case <-d.fleet.ctx.Done():
				d.completeJob(qj, JobResult{Status: StatusCancelled, Err: d.fleet.ctx.Err()})
			}
		}
	}
}

// isNativeJob reports whether qj must run inside the native critical section.
func (d *Dispatcher) isNativeJob(qj *queuedJob) bool {
	if !needsNativeCriticalSection(qj.job.Action) {
		return false
	}
	h := d.fleet.handle(qj.job.BrowserID)
	return h != nil && h.Native
}

// pickable returns the job the worker would take next, or nil when none may be
// taken. Caller must hold d.mu.
//
// While a lease is held or pending, native jobs are invisible to the worker but
// remote ones are not: the consumer borrowing the foreground must not stall
// CDP/BiDi traffic, which never touches the OS cursor. Skipping rather than
// head-blocking is the whole point — a remote job queued behind a native one
// would otherwise wait out the entire browser launch.
func (d *Dispatcher) pickable() *queuedJob {
	if d.queue.Len() == 0 {
		return nil
	}
	if d.leaseWanted == 0 {
		return d.queue.peek()
	}
	return d.queue.peekFunc(func(qj *queuedJob) bool { return !d.isNativeJob(qj) })
}

// popPickable removes and returns pickable(). Caller must hold d.mu.
func (d *Dispatcher) popPickable() *queuedJob {
	qj := d.pickable()
	if qj == nil {
		return nil
	}
	d.queue.remove(qj)
	return qj
}

// cdpWorker consumes parallel-safe jobs from the channel handed to it at spawn
// time (see spawnRemoteWorkerLocked: reading d.cdpJobs here would race abortAll).
//
// Three ways out, and the difference between them is the contract:
//   - quit: this one worker is surplus after a shrink. It is only ever observed
//     BETWEEN jobs, so the job it was running still completes.
//   - stop: the whole fleet is going down. Buffered work is cancelled so the
//     inflight WaitGroup balances.
//   - the channel is never closed, so there is no fourth way out.
func (d *Dispatcher) cdpWorker(jobs chan *queuedJob, stop <-chan struct{}, quit <-chan struct{}) {
	defer d.remoteWorkerDone()
	for {
		select {
		case qj := <-jobs:
			d.runJob(qj, false)
		case <-quit:
			return
		case <-stop:
			// Cancel whatever is still buffered so inflight balances. The
			// channel is deliberately never closed — see the field comment.
			for {
				select {
				case qj := <-jobs:
					d.completeJob(qj, JobResult{Status: StatusCancelled, Err: ErrFleetStopped})
				default:
					return
				}
			}
		}
	}
}
