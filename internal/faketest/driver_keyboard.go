package faketest

import (
	"errors"
	"time"
)

// -------------------------------------------------------------- keyboard ----

// ClearInput mirrors Keyboard.ClearInput — Ctrl+A + Delete on the focused
// element, modelled here as emptying the field.
func (d *Driver) ClearInput(sel ...string) error {
	arg := ""
	if len(sel) > 0 {
		arg = sel[0]
	}
	sleep(d.record("clearinput", arg, 0, 0))
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.clearErr != nil {
		return d.clearErr
	}
	d.value = ""
	return nil
}

// TypeHuman mirrors Keyboard.TypeHuman: characters land in the field one at a
// time. When the foreground-loss simulation is armed, the call returns after
// exactly loseAt characters have already been written — modelling the real
// failure where part of the text has escaped before the guard notices.
func (d *Driver) TypeHuman(text string) error {
	sleep(d.record("typehuman", text, 0, 0))

	d.mu.Lock()
	perChar, ok := d.perOp["key"]
	if !ok {
		perChar = 0
	}
	d.mu.Unlock()

	emitted := 0
	for _, r := range text {
		d.mu.Lock()
		d.value += string(r)
		d.counts["key"]++
		d.events = append(d.events, Event{Op: "key", Arg: string(r), At: time.Now()})
		emitted++
		lose := d.loseArmed && d.loseAt > 0 && emitted >= d.loseAt
		loseErr := d.loseErr
		if lose && !d.loseAlways {
			d.loseArmed = false
		}
		onKey := d.onKey
		d.mu.Unlock()

		if onKey != nil {
			onKey(emitted)
		}
		if lose {
			if loseErr == nil {
				loseErr = errors.New("faketest: foreground lost")
			}
			return loseErr
		}
		sleep(perChar)
	}
	return nil
}

// -------------------------------------------------- keyboard (widened, phase 8) --
// See the comment above the widened Page block (driver_page.go) — same
// generic-error-injection rationale applies here.

func (d *Driver) Down(key string) error {
	sleep(d.record("down", key, 0, 0))
	return d.extraErr("down")
}

func (d *Driver) Up(key string) error {
	sleep(d.record("up", key, 0, 0))
	return d.extraErr("up")
}

func (d *Driver) Press(key string) error {
	sleep(d.record("press", key, 0, 0))
	return d.extraErr("press")
}

func (d *Driver) Type(text string) error {
	sleep(d.record("type", text, 0, 0))
	return d.extraErr("type")
}

func (d *Driver) TypeFast(text string) error {
	sleep(d.record("typefast", text, 0, 0))
	return d.extraErr("typefast")
}

func (d *Driver) TypeSelector(selector, text string) error {
	sleep(d.record("typeselector", selector+"/"+text, 0, 0))
	return d.extraErr("typeselector")
}
