package automationfleet

import "github.com/multiprofilevn/firefoxkit"

type firefoxMouse struct{ m *firefoxkit.MouseAPI }

func (m firefoxMouse) MoveTo(x, y float64) error           { return m.m.MoveTo(x, y) }
func (m firefoxMouse) ClickAt(x, y float64) error          { return m.m.ClickAt(x, y) }
func (m firefoxMouse) Click(selector string) error         { return m.m.Click(selector) }
func (m firefoxMouse) RightClick(selector string) error    { return m.m.RightClick(selector) }
func (m firefoxMouse) MiddleClick(x, y float64) error      { return m.m.MiddleClick(x, y) }
func (m firefoxMouse) DoubleClick(selector string) error   { return m.m.DoubleClick(selector) }
func (m firefoxMouse) Hover(selector string) error         { return m.m.Hover(selector) }
func (m firefoxMouse) Scroll(deltaX, deltaY float64) error { return m.m.Scroll(deltaX, deltaY) }
func (m firefoxMouse) Drag(fromX, fromY, toX, toY float64) error {
	return m.m.Drag(fromX, fromY, toX, toY)
}
func (m firefoxMouse) DragAndDrop(fromSel, toSel string) error {
	return m.m.DragAndDrop(fromSel, toSel)
}
func (m firefoxMouse) Tap(x, y float64) error             { return m.m.Tap(x, y) }
func (m firefoxMouse) FocusElement(selector string) error { return m.m.FocusElement(selector) }
