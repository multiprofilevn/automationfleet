package faketest

// ----------------------------------------------------------------- mouse ----

// MoveTo mirrors Mouse.MoveTo.
func (d *Driver) MoveTo(x, y float64) error {
	sleep(d.record("moveto", "", x, y))
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.moveErr
}

// ClickAt mirrors Mouse.ClickAt.
func (d *Driver) ClickAt(x, y float64) error {
	sleep(d.record("clickat", "", x, y))
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.clickErr
}

// Click mirrors Mouse.Click (selector-based, remote path).
func (d *Driver) Click(sel string) error {
	sleep(d.record("click", sel, 0, 0))
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.clickErr
}

// FocusElement mirrors Mouse.FocusElement.
func (d *Driver) FocusElement(sel string) error {
	sleep(d.record("focuselement", sel, 0, 0))
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.moveErr
}

// -------------------------------------------------- mouse (widened, phase 8) --
// See the comment above the widened Page block (driver_page.go) — same
// generic-error-injection rationale applies here.

func (d *Driver) RightClick(sel string) error {
	sleep(d.record("rightclick", sel, 0, 0))
	return d.extraErr("rightclick")
}

func (d *Driver) MiddleClick(x, y float64) error {
	sleep(d.record("middleclick", "", x, y))
	return d.extraErr("middleclick")
}

func (d *Driver) DoubleClick(sel string) error {
	sleep(d.record("doubleclick", sel, 0, 0))
	return d.extraErr("doubleclick")
}

func (d *Driver) Hover(sel string) error {
	sleep(d.record("hover", sel, 0, 0))
	return d.extraErr("hover")
}

func (d *Driver) Scroll(deltaX, deltaY float64) error {
	sleep(d.record("scroll", "", deltaX, deltaY))
	return d.extraErr("scroll")
}

func (d *Driver) Drag(fromX, fromY, toX, toY float64) error {
	sleep(d.record("drag", "", fromX, fromY))
	return d.extraErr("drag")
}

func (d *Driver) DragAndDrop(fromSel, toSel string) error {
	sleep(d.record("draganddrop", fromSel+"->"+toSel, 0, 0))
	return d.extraErr("draganddrop")
}

func (d *Driver) Tap(x, y float64) error {
	sleep(d.record("tap", "", x, y))
	return d.extraErr("tap")
}
