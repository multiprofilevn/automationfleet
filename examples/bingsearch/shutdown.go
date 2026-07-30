package bingsearch

import (
	"context"
	stdlog "log"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"
)

// CloserTimeout bounds how long a single teardown step (closing one browser,
// removing one temp dir) may run before Shutdown gives up on it and moves to
// the next step — a browser process that refuses to close must never hang
// process exit forever.
const CloserTimeout = 10 * time.Second

// Shutdown collects teardown steps as they're created (one per launched
// browser, plus the fleet and any temp profile dir) and runs them all —
// LIFO, each bounded by CloserTimeout, exactly once — whether that's
// triggered by the normal end of RunMain's run() or by Ctrl+C.
//
// This exists because killing the process used to orphan every
// already-launched browser: nothing trapped SIGINT, so a Ctrl+C on a
// 25-window run left 25 (or with two kits at once, 400) stray browser
// processes for the operator to kill by hand.
type Shutdown struct {
	mu      sync.Mutex
	once    sync.Once
	closers []shutdownStep
}

type shutdownStep struct {
	label string
	fn    func()
}

// NewShutdown returns an empty teardown registry.
func NewShutdown() *Shutdown { return &Shutdown{} }

// Track registers one teardown step. Steps run in LIFO order (most recently
// added closes first) when Run fires — mirrors typical defer unwind order,
// so e.g. browsers close before the fleet they're registered to stops.
func (s *Shutdown) Track(label string, fn func()) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.closers = append(s.closers, shutdownStep{label, fn})
}

// Run executes every tracked step exactly once, most-recently-added first,
// printing each label before it runs so a slow shutdown (closing 25 browser
// processes takes a moment) reads as progress rather than a hang. Safe to
// call more than once — e.g. once from RunMain's normal path and once from a
// concurrent signal handler racing it — only the first call does anything.
func (s *Shutdown) Run() {
	s.once.Do(func() {
		s.mu.Lock()
		steps := s.closers
		s.mu.Unlock()

		for i := len(steps) - 1; i >= 0; i-- {
			step := steps[i]
			stdlog.Printf("shutdown: closing %s", step.label)
			done := make(chan struct{})
			go func() {
				defer close(done)
				step.fn()
			}()
			select {
			case <-done:
			case <-time.After(CloserTimeout):
				stdlog.Printf("shutdown: %s did not close within %s — abandoning it and continuing", step.label, CloserTimeout)
			}
		}
	})
}

// RunMain is the shared entrypoint for all four Bing examples. It installs a
// SIGINT/SIGTERM trap, hands run a cancellable context (cancelled the moment
// a signal arrives — see AcquireForeground/AllowRealInput call sites, which
// already select on ctx.Done for exactly this reason) and a Shutdown registry
// for run to populate as it launches browsers, and guarantees exactly one
// teardown pass runs before process exit: on a returned error, on normal
// completion, or on Ctrl+C.
//
// Exit code: 0 on success, 1 if run returns an error OR a signal interrupted
// it — a killed run is not a clean run, so it must not report success to the
// shell.
func RunMain(run func(ctx context.Context, sd *Shutdown) error) {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	sd := NewShutdown()

	runDone := make(chan error, 1)
	go func() { runDone <- run(ctx, sd) }()

	select {
	case err := <-runDone:
		sd.Run()
		if err != nil {
			stdlog.Fatal(err)
		}
	case <-ctx.Done():
		stdlog.Print("interrupt received — waiting briefly for the in-flight step to notice, then closing every launched browser")
		select {
		case <-runDone:
		case <-time.After(5 * time.Second):
			stdlog.Print("shutdown: in-flight step did not return in time — closing browsers anyway")
		}
		sd.Run()
		os.Exit(1)
	}
}
