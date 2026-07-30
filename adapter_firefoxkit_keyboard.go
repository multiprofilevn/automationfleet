package automationfleet

import "github.com/tuwibu/firefoxkit"

type firefoxKeyboard struct{ k *firefoxkit.KeyboardAPI }

// Down/Up/Press/TypeHuman/TypeSelector drop the kit's optional trailing
// variadic (input.Modifier / input.HumanTypeConfig / TypeSelectorConfig) —
// see driver_keyboard.go for why those types can't be shared across kits.
func (k firefoxKeyboard) ClearInput(selector ...string) error { return k.k.ClearInput(selector...) }
func (k firefoxKeyboard) Down(key string) error               { return k.k.Down(key) }
func (k firefoxKeyboard) Up(key string) error                 { return k.k.Up(key) }
func (k firefoxKeyboard) Press(key string) error              { return k.k.Press(key) }
func (k firefoxKeyboard) Type(text string) error              { return k.k.Type(text) }
func (k firefoxKeyboard) TypeFast(text string) error          { return k.k.TypeFast(text) }
func (k firefoxKeyboard) TypeHuman(text string) error         { return k.k.TypeHuman(text) }
func (k firefoxKeyboard) TypeSelector(selector, text string) error {
	return k.k.TypeSelector(selector, text)
}
