package automationfleet

import (
	"errors"
	"sync"
)

// Dispatcher owns the priority queue, the single native worker, and the CDP
// worker pool. One native worker is intentional — the OS has one cursor; any
// concurrency at this layer creates cross-window input races.
type Dispatcher struct {
	fleet *Fleet

	mu        sync.Mutex
	cond      *sync.Cond
	queue     priorityQueue
	insertSeq uint64

	started bool
	stopped bool
	// pauseReasons holds every active pause source (e.g. "ui", "hotkey",
	// "user-takeover"). The dispatcher is paused while the set is non-empty, so
	// one source resuming (e.g. the takeover watchdog after 10s idle) cannot
	// un-pause a fleet another source (manual UI/hotkey) is still holding.
	pauseReasons map[string]struct{}

	// leaseWanted counts callers that hold or are waiting for a foreground
	// lease. While it is non-zero the worker refuses to POP native jobs — but
	// keeps popping remote ones, which is why this is a pop filter rather than
	// a plain "stop the worker" flag. A head-blocking predicate would strand
	// every remote job queued behind one blocked native job.
	leaseWanted int

	inflight sync.WaitGroup

	// cdpJobs is created once at start and never closed or nil'd. It used to be
	// closed + set to nil by abortAll while nativeWorker read the field
	// unlocked; stopCh carries shutdown instead, and every worker captures the
	// channel at spawn.
	cdpJobs    chan *queuedJob
	stopCh     chan struct{}
	stopChOnce sync.Once
	wg         sync.WaitGroup

	// remoteQuits is the INTENDED remote pool: one quit channel per worker that
	// should be running. Growing appends and spawns, shrinking closes from the
	// tail — a closed quit only takes effect between jobs, which is what makes
	// a resize unable to cancel work in flight.
	remoteQuits []chan struct{}
	// remoteTarget is the size start() will spawn, and the size a resize before
	// start() records.
	remoteTarget int
	// remoteLive counts workers actually alive. It trails remoteQuits during a
	// shrink (a worker leaves only after its current job) and it, not the
	// intent, is what Snapshot reports.
	remoteLive int
}

// errCursorDrift signals the OS cursor moved during a critical section, most
// likely because a human grabbed the mouse mid-job. Retried up to
// cfg.driftRetries times with cfg.driftRetryDelay between attempts.
var errCursorDrift = errors.New("automationfleet: cursor drift detected")

// errWindowMoved signals the browser window itself moved between the moment the
// coordinate frame was pinned and the moment the cursor arrived — so the cursor
// is exactly where it was told to go, and that place is no longer the right one.
//
// Kept distinct from errCursorDrift on purpose. Drift means "something grabbed
// the mouse"; this means "the target moved". They call for different recovery:
// drift is retried as-is, a moved window has to be re-measured first, and
// reporting one as the other sends the retry loop back to a stale frame.
var errWindowMoved = errors.New("automationfleet: browser window moved mid-operation")

func newDispatcher(f *Fleet) *Dispatcher {
	d := &Dispatcher{
		fleet:        f,
		pauseReasons: make(map[string]struct{}),
		stopCh:       make(chan struct{}),
	}
	if f != nil {
		d.remoteTarget = f.cfg.cdpWorkers
	}
	d.cond = sync.NewCond(&d.mu)
	return d
}

// isPaused reports whether any pause reason is active. Caller must hold d.mu.
func (d *Dispatcher) isPaused() bool { return len(d.pauseReasons) > 0 }

// hasReason reports whether a specific pause source is currently active. Takes
// d.mu itself — safe to call from a concurrent status reader (e.g. the 1Hz tick).
func (d *Dispatcher) hasReason(reason string) bool {
	d.mu.Lock()
	defer d.mu.Unlock()
	_, ok := d.pauseReasons[reason]
	return ok
}
