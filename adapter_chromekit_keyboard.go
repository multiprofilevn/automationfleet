package automationfleet

import "github.com/multiprofilevn/chromekit"

type chromeKeyboard struct{ k *chromekit.KeyboardAPI }

// Down/Up/Press/TypeHuman/TypeSelector drop the kit's optional trailing
// variadic (input.Modifier / input.HumanTypeConfig / TypeSelectorConfig) —
// see driver_keyboard.go for why those types can't be shared across kits.
func (k chromeKeyboard) ClearInput(selector ...string) error { return k.k.ClearInput(selector...) }
func (k chromeKeyboard) Down(key string) error               { return k.k.Down(key) }
func (k chromeKeyboard) Up(key string) error                 { return k.k.Up(key) }
func (k chromeKeyboard) Press(key string) error              { return k.k.Press(key) }
func (k chromeKeyboard) Type(text string) error              { return k.k.Type(text) }
func (k chromeKeyboard) TypeFast(text string) error          { return k.k.TypeFast(text) }
func (k chromeKeyboard) TypeHuman(text string) error         { return k.k.TypeHuman(text) }
func (k chromeKeyboard) TypeSelector(selector, text string) error {
	return k.k.TypeSelector(selector, text)
}
