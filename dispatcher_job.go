package automationfleet

import (
	"context"
	"errors"
	"time"
)

// runJob executes a single job under a per-job timeout context.
func (d *Dispatcher) runJob(qj *queuedJob, native bool) {
	ctx, cancel := context.WithTimeout(d.fleet.ctx, qj.job.Timeout)
	defer cancel()

	start := time.Now()
	handle := d.fleet.handle(qj.job.BrowserID)
	if handle == nil {
		d.completeJob(qj, JobResult{Status: StatusFailed, Err: ErrUnknownBrowser, Took: time.Since(start)})
		return
	}

	var err error
	if native {
		err = d.executeCriticalWithRetry(ctx, handle, qj.job.Action)
	} else {
		err = d.executeCDPOnly(ctx, handle, qj.job.Action)
	}

	status := StatusDone
	if err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			status = StatusCancelled
		} else {
			status = StatusFailed
		}
	}
	took := time.Since(start)
	d.fleet.log.Infof("automationfleet: job=%d browser=%s action=%s status=%s took=%s err=%v",
		qj.id, qj.job.BrowserID, qj.job.Action.kind(), status, took, err)
	d.completeJob(qj, JobResult{Status: status, Err: err, Took: took})
}

// executeCriticalWithRetry retries up to cfg.driftRetries times on cursor
// drift, sleeping cfg.driftRetryDelay between attempts to let a human nudging
// the mouse settle. Non-drift errors surface immediately. Total attempts =
// 1 (initial) + driftRetries.
func (d *Dispatcher) executeCriticalWithRetry(ctx context.Context, h *BrowserHandle, a Action) error {
	// The foreground token covers the WHOLE retry sequence, not one attempt.
	// Held only around executeCritical, the driftRetryDelay gap between two
	// attempts would be a hole a consumer could open a browser through — which
	// is exactly the window this phase exists to close.
	release, err := d.fleet.holdForeground(ctx)
	if err != nil {
		return err
	}
	defer release()

	err = executeCritical(ctx, d.fleet, h, a)
	// errWindowMoved rides the same path: a retry re-enters executeCritical,
	// which pins the coordinate frame at the top, so the re-measurement the
	// moved window needs falls out of the retry itself.
	for attempt := 1; attempt <= d.fleet.cfg.driftRetries &&
		(errors.Is(err, errCursorDrift) || errors.Is(err, errWindowMoved)); attempt++ {
		d.fleet.log.Warnf("automationfleet: %v on browser=%s action=%s — retry %d/%d", err, h.ID, a.kind(), attempt, d.fleet.cfg.driftRetries)
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(d.fleet.cfg.driftRetryDelay):
		}
		err = executeCritical(ctx, d.fleet, h, a)
	}
	return err
}

func (d *Dispatcher) completeJob(qj *queuedJob, r JobResult) {
	r.ID = qj.id
	r.BrowserID = qj.job.BrowserID
	if r.Took == 0 {
		r.Took = time.Since(qj.enqueued)
	}
	deliver(qj.result, r)
	d.inflight.Done()
}

func deliver(ch chan JobResult, r JobResult) {
	defer func() { _ = recover() }()
	select {
	case ch <- r:
	default:
	}
	close(ch)
}
