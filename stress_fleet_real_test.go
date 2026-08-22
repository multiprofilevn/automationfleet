//go:build integration && windows

package automationfleet

// ============================================================================
// PHASE-5 TIER-2 STRESS — REAL BROWSERS, REAL MOUSE, REAL KEYBOARD.
//
// ⚠ NEVER run this on a machine with sensitive windows open. Every test here
// drives the physical cursor and keyboard. Two gates guard it:
//   1. build tag `integration` (this file is compile-only for CI:
//      `go vet -tags=integration ./...`)
//   2. env FLEET_ALLOW_REAL_INPUT=1 — absent → every test t.Skip()s.
//
// Run deliberately:
//   FLEET_ALLOW_REAL_INPUT=1 go test -tags=integration -run 'TestStressReal' -timeout=30m ./...
// ============================================================================

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/multiprofilevn/chromekit"
)

// 13. THE scenario of the whole phase: a new browser is launched while a Type
// job is mid-string. Without the lease the keystrokes leak into browser B;
// with it the launch waits and browser A ends 100% correct.
func TestStressReal_LaunchDuringType(t *testing.T) {
	requireRealInput(t)
	useRealOSSeams(t)
	const text = "the-quick-brown-fox-jumps-over-the-lazy-dog-0123456789-abcd" // 60 chars

	t.Run("WithLease", func(t *testing.T) {
		f := newTestFleet(t)
		launchChromeNative(t, f, "A", 0, 0)
		pageA := navigateHandle(t, f, "A", typingPage)
		f.Start()

		launched := make(chan *chromekit.Browser, 1)
		go func() {
			// Poll until typing has genuinely begun, then try to launch B
			// UNDER a lease — the acquire must block until the type finishes.
			deadline := time.Now().Add(30 * time.Second)
			for time.Now().Before(deadline) {
				if v, err := pageA.ReadValue("#field"); err == nil && v != "" && v != text {
					break
				}
				time.Sleep(50 * time.Millisecond)
			}
			lease, err := f.AcquireForeground(context.Background(), "launch-B")
			if err != nil {
				launched <- nil
				return
			}
			defer lease.Release()
			b, err := chromekit.Launch(resolveChromeExe(t),
				chromekit.WithHeadless(false), chromekit.WithUserDataDir(t.TempDir()))
			if err != nil {
				launched <- nil
				return
			}
			launched <- b
		}()

		r := resultWithin(t, mustSubmit(t, f, Job{BrowserID: "A", Timeout: 60 * time.Second,
			Action: TypeAction{Selector: "#field", Text: text, ClearFirst: true}}),
			90*time.Second, "type under leased launch")
		if r.Status != StatusDone {
			t.Fatalf("type job: status=%s err=%v", r.Status, r.Err)
		}
		got, err := pageA.ReadValue("#field")
		if err != nil {
			t.Fatalf("readValue A: %v", err)
		}
		if got != text {
			t.Fatalf("browser A field = %q, want exactly %q — lease did not protect the type", got, text)
		}
		if b := <-launched; b != nil {
			b.Close()
		} else {
			t.Fatal("leased launch of browser B never completed")
		}
	})

	t.Run("NoLeaseNeverCorruptsSilently", func(t *testing.T) {
		f := newTestFleet(t)
		launchChromeNative(t, f, "A", 0, 0)
		pageA := navigateHandle(t, f, "A", typingPage)
		f.Start()

		go func() {
			deadline := time.Now().Add(30 * time.Second)
			for time.Now().Before(deadline) {
				if v, err := pageA.ReadValue("#field"); err == nil && v != "" && v != text {
					break
				}
				time.Sleep(50 * time.Millisecond)
			}
			// Deliberately NO lease: this is the user bug. Focus theft happens here.
			if b, err := chromekit.Launch(resolveChromeExe(t),
				chromekit.WithHeadless(false), chromekit.WithUserDataDir(t.TempDir())); err == nil {
				defer b.Close()
				time.Sleep(5 * time.Second) // hold the stolen foreground a while
			}
		}()

		r := resultWithin(t, mustSubmit(t, f, Job{BrowserID: "A", Timeout: 60 * time.Second,
			Action: TypeAction{Selector: "#field", Text: text, ClearFirst: true}}),
			90*time.Second, "type under unleased launch")
		got, err := pageA.ReadValue("#field")
		if err != nil {
			t.Fatalf("readValue A: %v", err)
		}
		// Phase-1 contract: EITHER the repair path lands the exact value
		// (Done + exact) OR the job fails loudly with ErrForegroundLost.
		// "Done with wrong/duplicated content" is the silent corruption this
		// test exists to catch (it is what pre-phase-1 code did).
		switch r.Status {
		case StatusDone:
			if got != text {
				t.Fatalf("SILENT CORRUPTION: job Done but field = %q, want %q", got, text)
			}
		case StatusFailed:
			if !errors.Is(r.Err, ErrForegroundLost) {
				t.Fatalf("failed with %v, want ErrForegroundLost", r.Err)
			}
		default:
			t.Fatalf("unexpected status %s (err %v)", r.Status, r.Err)
		}
	})
}

// 14. The window is dragged 300px while 5 click jobs sit in the queue. Every
// later job must re-pin the frame and still hit the target — the page's own
// counter is the ground truth for where the OS actually clicked.
func TestStressReal_WindowMovedMidQueue(t *testing.T) {
	requireRealInput(t)
	useRealOSSeams(t)
	f := newTestFleet(t)
	launchChromeNative(t, f, "A", 100, 100)
	pageA := navigateHandle(t, f, "A", clickRecorderPage)
	f.Start()

	f.Pause("stage")
	chans := make([]<-chan JobResult, 5)
	for i := range chans {
		chans[i] = mustSubmit(t, f, Job{BrowserID: "A", Timeout: 30 * time.Second,
			Action: ClickAction{Selector: "#target"}})
	}
	hwnd, err := f.handle("A").Driver.HWND()
	if err != nil || hwnd == 0 {
		t.Fatalf("HWND of A: %v (0x%x)", err, hwnd)
	}
	moveWindowTo(t, hwnd, 400, 150) // +300px while the jobs are queued
	f.Resume("stage")

	for i, ch := range chans {
		if r := resultWithin(t, ch, 30*time.Second, fmt.Sprintf("click %d after move", i)); r.Status != StatusDone {
			t.Fatalf("click %d after window move: status=%s err=%v", i, r.Status, r.Err)
		}
	}
	waitFor(t, 10*time.Second, "page click counter", func() bool {
		v, err := pageA.ReadValue("#count")
		return err == nil && v == "5"
	})
}
