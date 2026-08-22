package automationfleet

import "github.com/multiprofilevn/chromekit"

type chromeMouse struct{ m *chromekit.MouseAPI }

func (m chromeMouse) MoveTo(x, y float64) error           { return m.m.MoveTo(x, y) }
func (m chromeMouse) ClickAt(x, y float64) error          { return m.m.ClickAt(x, y) }
func (m chromeMouse) Click(selector string) error         { return m.m.Click(selector) }
func (m chromeMouse) RightClick(selector string) error    { return m.m.RightClick(selector) }
func (m chromeMouse) MiddleClick(x, y float64) error      { return m.m.MiddleClick(x, y) }
func (m chromeMouse) DoubleClick(selector string) error   { return m.m.DoubleClick(selector) }
func (m chromeMouse) Hover(selector string) error         { return m.m.Hover(selector) }
func (m chromeMouse) Scroll(deltaX, deltaY float64) error { return m.m.Scroll(deltaX, deltaY) }
func (m chromeMouse) Drag(fromX, fromY, toX, toY float64) error {
	return m.m.Drag(fromX, fromY, toX, toY)
}
func (m chromeMouse) DragAndDrop(fromSel, toSel string) error {
	return m.m.DragAndDrop(fromSel, toSel)
}
func (m chromeMouse) Tap(x, y float64) error             { return m.m.Tap(x, y) }
func (m chromeMouse) FocusElement(selector string) error { return m.m.FocusElement(selector) }
