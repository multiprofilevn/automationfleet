package automationfleet

import (
	"context"
	"sync"
	"time"
)

// DefaultLeaseTimeout bounds how long a foreground lease may be held before the
// fleet declares it stuck. Hitting it does NOT release the lease — auto-release
// would drop the exclusivity invariant precisely when it matters most (a cold
// Chrome start legitimately exceeding a minute). The fleet logs and escalates
// through OnLeaseStuck instead.
const DefaultLeaseTimeout = 60 * time.Second

// ForegroundLease is an exclusive borrow of the OS foreground. While one is
// held the dispatcher will not start a NEW native job, and AcquireForeground
// has already waited for any in-flight native job (including every drift
// retry) to finish. Remote (CDP/BiDi) jobs are unaffected and keep flowing.
//
// Release is idempotent and safe from any goroutine. Always defer it.
type ForegroundLease struct {
	fleet    *Fleet
	reason   string
	acquired time.Time

	once  sync.Once
	timer *time.Timer
}

// Reason returns the label passed to AcquireForeground.
func (l *ForegroundLease) Reason() string {
	if l == nil {
		return ""
	}
	return l.reason
}

// HeldFor reports how long the lease has been held.
func (l *ForegroundLease) HeldFor() time.Duration {
	if l == nil {
		return 0
	}
	return time.Since(l.acquired)
}

// AcquireForeground borrows the OS foreground for the caller: it blocks new
// native jobs, waits for any in-flight native job to finish, and returns a
// lease the caller must Release.
//
// It returns ctx.Err() if ctx is cancelled while waiting, and ErrFleetStopped
// once the fleet has stopped.
//
// Two mechanisms, because one native job has two states that both have to be
// covered. leaseWanted stops the worker POPPING new native jobs (remote ones
// keep flowing past it); the token channel is what waits out the job already
// running, since executeCriticalWithRetry holds that same token for its whole
// retry sequence.
func (f *Fleet) AcquireForeground(ctx context.Context, reason string) (*ForegroundLease, error) {
	if err := f.ctx.Err(); err != nil {
		return nil, ErrFleetStopped
	}
	d := f.dispatcher

	// Raise the flag BEFORE waiting for the token: a lease that only blocks
	// native jobs once it owns the token would let the worker start one more
	// while it waits, and then have to wait that one out too.
	d.mu.Lock()
	d.leaseWanted++
	d.mu.Unlock()

	acquired := false
	defer func() {
		if acquired {
			return
		}
		d.mu.Lock()
		d.leaseWanted--
		d.mu.Unlock()
		d.cond.Broadcast()
	}()

	select {
	case <-f.leaseToken:
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-f.ctx.Done():
		return nil, ErrFleetStopped
	}
	acquired = true

	l := &ForegroundLease{fleet: f, reason: reason, acquired: time.Now()}
	l.timer = time.AfterFunc(f.cfg.leaseTimeout, func() { l.reportStuck() })
	return l, nil
}

// reportStuck fires OnLeaseStuck. It deliberately does NOT release: dropping
// the lease on a timer would break exclusivity silently during a cold Chrome
// start that legitimately runs long — the one moment it is most needed.
func (l *ForegroundLease) reportStuck() {
	f := l.fleet
	if f == nil {
		return
	}
	held := l.HeldFor()
	f.log.Errorf("automationfleet: foreground lease %q held %s — still not released", l.reason, held)
	cb := f.cfg.onLeaseStuck
	if cb == nil {
		return
	}
	defer func() {
		if r := recover(); r != nil {
			f.log.Errorf("automationfleet: OnLeaseStuck panic: %v", r)
		}
	}()
	cb(l.reason, held)
}

// Release returns the foreground to the dispatcher. Idempotent — calling it
// more than once (or on a nil lease) is a no-op, never a panic.
func (l *ForegroundLease) Release() {
	if l == nil {
		return
	}
	l.once.Do(func() {
		if l.timer != nil {
			l.timer.Stop()
		}
		f := l.fleet
		if f == nil {
			return
		}
		d := f.dispatcher
		d.mu.Lock()
		if d.leaseWanted > 0 {
			d.leaseWanted--
		}
		d.mu.Unlock()
		// Non-blocking: the token is capacity 1 and this lease owns the only
		// one outstanding, so the send always succeeds. The default arm exists
		// so a double return can never wedge the caller.
		select {
		case f.leaseToken <- struct{}{}:
		default:
		}
		d.cond.Broadcast()
	})
}

// holdForeground takes the foreground token for one native critical section and
// returns the function that gives it back. It is the same token
// AcquireForeground competes for, which is what makes "wait for the in-flight
// native job" work without a second counter.
func (f *Fleet) holdForeground(ctx context.Context) (func(), error) {
	select {
	case <-f.leaseToken:
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-f.ctx.Done():
		return nil, ErrFleetStopped
	}
	var once sync.Once
	return func() {
		once.Do(func() {
			select {
			case f.leaseToken <- struct{}{}:
			default:
			}
		})
	}, nil
}

// OnLeaseStuck registers a callback fired when a lease outlives the lease
// timeout. The lease is still held when it fires — the callback exists so the
// owner can surface the stall, not so the fleet can paper over it.
func OnLeaseStuck(cb func(reason string, heldFor time.Duration)) Option {
	return func(c *config) { c.onLeaseStuck = cb }
}

// WithLeaseTimeout overrides DefaultLeaseTimeout.
func WithLeaseTimeout(d time.Duration) Option {
	return func(c *config) {
		if d > 0 {
			c.leaseTimeout = d
		}
	}
}
