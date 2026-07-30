package automationfleet

// Phase 3, R1 — Register (Test Spec 6-10).
//
// The input mode is derived from the kit, never declared. ExpectNative() /
// ExpectRemote() replace the two-way check the old BrowserHandle.validate()
// did, and they must fail LOUD: silently registering a browser the caller
// believed was native as a remote one strips the anti-bot guarantees that
// browser was launched for, and nothing downstream would report it.

import (
	"errors"
	"testing"
)

// 6. Mode comes from the kit, both ways.
func TestRegisterChrome_DerivesNativeFromKit(t *testing.T) {
	f := newTestFleet(t)

	if err := registerFake(f, "n1", true); err != nil {
		t.Fatalf("register native: %v", err)
	}
	if err := registerFake(f, "r1", false); err != nil {
		t.Fatalf("register remote: %v", err)
	}

	got, err := f.InputMode("n1")
	if err != nil {
		t.Fatalf("InputMode(n1): %v", err)
	}
	if got != InputNative {
		t.Errorf("InputMode(n1) = %v, want %v", got, InputNative)
	}

	got, err = f.InputMode("r1")
	if err != nil {
		t.Fatalf("InputMode(r1): %v", err)
	}
	if got != InputRemote {
		t.Errorf("InputMode(r1) = %v, want %v", got, InputRemote)
	}

	if _, err := f.InputMode("ghost"); !errors.Is(err, ErrUnknownBrowser) {
		t.Errorf("InputMode(ghost) = %v, want ErrUnknownBrowser", err)
	}
}

// 7. Launched remote, registered with ExpectNative() → error. The failure mode
// this pins is not "wrong error message", it is "registered anyway as remote".
func TestRegister_ExpectNativeMismatchFailsLoud(t *testing.T) {
	f := newTestFleet(t)

	err := registerFake(f, "x1", false, ExpectNative())
	if err == nil {
		t.Fatal("ExpectNative() on a remote browser was accepted — the fleet silently downgraded to remote, " +
			"which is exactly the anti-bot trap the old validate() blocked")
	}
	if registeredCount(f) != 0 {
		t.Errorf("mismatch left %d handle(s) in the registry — a rejected Register must register nothing", registeredCount(f))
	}
	if _, lookupErr := f.InputMode("x1"); !errors.Is(lookupErr, ErrUnknownBrowser) {
		t.Errorf("InputMode after rejected Register = %v, want ErrUnknownBrowser", lookupErr)
	}
}

// 8. The other direction: launched native, registered with ExpectRemote(). Just
// as loud — a native browser accepted as remote silently eats a capacity slot
// and routes OS input the caller did not ask for.
func TestRegister_ExpectRemoteMismatchFailsLoud(t *testing.T) {
	f := newTestFleet(t)

	if err := registerFake(f, "x2", true, ExpectRemote()); err == nil {
		t.Fatal("ExpectRemote() on a native browser was accepted — mismatch must fail, never fall through")
	}
	if registeredCount(f) != 0 {
		t.Errorf("mismatch left %d handle(s) in the registry", registeredCount(f))
	}
}

// 9. No expectation → derive, no error. Both matching expectations also pass,
// so the option is an assertion and not a filter.
func TestRegister_NoExpectationDerivesSilently(t *testing.T) {
	f := newTestFleet(t)

	if err := registerFake(f, "a", true); err != nil {
		t.Errorf("no option, native kit: %v", err)
	}
	if err := registerFake(f, "b", false); err != nil {
		t.Errorf("no option, remote kit: %v", err)
	}
	if err := registerFake(f, "c", true, ExpectNative()); err != nil {
		t.Errorf("matching ExpectNative(): %v", err)
	}
	if err := registerFake(f, "d", false, ExpectRemote()); err != nil {
		t.Errorf("matching ExpectRemote(): %v", err)
	}
	if registeredCount(f) != 4 {
		t.Errorf("registered %d handles, want 4", registeredCount(f))
	}
}

// 10. Duplicate id is rejected and the FIRST handle survives — an overwrite
// would strand every job already queued against the original driver.
func TestRegister_DuplicateID(t *testing.T) {
	f := newTestFleet(t)

	first, _ := newFakeHandle("dup", false)
	if err := f.Register(first); err != nil {
		t.Fatalf("first register: %v", err)
	}
	second, _ := newFakeHandle("dup", false)
	if err := f.Register(second); err == nil {
		t.Fatal("duplicate id was accepted")
	}
	if got := handleOf(f, "dup"); got != first {
		t.Error("duplicate Register overwrote the existing handle")
	}
}
