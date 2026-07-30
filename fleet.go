package automationfleet

import (
	"context"
	"sync"
)

// Fleet is the public face of the orchestrator. Construct with New, register
// browsers, Submit jobs, Stop when done.
type Fleet struct {
	cfg    *config
	log    Logger
	ctx    context.Context
	cancel context.CancelFunc

	mu          sync.RWMutex
	handles     map[string]*BrowserHandle
	nativeCount int // # of registered handles with Native=true; gates the takeover hook
	stopped     bool
	stopOnce    sync.Once

	dispatcher *Dispatcher
	hotkeyDone chan struct{}

	// leaseToken is the single foreground permit, capacity 1 and pre-filled.
	// Whoever holds it owns the OS foreground: either one native critical
	// section (executeCriticalWithRetry) or one ForegroundLease. A channel
	// rather than a Mutex because it has to be selectable against ctx.Done and
	// releasable from a goroutine other than the one that took it.
	leaseToken chan struct{}

	// Mouse-takeover watchdog, installed only while ≥1 native browser is
	// registered. takeoverMu serializes install/uninstall independently of mu so
	// the OS hook syscalls never run under the registry lock.
	takeoverMu    sync.Mutex
	watchdog      *takeoverWatchdog
	hookUninstall func()
}

// New builds a Fleet. Call Start() before Submit.
func New(opts ...Option) *Fleet {
	cfg := defaultConfig()
	for _, opt := range opts {
		opt(cfg)
	}
	ctx, cancel := context.WithCancel(context.Background())
	f := &Fleet{
		cfg:        cfg,
		log:        cfg.logger,
		ctx:        ctx,
		cancel:     cancel,
		handles:    make(map[string]*BrowserHandle),
		leaseToken: make(chan struct{}, 1),
	}
	f.leaseToken <- struct{}{}
	f.dispatcher = newDispatcher(f)
	return f
}

// handle looks up a registered browser by id; returns nil when missing.
func (f *Fleet) handle(id string) *BrowserHandle {
	f.mu.RLock()
	defer f.mu.RUnlock()
	return f.handles[id]
}
