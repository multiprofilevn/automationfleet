package automationfleet

import "time"

func WithLogger(l Logger) Option {
	return func(c *config) {
		if l != nil {
			c.logger = l
		}
	}
}

func WithDefaultTimeout(d time.Duration) Option {
	return func(c *config) {
		if d > 0 {
			c.defaultTimeout = d
		}
	}
}

// WithRemoteWorkers sets the initial size of the remote (CDP / BiDi) worker
// pool. Change it later with Fleet.SetRemoteWorkers. n <= 0 keeps the default.
func WithRemoteWorkers(n int) Option {
	return func(c *config) {
		if n > 0 {
			c.cdpWorkers = n
		}
	}
}

// WithCDPWorkers is the former name of WithRemoteWorkers.
//
// Deprecated: use WithRemoteWorkers.
func WithCDPWorkers(n int) Option { return WithRemoteWorkers(n) }

// WithNativeCapacity caps how many native handles may be registered at once.
// n <= 0 restores DefaultNativeCapacity. The cap is shared across kits.
func WithNativeCapacity(n int) Option {
	return func(c *config) {
		if n <= 0 {
			c.nativeCap = DefaultNativeCapacity
			return
		}
		c.nativeCap = n
	}
}

// WithStopHotkey enables the abort combo (default Ctrl+Alt+Shift+S) and
// overrides the key. Stop is destructive — fires AbortAll, no resume.
func WithStopHotkey(h Hotkey) Option {
	return func(c *config) {
		c.stopHotkey = h
		c.stopEnabled = true
	}
}

// WithStopHotkeyDisabled is now a no-op (stop is disabled by default).
// Kept for backwards-compat with existing examples.
func WithStopHotkeyDisabled() Option {
	return func(c *config) { c.stopEnabled = false }
}

// WithPauseHotkey overrides the default Ctrl+F10 pause combo.
func WithPauseHotkey(h Hotkey) Option {
	return func(c *config) {
		c.pauseHotkey = h
		c.pauseEnabled = true
	}
}

// WithPauseHotkeyDisabled disables the pause hotkey listener.
func WithPauseHotkeyDisabled() Option {
	return func(c *config) { c.pauseEnabled = false }
}

// WithResumeHotkey overrides the default Ctrl+F11 resume combo.
func WithResumeHotkey(h Hotkey) Option {
	return func(c *config) {
		c.resumeHotkey = h
		c.resumeEnabled = true
	}
}

// WithResumeHotkeyDisabled disables the resume hotkey listener.
func WithResumeHotkeyDisabled() Option {
	return func(c *config) { c.resumeEnabled = false }
}

// OnStop registers a callback fired when AbortAll runs (hotkey, manual, or
// fleet shutdown).
func OnStop(cb func(reason string)) Option {
	return func(c *config) { c.onStop = cb }
}

// OnPause registers a callback fired when the pause hotkey hits or Pause()
// is called. The callback runs on the listener goroutine — keep it short.
func OnPause(cb func(reason string)) Option {
	return func(c *config) { c.onPause = cb }
}

// OnResume registers a callback fired when the resume hotkey hits or
// Resume() is called.
func OnResume(cb func(reason string)) Option {
	return func(c *config) { c.onResume = cb }
}

// WithDriftThresholdPx tunes the cursor-drift guard. If the OS cursor moves
// further than this between MoveTo and the next op, the dispatcher assumes
// human interference and retries the job.
func WithDriftThresholdPx(px int) Option {
	return func(c *config) {
		if px > 0 {
			c.driftThreshold = px
		}
	}
}

// WithDriftRetries sets max retries on cursor drift (default 3). Initial
// attempt KHÔNG tính — total = 1 + n. Set 0 để disable retry (legacy single-shot).
func WithDriftRetries(n int) Option {
	return func(c *config) {
		if n >= 0 {
			c.driftRetries = n
		}
	}
}

// WithDriftRetryDelay set thời gian chờ giữa 2 attempt drift retry (default
// 250ms). Cho cursor settle khi user đang nudge.
func WithDriftRetryDelay(d time.Duration) Option {
	return func(c *config) {
		if d > 0 {
			c.driftRetryDelay = d
		}
	}
}
