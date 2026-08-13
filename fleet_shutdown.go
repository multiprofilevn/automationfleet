package automationfleet

import "context"

// Stop requests shutdown and blocks until hotkeys, dispatcher workers, and
// every in-flight job have fully torn down. Idempotent.
func (f *Fleet) Stop() {
	_ = f.StopContext(context.Background())
}

// StopContext requests the same permanent shutdown as Stop, but bounds how
// long the caller waits for the full drain. A context error only stops this
// caller from waiting; shutdown continues in the background, and a later
// Stop/StopContext joins the same drain.
func (f *Fleet) StopContext(ctx context.Context) error {
	f.requestStop("stop")
	select {
	case <-f.shutdownDone:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// requestStop is the single funnel for shutting down the dispatcher.
// Idempotent via stopOnce — safe to call from hotkey, Stop, or AbortAll.
func (f *Fleet) requestStop(reason string) {
	f.stopOnce.Do(func() {
		// Start and the first stop request are mutually exclusive. Once this
		// section marks the dispatcher stopped, start/resize/enqueue cannot add
		// to either WaitGroup, so the waiter below is race-free.
		f.lifecycleMu.Lock()
		f.log.Infof("automationfleet: stop requested (%s)", reason)
		// abortAll owns the stopped state for BOTH structs — see its markStopped
		// call. Setting f.stopped here as well is what let the two flags drift:
		// Submit could reject while enqueue still accepted, or the reverse.
		f.dispatcher.abortAll(reason)
		hotkeyDone := f.hotkeyDone
		f.cancel()
		f.lifecycleMu.Unlock()

		// Potentially blocking OS-hook teardown and user callbacks belong to the
		// joined drain, never the request path. StopContext can therefore observe
		// its deadline even if either one stalls; Stop still waits for all of it.
		go func() {
			f.reconcileTakeover()
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
			if hotkeyDone != nil {
				<-hotkeyDone
			}
			f.dispatcher.wg.Wait()
			f.dispatcher.inflight.Wait()
			close(f.shutdownDone)
		}()
	})
}
