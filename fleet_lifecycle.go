package automationfleet

import "errors"

// Start spawns dispatcher workers and the hotkey listener (if any hotkey
// enabled). Idempotent — calling twice is a no-op.
func (f *Fleet) Start() {
	f.dispatcher.start()

	bindings := make([]HotkeyBinding, 0, 3)
	if f.cfg.pauseEnabled {
		bindings = append(bindings, HotkeyBinding{
			Hotkey: f.cfg.pauseHotkey,
			OnFire: func() { f.Pause("hotkey") },
		})
	}
	if f.cfg.resumeEnabled {
		bindings = append(bindings, HotkeyBinding{
			Hotkey: f.cfg.resumeHotkey,
			OnFire: func() { f.Resume("hotkey") },
		})
	}
	if f.cfg.stopEnabled {
		bindings = append(bindings, HotkeyBinding{
			Hotkey: f.cfg.stopHotkey,
			OnFire: func() { f.requestStop("hotkey") },
		})
	}
	if len(bindings) == 0 {
		return
	}
	f.hotkeyDone = make(chan struct{})
	go func() {
		defer close(f.hotkeyDone)
		if err := runHotkeyMultiListener(f.ctx, bindings); err != nil {
			f.log.Warnf("automationfleet: hotkey listener: %v", err)
		}
	}()
}

// Pause stops the dispatcher from popping new jobs. In-flight jobs continue
// to completion. Idempotent — calling twice fires OnPause once.
func (f *Fleet) Pause(reason string) {
	if f.dispatcher.pause(reason) {
		f.log.Infof("automationfleet: pause requested (%s)", reason)
		if f.cfg.onPause != nil {
			func() {
				defer func() {
					if r := recover(); r != nil {
						f.log.Errorf("automationfleet: OnPause panic: %v", r)
					}
				}()
				f.cfg.onPause(reason)
			}()
		}
	}
}

// Resume unblocks the dispatcher to pop pending jobs. Idempotent — fires
// OnResume only when transitioning from paused to running.
func (f *Fleet) Resume(reason string) {
	if f.dispatcher.resume(reason) {
		f.log.Infof("automationfleet: resume requested (%s)", reason)
		if f.cfg.onResume != nil {
			func() {
				defer func() {
					if r := recover(); r != nil {
						f.log.Errorf("automationfleet: OnResume panic: %v", r)
					}
				}()
				f.cfg.onResume(reason)
			}()
		}
	}
}

// Submit enqueues a job. Returns a buffered channel that receives exactly one
// JobResult — Done, Failed, Cancelled, or Rejected. The channel is closed
// after the result lands.
func (f *Fleet) Submit(j Job) (<-chan JobResult, error) {
	if j.Action == nil {
		return nil, errors.New("automationfleet: Job.Action required")
	}
	if err := j.Action.validate(); err != nil {
		return nil, err
	}
	f.mu.RLock()
	if f.stopped {
		f.mu.RUnlock()
		return nil, ErrFleetStopped
	}
	if _, ok := f.handles[j.BrowserID]; !ok {
		f.mu.RUnlock()
		return nil, ErrUnknownBrowser
	}
	f.mu.RUnlock()

	if j.Timeout <= 0 {
		j.Timeout = f.cfg.defaultTimeout
	}
	return f.dispatcher.enqueue(j), nil
}

// Stop cancels in-flight jobs, drains the queue, and tears down workers.
// Idempotent.
func (f *Fleet) Stop() {
	f.requestStop("stop")
	if f.hotkeyDone != nil {
		<-f.hotkeyDone
	}
}

// Wait blocks until the queue is fully drained (no in-flight, no pending).
// Useful for "submit N then wait" patterns. Returns immediately if stopped.
func (f *Fleet) Wait() {
	f.dispatcher.waitDrained()
}

// requestStop is the single funnel for shutting down the dispatcher.
// Idempotent via stopOnce — safe to call from hotkey, Stop, or AbortAll.
func (f *Fleet) requestStop(reason string) {
	f.stopOnce.Do(func() {
		f.log.Infof("automationfleet: stop requested (%s)", reason)
		// abortAll owns the stopped state for BOTH structs — see its markStopped
		// call. Setting f.stopped here as well is what let the two flags drift:
		// Submit could reject while enqueue still accepted, or the reverse.
		f.dispatcher.abortAll(reason)
		f.reconcileTakeover() // tear down the takeover hook now that we're stopped
		if f.cfg.onStop != nil {
			func() {
				defer func() {
					if r := recover(); r != nil {
						f.log.Errorf("automationfleet: OnStop panic: %v", r)
					}
				}()
				f.cfg.onStop(reason)
			}()
		}
		f.cancel()
	})
}

// Snapshot is a point-in-time view of takeover-relevant fleet state, built as a
// new public surface because automationfleet has no generic Stats API. HasNative
// reports whether ≥1 native browser is registered (the takeover hook only runs
// then); AutoPaused reports whether the takeover watchdog currently holds the
// fleet paused via the "user-takeover" reason. Safe to call concurrently (e.g.
// from a 1Hz status tick) — each field is read under its owning lock.
// The pre-existing fields keep their name AND type: the consumer maps this
// struct 1:1 into a Wails DTO, so renaming one is a breaking change disguised
// as a refactor. New fields are additive only.
type Snapshot struct {
	HasNative  bool
	AutoPaused bool

	// NativeUsed / NativeMax expose the cap without a second call.
	NativeUsed int
	NativeMax  int
	// RemoteWorkers is the live size of the remote pool, which
	// SetRemoteWorkers can change at any moment.
	RemoteWorkers int
}

// Snapshot returns the current takeover-relevant fleet state.
func (f *Fleet) Snapshot() Snapshot {
	f.mu.RLock()
	used, max := f.nativeCount, f.cfg.nativeCap
	f.mu.RUnlock()
	return Snapshot{
		HasNative:     used > 0,
		AutoPaused:    f.dispatcher.hasReason(ReasonUserTakeover),
		NativeUsed:    used,
		NativeMax:     max,
		RemoteWorkers: f.dispatcher.liveRemoteWorkers(),
	}
}
