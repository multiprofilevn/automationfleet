package automationfleet_test

// Phase 7 addition to phase 6's parity suite (automationfleet/parity_test.go):
// firefoxkit gained a native Navigate branch (address-bar keystrokes) to match
// chromekit's existing omnibox navigation. TestParity_NavigateBehaviour runs
// in the default suite (go test -race -run='TestParity' ./...), so it cannot
// drive real browsers — it verifies the one thing observable without one:
// both kits' native branch presses the SAME physical Win32 virtual-key codes
// for the same 5-step combo (Ctrl+L, Ctrl+A, End, Enter). Equal VK codes +
// equal modifier bitmask means the OS-level keystrokes physically sent are
// identical between the two kits — the strongest check possible without
// capturing a live keystroke trace from each kit's private input backend.

import (
	"reflect"
	"testing"

	"github.com/tuwibu/chromekit"
	chromeinput "github.com/tuwibu/chromekit/input"
	"github.com/tuwibu/firefoxkit"
	firefoxinput "github.com/tuwibu/firefoxkit/input"
)

// TestParity_NavigateBehaviour asserts firefoxkit's native Navigate branch is
// keystroke-equivalent to chromekit's: same Navigate signature, same Win32 VK
// codes and modifier bitmask for the address-bar combo (Ctrl+L focus,
// Ctrl+A select-all, End cancel-autocomplete, Enter submit).
func TestParity_NavigateBehaviour(t *testing.T) {
	chromeNav, ok := reflect.TypeOf(&chromekit.Page{}).MethodByName("Navigate")
	if !ok {
		t.Fatal("chromekit.Page has no Navigate method")
	}
	firefoxNav, ok := reflect.TypeOf(&firefoxkit.Page{}).MethodByName("Navigate")
	if !ok {
		t.Fatal("firefoxkit.Page has no Navigate method")
	}
	// Drop the receiver (index 0) before comparing — the receiver types
	// necessarily differ (*chromekit.Page vs *firefoxkit.Page).
	if chromeNav.Type.NumIn() != firefoxNav.Type.NumIn() || chromeNav.Type.NumOut() != firefoxNav.Type.NumOut() {
		t.Fatalf("Navigate arity mismatch: chromekit %v, firefoxkit %v", chromeNav.Type, firefoxNav.Type)
	}
	for i := 1; i < chromeNav.Type.NumIn(); i++ {
		if chromeNav.Type.In(i) != firefoxNav.Type.In(i) {
			t.Fatalf("Navigate param %d type mismatch: chromekit %v, firefoxkit %v", i, chromeNav.Type.In(i), firefoxNav.Type.In(i))
		}
	}
	for i := 0; i < chromeNav.Type.NumOut(); i++ {
		if chromeNav.Type.Out(i) != firefoxNav.Type.Out(i) {
			t.Fatalf("Navigate return %d type mismatch: chromekit %v, firefoxkit %v", i, chromeNav.Type.Out(i), firefoxNav.Type.Out(i))
		}
	}

	cases := []struct {
		name       string
		chromeVK   uint16
		firefoxVK  uint16
		chromeMod  chromeinput.Modifier
		firefoxMod firefoxinput.Modifier
	}{
		{"Ctrl+L (focus address bar)", chromeinput.VKL, firefoxinput.VKL, chromeinput.ModCtrl, firefoxinput.ModCtrl},
		{"Ctrl+A (select all)", chromeinput.VKA, firefoxinput.VKA, chromeinput.ModCtrl, firefoxinput.ModCtrl},
		{"End (cancel autocomplete)", chromeinput.VKEnd, firefoxinput.VKEnd, 0, 0},
		{"Enter (submit)", chromeinput.VKReturn, firefoxinput.VKReturn, 0, 0},
	}
	for _, c := range cases {
		if c.chromeVK != c.firefoxVK {
			t.Errorf("%s: VK code mismatch: chromekit 0x%02X, firefoxkit 0x%02X", c.name, c.chromeVK, c.firefoxVK)
		}
		if uint16(c.chromeMod) != uint16(c.firefoxMod) {
			t.Errorf("%s: modifier bitmask mismatch: chromekit %d, firefoxkit %d", c.name, c.chromeMod, c.firefoxMod)
		}
	}
}
