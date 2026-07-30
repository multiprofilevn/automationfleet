package automationfleet

// Runtime sizing of the remote (CDP / BiDi) worker pool. Split out of
// dispatcher.go, which owns lifecycle and job execution.
//
// Two numbers, deliberately: remoteQuits is what the caller ASKED for, remoteLive
// is what is actually running. They differ exactly while a shrink is waiting for
// surplus workers to finish the jobs they are on — and reporting the intent as
// if it were reality would make "the pool is down to two" true before two of the
// eight in-flight jobs had finished.

import "fmt"

// setRemoteWorkers resizes the pool. n <= 0 is an error and changes nothing;
// after Stop / AbortAll it returns ErrFleetStopped without spawning anything.
//
// The whole operation runs under d.mu, in the same critical section abortAll
// uses to set stopped: that is what makes "a stopped fleet never spawns a
// worker" true even when a resize and an abort race.
func (d *Dispatcher) setRemoteWorkers(n int) error {
	if n <= 0 {
		return fmt.Errorf("automationfleet: SetRemoteWorkers: n must be > 0, got %d", n)
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.stopped {
		return ErrFleetStopped
	}
	d.remoteTarget = n
	if !d.started {
		// Nothing to spawn onto yet — cdpJobs is created by start(), which will
		// spawn remoteTarget workers.
		return nil
	}
	for len(d.remoteQuits) < n {
		d.spawnRemoteWorkerLocked()
	}
	for len(d.remoteQuits) > n {
		last := len(d.remoteQuits) - 1
		// Closing only asks. The worker is in runJob or blocked on the select;
		// either way it leaves between jobs, never mid-job.
		close(d.remoteQuits[last])
		d.remoteQuits = d.remoteQuits[:last]
	}
	return nil
}

// spawnRemoteWorkerLocked starts one worker and hands it the channels. Caller
// must hold d.mu — reading d.cdpJobs / d.stopCh here rather than in the worker
// is the whole point of capturing at spawn.
func (d *Dispatcher) spawnRemoteWorkerLocked() {
	quit := make(chan struct{})
	d.remoteQuits = append(d.remoteQuits, quit)
	d.remoteLive++
	d.wg.Add(1)
	go d.cdpWorker(d.cdpJobs, d.stopCh, quit)
}

// remoteWorkerDone is the single exit path of a remote worker.
func (d *Dispatcher) remoteWorkerDone() {
	d.mu.Lock()
	d.remoteLive--
	d.mu.Unlock()
	d.wg.Done()
}

// liveRemoteWorkers reports how many remote workers are running right now.
func (d *Dispatcher) liveRemoteWorkers() int {
	d.mu.Lock()
	defer d.mu.Unlock()
	if !d.started {
		return d.remoteTarget
	}
	return d.remoteLive
}
