//go:build windows

package automationfleet

// Phase-5 tier-1 stress: scenario 12 — foreground loss injected randomly
// across 500 TypeActions; no field may ever end with duplicated content.
// 10 independent fleets run their 50 jobs in parallel (the 80ms focus-settle
// inside typeWithRepair makes 500 serial jobs blow the -count=10 budget; the
// native worker is serial PER FLEET, so parallel fleets change nothing about
// the interleaving under test). Seeds are fixed — deterministic under -race.

import (
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"math/rand"
)

func TestStress_ForegroundLostRepairNoDuplication(t *testing.T) {
	const fleets, perFleet = 10, 50 // 500 TypeActions total
	var wg sync.WaitGroup
	for fi := 0; fi < fleets; fi++ {
		wg.Add(1)
		go func(fi int) {
			defer wg.Done()
			rng := rand.New(rand.NewSource(int64(1000 + fi)))
			f := New(WithPauseHotkeyDisabled(), WithResumeHotkeyDisabled())
			defer func() {
				f.Stop()
				f.dispatcher.wg.Wait()
			}()
			d := addFake(f, "n1", true)
			f.Start()

			for i := 0; i < perFleet; i++ {
				text := randText(rng, 4+rng.Intn(5))
				k := 1 + rng.Intn(len(text))
				mode := rng.Intn(4)
				switch mode {
				case 0: // no loss
					d.LoseForegroundAt(0, nil, false)
				case 1: // lose once mid-string → repair must retype exactly once
					d.LoseForegroundAt(k, ErrForegroundLost, false)
				case 2: // lose EVERY time → repair must give up cleanly after 2 rounds
					d.LoseForegroundAt(k, ErrForegroundLost, true)
				case 3: // lose AFTER the last char — the field is already correct;
					// a blind retype here IS the duplication bug
					d.LoseForegroundAt(len(text), ErrForegroundLost, false)
				}
				keysBefore := d.Count("key")

				ch, err := f.Submit(Job{BrowserID: "n1", Timeout: 15 * time.Second,
					Action: TypeAction{Selector: "#f", Text: text, ClearFirst: true}})
				if err != nil {
					t.Errorf("fleet %d job %d submit: %v", fi, i, err)
					return
				}
				var r JobResult
				select {
				case r = <-ch:
				case <-time.After(20 * time.Second):
					t.Errorf("fleet %d job %d: result timeout (mode %d)", fi, i, mode)
					return
				}
				got := d.Value()
				switch r.Status {
				case StatusDone:
					if got != text {
						t.Errorf("fleet %d job %d (mode %d, k %d): Done but field = %q, want exactly %q — duplicated or torn content",
							fi, i, mode, k, got, text)
						return
					}
					if mode == 3 {
						// Already-complete detection: NOT one extra keystroke
						// beyond the original text may have been emitted.
						if delta := d.Count("key") - keysBefore; delta != len(text) {
							t.Errorf("fleet %d job %d: foreground lost after last char but %d keys emitted for %d-char text — blind retype (duplication)",
								fi, i, delta, len(text))
							return
						}
					}
				case StatusFailed:
					if !errors.Is(r.Err, ErrForegroundLost) {
						t.Errorf("fleet %d job %d (mode %d): failed with %v, want ErrForegroundLost", fi, i, mode, r.Err)
						return
					}
					// A clean failure may leave a partial value, but NEVER a
					// duplicated one: whatever is in the field must be a prefix
					// of the intended text.
					if !strings.HasPrefix(text, got) {
						t.Errorf("fleet %d job %d (mode %d, k %d): failed job left %q which is not a prefix of %q — duplicated/foreign characters",
							fi, i, mode, k, got, text)
						return
					}
				default:
					t.Errorf("fleet %d job %d: unexpected status %s (err %v)", fi, i, r.Status, r.Err)
					return
				}
			}
		}(fi)
	}
	wg.Wait()
}
