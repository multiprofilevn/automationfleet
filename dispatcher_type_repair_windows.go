//go:build windows

package automationfleet

import (
	"context"
	"errors"
	"fmt"
	"time"
	"unicode/utf8"

	"github.com/multiprofilevn/automationfleet/internal/winapi"
)

// maxRepairRounds bounds verify-then-repair. Two rounds is not a tuning knob:
// if the foreground is being taken twice in a row, something else on the
// machine is fighting for it and a third retype only spills more characters.
const maxRepairRounds = 2

// typeWithRepair runs a TypeAction and, when the foreground is lost mid-string,
// repairs against the TARGET STATE instead of retrying blindly.
//
// Blind retry is what duplicates content: losing the foreground at character 7
// of 20 and re-running the action leaves 27 characters in the field while the
// job still reports done. The ground truth is the field itself, read back over
// the protocol — reading never needs the keyboard, so it stays available
// exactly when the keyboard is not.
//
// The comparison is against before+text, not against "how many characters did I
// manage to send". That is what makes ClearFirst=false correct: a pre-existing
// "abc" plus "xyz" must end as "abcxyz", and a repair keyed off the typed count
// would drop the "abc".
func typeWithRepair(ctx context.Context, f *Fleet, h *BrowserHandle, page Page, act TypeAction) error {
	x, y, err := page.ElementCenter(act.Selector)
	if err != nil {
		return err
	}
	if err := clickAt(ctx, f, h, page, x, y); err != nil {
		return err
	}
	// brief settle so focus lands inside the input element
	time.Sleep(80 * time.Millisecond)

	// Snapshot BEFORE any keystroke. A read that fails is fatal here on
	// purpose: without ground truth the only recovery left is the blind
	// retype this function exists to avoid.
	before, err := page.ReadValue(act.Selector)
	if err != nil {
		return fmt.Errorf("readValue: %w", err)
	}
	if act.ClearFirst {
		before = ""
	}
	want := before + act.Text

	prevLayout, layoutErr := winapi.ForceENUSLayout()
	if layoutErr != nil {
		f.log.Warnf("automationfleet: layout switch failed: %v (typing anyway)", layoutErr)
	}
	defer winapi.RestoreLayout(prevLayout)

	if act.ClearFirst {
		// Already focused after clickAt — ClearInput w/o selector does
		// Ctrl+A + Delete on the focused element.
		if err := page.Keyboard().ClearInput(); err != nil {
			return fmt.Errorf("clearInput: %w", err)
		}
	}
	// TypeHuman: 80-220ms/char + 5% typo (backspace + retype). Mechanical
	// Keyboard.Type 50-150ms không có typo → anti-bot detect.
	err = page.Keyboard().TypeHuman(act.Text)
	emitted := utf8.RuneCountInString(act.Text)

	for round := 1; err != nil && errors.Is(err, ErrForegroundLost); round++ {
		if round > maxRepairRounds {
			return &ForegroundLostError{Op: "type", Emitted: emitted}
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		// Take the foreground back before touching the keyboard again —
		// otherwise the repair types into the same foreign window.
		if ferr := h.Driver.Focus(ctx); ferr != nil {
			return fmt.Errorf("repair focus: %w", ferr)
		}
		now, rerr := page.ReadValue(act.Selector)
		if rerr != nil {
			return fmt.Errorf("repair readValue: %w", rerr)
		}
		if now == want {
			// The tail landed before the guard noticed. Retyping here is the
			// duplication bug, not the fix for it.
			f.log.Warnf("automationfleet: foreground lost on browser=%s but the field already holds the target value — no retype", h.ID)
			return nil
		}
		if cerr := page.Keyboard().ClearInput(); cerr != nil {
			return fmt.Errorf("repair clearInput: %w", cerr)
		}
		err = page.Keyboard().TypeHuman(want)
		emitted += utf8.RuneCountInString(want)
	}
	return err
}
