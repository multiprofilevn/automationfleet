//go:build windows

package automationfleet

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/multiprofilevn/automationfleet/internal/faketest"
)

// Blind retry is what duplicates content: losing the foreground at character 7
// of 20 and re-running the whole action leaves 27 characters in the field while
// the job still reports done. The fix is to compare the field against the
// TARGET STATE (before + text) read back over the protocol, then repair only
// when they differ.

// repairFixture builds a fleet + native handle driving a fake input field.
func repairFixture(t *testing.T, opts ...Option) (*Fleet, *BrowserHandle, *faketest.Driver) {
	t.Helper()
	base := []Option{WithPauseHotkeyDisabled(), WithResumeHotkeyDisabled()}
	f := New(append(base, opts...)...)
	d := faketest.New()
	h := &BrowserHandle{ID: "n1", Driver: fakeDriver{d}, Native: true}
	return f, h, d
}

// 17. Foreground lost AFTER the last character: the field already holds the
// target state, so the correct repair is to do nothing at all.
func TestTypeAction_RepairDetectsAlreadyComplete(t *testing.T) {
	f, h, d := repairFixture(t)
	const text = "hello"
	d.LoseForegroundAt(len(text), ErrForegroundLost, false)

	err := executeCritical(context.Background(), f, h, TypeAction{Selector: "#u", Text: text})

	if err != nil {
		t.Fatalf("err = %v, want nil (field already matches the target state)", err)
	}
	if got := d.Value(); got != text {
		t.Fatalf("field = %q, want %q", got, text)
	}
	if got := d.Count("typehuman"); got != 1 {
		t.Fatalf("retyped despite the field already being correct: typehuman = %d, want 1", got)
	}
	if got := d.Count("clearinput"); got != 0 {
		t.Fatalf("cleared a correct field: clearinput = %d, want 0", got)
	}
	if got := d.Count("readvalue"); got == 0 {
		t.Fatal("repair never read the field back — it cannot know whether a retype is needed")
	}
}

// 18. Foreground lost mid-word: the field is a prefix of the target, so the
// repair must CLEAR and retype the whole target. Appending would duplicate.
func TestTypeAction_RepairClearsAndRetypes(t *testing.T) {
	f, h, d := repairFixture(t)
	const text = "hello"
	d.LoseForegroundAt(3, ErrForegroundLost, false)

	err := executeCritical(context.Background(), f, h, TypeAction{Selector: "#u", Text: text})

	if err != nil {
		t.Fatalf("err = %v, want nil (repair should succeed on the second round)", err)
	}
	if got := d.Value(); got != text {
		t.Fatalf("field = %q, want exactly %q (no duplication, no truncation)", got, text)
	}
	if got := d.Count("clearinput"); got < 1 {
		t.Fatalf("repair appended instead of clearing: clearinput = %d, want >= 1", got)
	}
}

// 19. ClearFirst=false with a pre-existing value: the target state is
// before+text, so the repair must restore "abc"+"xyz" — comparing against
// "how much did I manage to type" would lose the pre-existing "abc".
func TestTypeAction_RepairPreservesPreexistingValue(t *testing.T) {
	f, h, d := repairFixture(t)
	d.SetValue("abc")
	d.LoseForegroundAt(2, ErrForegroundLost, false)

	err := executeCritical(context.Background(), f, h,
		TypeAction{Selector: "#u", Text: "xyz", ClearFirst: false})

	if err != nil {
		t.Fatalf("err = %v, want nil", err)
	}
	if got := d.Value(); got != "abcxyz" {
		t.Fatalf("field = %q, want %q", got, "abcxyz")
	}
}

// 20. Repair is bounded at two rounds. Giving up must report how many
// characters may have escaped — with credentials that bound is the whole point.
func TestTypeAction_RepairGivesUpAfterTwoRounds(t *testing.T) {
	f, h, d := repairFixture(t)
	d.LoseForegroundAt(3, ErrForegroundLost, true) // never recovers

	err := executeCritical(context.Background(), f, h, TypeAction{Selector: "#u", Text: "hello"})

	if err == nil {
		t.Fatal("err = nil, want failure after two exhausted repair rounds")
	}
	if !errors.Is(err, ErrForegroundLost) {
		t.Fatalf("err = %v, want it to wrap ErrForegroundLost", err)
	}
	var lost *ForegroundLostError
	if !errors.As(err, &lost) {
		t.Fatalf("err = %v, want a *ForegroundLostError carrying the escaped-character count", err)
	}
	if lost.Emitted <= 0 {
		t.Fatalf("ForegroundLostError.Emitted = %d, want the upper bound on escaped characters", lost.Emitted)
	}
	if got := d.Count("typehuman"); got != 3 {
		t.Fatalf("typehuman = %d, want 3 (initial + 2 repair rounds)", got)
	}
}

// 21. ErrForegroundLost must NOT ride the errCursorDrift retry path. Drift
// retry re-runs the whole action after a settle delay — exactly the blind
// retype that duplicates content. Measured by wall clock so the assertion does
// not depend on how the repair loop is factored internally.
func TestForegroundLost_NotRoutedToDriftRetry(t *testing.T) {
	const driftDelay = time.Second
	f, h, d := repairFixture(t, WithDriftRetries(3), WithDriftRetryDelay(driftDelay))
	d.LoseForegroundAt(3, ErrForegroundLost, true)

	start := time.Now()
	err := f.dispatcher.executeCriticalWithRetry(context.Background(), h,
		TypeAction{Selector: "#u", Text: "hello"})
	elapsed := time.Since(start)

	if err == nil {
		t.Fatal("err = nil, want failure")
	}
	var lost *ForegroundLostError
	if !errors.As(err, &lost) {
		t.Fatalf("err = %v, want *ForegroundLostError", err)
	}
	if elapsed >= driftDelay {
		t.Fatalf("took %s — ErrForegroundLost was routed through the drift-retry sleep", elapsed)
	}
}
