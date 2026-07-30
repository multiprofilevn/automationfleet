package automationfleet

import "time"

// Logger is what automationfleet writes diagnostic lines to. Wire your own (zap,
// zerolog, log/slog) or use NoopLogger. Keep it allocation-light — the
// dispatcher logs once per job.
type Logger interface {
	Infof(format string, args ...any)
	Warnf(format string, args ...any)
	Errorf(format string, args ...any)
}

// NoopLogger silently drops every line.
type NoopLogger struct{}

func (NoopLogger) Infof(string, ...any)  {}
func (NoopLogger) Warnf(string, ...any)  {}
func (NoopLogger) Errorf(string, ...any) {}

// Option configures New.
type Option func(*config)

type config struct {
	logger         Logger
	defaultTimeout time.Duration
	cdpWorkers     int

	// nativeCap bounds how many native handles may be registered at once,
	// counted across every kit. The limit is physical (one mouse, one
	// keyboard), so nine Chrome windows and nine Firefox windows are the same
	// nine slots.
	nativeCap int

	// Stop hotkey aborts every in-flight + pending job. Default DISABLED —
	// users who want abort semantics opt-in via WithStopHotkey.
	stopHotkey  Hotkey
	stopEnabled bool
	onStop      func(reason string)

	// Pause/Resume hotkey pair. Default Ctrl+F10 / Ctrl+F11. Pause drains
	// in-flight, then blocks the worker on a cond — resume unblocks.
	pauseHotkey   Hotkey
	pauseEnabled  bool
	onPause       func(reason string)
	resumeHotkey  Hotkey
	resumeEnabled bool
	onResume      func(reason string)

	driftThreshold int

	// driftRetries: số lần retry trên errCursorDrift (initial attempt KHÔNG
	// tính). Default 3 cho phép user nudge chuột vài lần mà không fall back.
	// driftRetryDelay: chờ giữa các attempt để cursor settle.
	driftRetries    int
	driftRetryDelay time.Duration

	// Foreground lease. leaseTimeout bounds how long a lease may be held
	// before onLeaseStuck fires; expiry does NOT release the lease.
	leaseTimeout time.Duration
	onLeaseStuck func(reason string, heldFor time.Duration)
}

// DefaultPauseHotkey is Ctrl+F10. F-keys + Ctrl rarely conflict with global
// shortcuts on Windows / VS Code / Chrome.
var DefaultPauseHotkey = Hotkey{Mods: ModCtrl, Key: KeyF10}

// DefaultResumeHotkey is Ctrl+F11.
var DefaultResumeHotkey = Hotkey{Mods: ModCtrl, Key: KeyF11}

func defaultConfig() *config {
	return &config{
		logger:         NoopLogger{},
		defaultTimeout: 10 * time.Second,
		cdpWorkers:     4,
		nativeCap:      DefaultNativeCapacity,

		stopHotkey:  DefaultStopHotkey,
		stopEnabled: false, // opt-in: stop is destructive (no resume)

		pauseHotkey:   DefaultPauseHotkey,
		pauseEnabled:  true,
		resumeHotkey:  DefaultResumeHotkey,
		resumeEnabled: true,

		driftThreshold:  5,
		driftRetries:    3,
		driftRetryDelay: 250 * time.Millisecond,

		leaseTimeout: DefaultLeaseTimeout,
	}
}

// DefaultNativeCapacity is the number of native handles a fleet accepts when
// WithNativeCapacity is not used. Nine is a machine-wide budget, not a
// per-kit one.
const DefaultNativeCapacity = 9
