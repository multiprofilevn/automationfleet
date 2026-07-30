package automationfleet

// Keyboard mirrors the kits' KeyboardAPI surface. Both kits' Press/TypeHuman/
// TypeSelector accept an optional trailing variadic (input.Modifier /
// input.HumanTypeConfig / TypeSelectorConfig) whose type is package-specific
// and cannot be shared without leaking a kit type into this interface — the
// same rule driver.go documents for ElementCenter/QuerySelector. Rather than
// abstracting those option types, this interface drops the trailing variadic
// entirely: adapters call through with zero options (the kit's own default
// behavior). A caller needing kit-specific tuning uses the kit directly.
type Keyboard interface {
	// ClearInput selects (Ctrl+A) and deletes the focused element's content.
	// With no selector it acts on whatever currently has focus.
	ClearInput(selector ...string) error
	// Down presses key without releasing it.
	Down(key string) error
	// Up releases a previously pressed key.
	Up(key string) error
	// Press taps key (down then up).
	Press(key string) error
	// Type sends text via the protocol's key-event path, no artificial delay.
	Type(text string) error
	// TypeFast sends text with the minimum inter-key delay the backend allows.
	TypeFast(text string) error
	// TypeHuman sends text with randomized human-like inter-key timing.
	TypeHuman(text string) error
	// TypeSelector focuses selector, then types text into it.
	TypeSelector(selector, text string) error
}
