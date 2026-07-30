// parity_test.go — reflect-based conformance between chromekit and
// firefoxkit's public API. automationfleet is the only module that imports
// both kits, so this is the only place these checks can run — see
// plans/260729-2027-native-foreground-and-kit-parity/phase-06-chromekit-superset-parity.md,
// Test Spec items 1-3.
//
// Direction is one-way: chromekit's method set must be a SUPERSET of
// firefoxkit's, module for the exemptions declared below. firefoxkit having
// extra methods chromekit lacks is fine and not checked here.
package automationfleet

import (
	"reflect"
	"sort"
	"strconv"
	"strings"
	"testing"

	"github.com/tuwibu/chromekit"
	"github.com/tuwibu/firefoxkit"
)

// exemption documents one deliberate, permanent parity divergence. Every
// entry MUST carry a real reason — TestParity_ExemptionsHaveReasons rejects
// "temporary" / "TODO" language so debt can't hide here silently.
type exemption struct {
	typ    string // "Page" | "MouseAPI" | "KeyboardAPI" | "Browser"
	method string
	reason string
}

// methodSetExemptions — firefoxkit has the method, chromekit deliberately
// does NOT add it. Only consulted by TestParity_*MethodSets.
var methodSetExemptions = []exemption{
	{"Browser", "InstallWebExtension", "Firefox-only WebExtension install API — chromekit automates via CDP, not browser extensions; there is no Chrome equivalent to wrap"},
	{"Browser", "SetWindowBounds", "Firefox-only BiDi window-bounds RPC — chromekit moves/sizes the OS window via native win32 APIs (SetHWND + the native input backend) instead of a protocol call"},
}

// signatureExemptions — the method exists in both kits under the same name,
// but the shape is intentionally different. These are the "Cố ý KHÔNG đồng
// bộ" divergences from phase-06: chromekit keeps *cdp.Node instead of
// introducing firefoxkit's *ElementHandle, which would break every existing
// QuerySelector caller for no benefit the fleet needs (WaitForSelector +
// ElementCenter already cover element-location for the fleet — see
// driver.go).
var signatureExemptions = []exemption{
	{"Page", "QuerySelector", "chromekit returns *cdp.Node, firefoxkit returns *ElementHandle — introducing ElementHandle into chromekit is a breaking change for every existing caller and buys nothing the fleet needs"},
	{"Page", "QuerySelectorAll", "same *cdp.Node vs *ElementHandle divergence as QuerySelector, just []T"},
	{"Page", "BoundingBox", "chromekit takes *cdp.Node (from QuerySelector's own divergent return type); firefoxkit takes a selector string directly since it has no comparable node handle to reuse"},
	{"Page", "Frames", "firefoxkit takes ctx (its BiDi getTree round-trip needs external cancellation); chromekit's CDP frame-tree fetch is bound by the page's own ctx instead. Pre-existing: chromekit's Frames also already returns the full recursive tree rather than one level — a behavioral divergence that predates this phase and is out of its scope"},
	{"Page", "FrameByName", "same ctx-parameter divergence as Frames"},
	{"Page", "FrameByURL", "same ctx-parameter divergence as Frames"},
	{"Page", "MainFrame", "chromekit returns (*Frame, error): the CDP frame-tree fetch backing it can fail. firefoxkit returns *Frame unconditionally — it just wraps the page's own contextID with no round-trip"},
}

// typeName normalizes a reflect.Type for cross-package comparison. chromekit
// and firefoxkit each define their own same-named-but-distinct types
// (MouseAPI, KeyboardAPI, ConsoleEvent, input.Modifier, ...), so exact
// reflect.Type equality would flag nearly every method that touches one.
// Comparing by bare name (stripping the package qualifier) checks shape
// parity — which is what this phase is actually about — without demanding
// the two modules share types they were never meant to share.
func typeName(t reflect.Type) string {
	switch t.Kind() {
	case reflect.Ptr:
		return "*" + typeName(t.Elem())
	case reflect.Slice:
		return "[]" + typeName(t.Elem())
	case reflect.Array:
		return "[" + strconv.Itoa(t.Len()) + "]" + typeName(t.Elem())
	case reflect.Map:
		return "map[" + typeName(t.Key()) + "]" + typeName(t.Elem())
	case reflect.Chan:
		return "chan " + typeName(t.Elem())
	case reflect.Func:
		ins := make([]string, t.NumIn())
		for i := range ins {
			ins[i] = typeName(t.In(i))
		}
		outs := make([]string, t.NumOut())
		for i := range outs {
			outs[i] = typeName(t.Out(i))
		}
		sig := "func(" + strings.Join(ins, ", ") + ")"
		switch len(outs) {
		case 0:
		case 1:
			sig += " " + outs[0]
		default:
			sig += " (" + strings.Join(outs, ", ") + ")"
		}
		return sig
	default:
		if t.Name() == "" {
			return t.String() // unnamed: interface{}, etc.
		}
		return t.Name() // strips the package qualifier on purpose
	}
}

// methodNames returns every exported method name on t (a pointer-to-struct
// type from reflect.TypeOf((*T)(nil))).
func methodNames(t reflect.Type) map[string]bool {
	out := make(map[string]bool, t.NumMethod())
	for i := 0; i < t.NumMethod(); i++ {
		out[t.Method(i).Name] = true
	}
	return out
}

// methodSignature returns a normalized "(params) results" string for the
// named method on t, skipping the receiver (index 0). ok=false when t has
// no such method.
func methodSignature(t reflect.Type, name string) (string, bool) {
	m, ok := t.MethodByName(name)
	if !ok {
		return "", false
	}
	ins := make([]string, 0, m.Type.NumIn()-1)
	for i := 1; i < m.Type.NumIn(); i++ {
		ins = append(ins, typeName(m.Type.In(i)))
	}
	outs := make([]string, 0, m.Type.NumOut())
	for i := 0; i < m.Type.NumOut(); i++ {
		outs = append(outs, typeName(m.Type.Out(i)))
	}
	sig := "(" + strings.Join(ins, ", ") + ")"
	switch len(outs) {
	case 0:
	case 1:
		sig += " " + outs[0]
	default:
		sig += " (" + strings.Join(outs, ", ") + ")"
	}
	return sig, true
}

func exemptMethod(list []exemption, typ, method string) (string, bool) {
	for _, e := range list {
		if e.typ == typ && e.method == method {
			return e.reason, true
		}
	}
	return "", false
}

// checkSuperset fails when firefoxT has an exported method chromeT lacks,
// unless that method name is declared in methodSetExemptions for typ.
func checkSuperset(t *testing.T, typ string, chromeT, firefoxT reflect.Type) {
	t.Helper()
	chromeM := methodNames(chromeT)
	firefoxM := methodNames(firefoxT)

	var missing []string
	for name := range firefoxM {
		if chromeM[name] {
			continue
		}
		if _, ok := exemptMethod(methodSetExemptions, typ, name); ok {
			continue
		}
		missing = append(missing, name)
	}
	sort.Strings(missing)
	if len(missing) > 0 {
		t.Errorf("chromekit.%s is missing %d method(s) present on firefoxkit.%s (not exempted): %s",
			typ, len(missing), typ, strings.Join(missing, ", "))
	}
}

// 1. chromekit.Page ⊇ firefoxkit.Page.
func TestParity_PageMethodSets(t *testing.T) {
	checkSuperset(t, "Page",
		reflect.TypeOf((*chromekit.Page)(nil)),
		reflect.TypeOf((*firefoxkit.Page)(nil)))
}

// 2. chromekit.MouseAPI ⊇ firefoxkit.MouseAPI, chromekit.KeyboardAPI ⊇
// firefoxkit.KeyboardAPI, chromekit.Browser ⊇ firefoxkit.Browser.
func TestParity_MouseMethodSets(t *testing.T) {
	checkSuperset(t, "MouseAPI",
		reflect.TypeOf((*chromekit.MouseAPI)(nil)),
		reflect.TypeOf((*firefoxkit.MouseAPI)(nil)))
}

func TestParity_KeyboardMethodSets(t *testing.T) {
	checkSuperset(t, "KeyboardAPI",
		reflect.TypeOf((*chromekit.KeyboardAPI)(nil)),
		reflect.TypeOf((*firefoxkit.KeyboardAPI)(nil)))
}

func TestParity_BrowserMethodSets(t *testing.T) {
	checkSuperset(t, "Browser",
		reflect.TypeOf((*chromekit.Browser)(nil)),
		reflect.TypeOf((*firefoxkit.Browser)(nil)))
}

// 3. For every method present on both sides, the normalized signature
// (params + results, receiver excluded, package qualifiers stripped) must
// match — unless the method name is declared in signatureExemptions for
// that type.
func TestParity_SignaturesMatch(t *testing.T) {
	cases := []struct {
		typ               string
		chromeT, firefoxT reflect.Type
	}{
		{"Page", reflect.TypeOf((*chromekit.Page)(nil)), reflect.TypeOf((*firefoxkit.Page)(nil))},
		{"MouseAPI", reflect.TypeOf((*chromekit.MouseAPI)(nil)), reflect.TypeOf((*firefoxkit.MouseAPI)(nil))},
		{"KeyboardAPI", reflect.TypeOf((*chromekit.KeyboardAPI)(nil)), reflect.TypeOf((*firefoxkit.KeyboardAPI)(nil))},
		{"Browser", reflect.TypeOf((*chromekit.Browser)(nil)), reflect.TypeOf((*firefoxkit.Browser)(nil))},
	}
	for _, c := range cases {
		var names []string
		for name := range methodNames(c.firefoxT) {
			names = append(names, name)
		}
		sort.Strings(names)
		for _, name := range names {
			chromeSig, chromeOK := methodSignature(c.chromeT, name)
			if !chromeOK {
				continue // absent entirely — covered by TestParity_*MethodSets
			}
			firefoxSig, _ := methodSignature(c.firefoxT, name)
			if chromeSig == firefoxSig {
				continue
			}
			if _, ok := exemptMethod(signatureExemptions, c.typ, name); ok {
				continue
			}
			t.Errorf("%s.%s signature diverges (not exempted):\n  chromekit%s\n  firefoxkit%s",
				c.typ, name, chromeSig, firefoxSig)
		}
	}
}

// mouseCoverageExemptions / keyboardCoverageExemptions are consulted only by
// TestConformance_AdapterCoversMouse / *Keyboard. Both are empty: every
// exported chromekit.MouseAPI / KeyboardAPI method (12 and 8 respectively) has
// an identical name on the sibling kit and a signature that wraps cleanly
// (primitives only, or — for Keyboard's optional trailing variadic — dropped
// per driver_keyboard.go), so nothing needed exempting. Kept declared rather
// than deleted so a future divergence has a place to land with a reason
// instead of silently widening the interface's blind spot.
var mouseCoverageExemptions []exemption
var keyboardCoverageExemptions []exemption

// checkInterfaceCoverage fails when a kit's method (by name) is absent from
// the fleet interface type ifaceT, unless exempted. Reused by the Mouse and
// Keyboard conformance tests — see checkSuperset above for the analogous
// cross-kit check.
func checkInterfaceCoverage(t *testing.T, typ string, ifaceT reflect.Type, kits []reflect.Type, exemptions []exemption) {
	t.Helper()
	fleetM := methodNames(ifaceT)
	var missing []string
	for _, kitT := range kits {
		for name := range methodNames(kitT) {
			if fleetM[name] {
				continue
			}
			if _, ok := exemptMethod(exemptions, typ, name); ok {
				continue
			}
			missing = append(missing, name)
		}
	}
	sort.Strings(missing)
	// De-dup (chrome/firefox share almost every name).
	uniq := missing[:0]
	seen := map[string]bool{}
	for _, m := range missing {
		if !seen[m] {
			seen[m] = true
			uniq = append(uniq, m)
		}
	}
	if len(uniq) > 0 {
		t.Errorf("automationfleet.%s is missing %d method(s) present on chromekit/firefoxkit %s (not exempted): %s",
			typ, len(uniq), typ, strings.Join(uniq, ", "))
	}
}

// 1. Every chromekit.MouseAPI / firefoxkit.MouseAPI method is present on
// automationfleet.Mouse, trừ danh sách miễn trừ tường minh (empty today).
func TestConformance_AdapterCoversMouse(t *testing.T) {
	checkInterfaceCoverage(t, "MouseAPI",
		reflect.TypeOf((*Mouse)(nil)).Elem(),
		[]reflect.Type{
			reflect.TypeOf((*chromekit.MouseAPI)(nil)),
			reflect.TypeOf((*firefoxkit.MouseAPI)(nil)),
		},
		mouseCoverageExemptions)
}

// 2. Same as above for KeyboardAPI.
func TestConformance_AdapterCoversKeyboard(t *testing.T) {
	checkInterfaceCoverage(t, "KeyboardAPI",
		reflect.TypeOf((*Keyboard)(nil)).Elem(),
		[]reflect.Type{
			reflect.TypeOf((*chromekit.KeyboardAPI)(nil)),
			reflect.TypeOf((*firefoxkit.KeyboardAPI)(nil)),
		},
		keyboardCoverageExemptions)
}

// pageAccountingBannedWords mirrors the banned-language check applied to
// methodSetExemptions/signatureExemptions below, reused here for
// unwrappedPageMethods so a deferred-debt reason can't hide in either list.
var pageAccountingBannedWords = []string{"temporary", "todo", "tạm thời", "sẽ làm sau", "fixme", "later", "for now"}

// 3. Every exported method on chromekit.Page ∪ firefoxkit.Page is accounted
// for exactly once: either present on automationfleet.Page (by name), or
// declared in unwrappedPageMethods with a real reason. Wrapping too much
// (a name with no kit counterpart) or too little (present on neither side)
// both fail — this is the whole point of the test per phase-08's spec.
func TestConformance_PageSubsetDeclared(t *testing.T) {
	kitMethods := map[string]bool{}
	for name := range methodNames(reflect.TypeOf((*chromekit.Page)(nil))) {
		kitMethods[name] = true
	}
	for name := range methodNames(reflect.TypeOf((*firefoxkit.Page)(nil))) {
		kitMethods[name] = true
	}

	fleetMethods := methodNames(reflect.TypeOf((*Page)(nil)).Elem())
	// ElementCenter/ReadValue are fleet-only composites (predate this phase —
	// see driver_page.go) that don't correspond 1:1 to a single kit method
	// name, so they're excluded from the "must exist on a kit" check below.
	composites := map[string]bool{"ElementCenter": true, "ReadValue": true}

	unwrapped := map[string]string{}
	for _, u := range unwrappedPageMethods {
		unwrapped[u.method] = u.reason
	}

	var wrappedAndDeclared, unaccounted []string
	for name := range kitMethods {
		_, wrapped := fleetMethods[name]
		_, declared := unwrapped[name]
		switch {
		case wrapped && declared:
			wrappedAndDeclared = append(wrappedAndDeclared, name)
		case !wrapped && !declared:
			unaccounted = append(unaccounted, name)
		}
	}
	sort.Strings(wrappedAndDeclared)
	sort.Strings(unaccounted)
	if len(wrappedAndDeclared) > 0 {
		t.Errorf("wrapped on Page AND declared in unwrappedPageMethods (pick one): %s", strings.Join(wrappedAndDeclared, ", "))
	}
	if len(unaccounted) > 0 {
		t.Errorf("neither wrapped on Page nor declared in unwrappedPageMethods — accounting gap: %s", strings.Join(unaccounted, ", "))
	}

	var stale []string
	for name := range fleetMethods {
		if composites[name] || kitMethods[name] {
			continue
		}
		stale = append(stale, name)
	}
	sort.Strings(stale)
	if len(stale) > 0 {
		t.Errorf("Page wraps method(s) that exist on neither kit's Page — typo or stale wrap: %s", strings.Join(stale, ", "))
	}

	var staleUnwrapped []string
	for name := range unwrapped {
		if !kitMethods[name] {
			staleUnwrapped = append(staleUnwrapped, name)
		}
	}
	sort.Strings(staleUnwrapped)
	if len(staleUnwrapped) > 0 {
		t.Errorf("unwrappedPageMethods has stale entry(ies) — method doesn't exist on either kit's Page: %s", strings.Join(staleUnwrapped, ", "))
	}

	for _, u := range unwrappedPageMethods {
		reason := strings.TrimSpace(u.reason)
		if reason == "" {
			t.Errorf("unwrappedPageMethods %s has no reason", u.method)
			continue
		}
		if len(reason) < 20 {
			t.Errorf("unwrappedPageMethods %s reason %q is too short to actually explain the exclusion", u.method, reason)
		}
		low := strings.ToLower(reason)
		for _, b := range pageAccountingBannedWords {
			if strings.Contains(low, b) {
				t.Errorf("unwrappedPageMethods %s reason %q reads like deferred debt (contains %q)", u.method, u.reason, b)
			}
		}
	}
}

// 4. Compile-time: both adapters implement all four fleet interfaces.
// Written as explicit `var _ Iface = concreteType{}` assignments (rather than
// relying on usage elsewhere in the package) so a missing method fails right
// here with a readable "does not implement" compiler error, instead of
// surfacing as a confusing error somewhere Current()/Mouse()/Keyboard() is
// called.
func TestConformance_BothAdaptersImplementDriver(t *testing.T) {
	var cd chromeDriver
	var fd firefoxDriver
	var cp chromePage
	var fp firefoxPage
	var cm chromeMouse
	var fm firefoxMouse
	var ck chromeKeyboard
	var fk firefoxKeyboard

	var _ Driver = cd
	var _ Driver = fd
	var _ Page = cp
	var _ Page = fp
	var _ Mouse = cm
	var _ Mouse = fm
	var _ Keyboard = ck
	var _ Keyboard = fk
}

// Reject exemptions that read like deferred debt instead of a permanent,
// documented divergence. This is the whole point of the exemption list
// being a hardcoded, reviewed constant instead of an auto-generated diff.
func TestParity_ExemptionsHaveReasons(t *testing.T) {
	all := append(append([]exemption{}, methodSetExemptions...), signatureExemptions...)
	if len(all) == 0 {
		t.Fatal("no exemptions declared — suspicious for a two-kit parity list; if genuinely zero, delete this test's precondition")
	}
	banned := []string{"temporary", "todo", "tạm thời", "sẽ làm sau", "fixme", "later", "for now"}
	for _, e := range all {
		reason := strings.TrimSpace(e.reason)
		if reason == "" {
			t.Errorf("exemption %s.%s has no reason", e.typ, e.method)
			continue
		}
		if len(reason) < 20 {
			t.Errorf("exemption %s.%s reason %q is too short to actually explain the divergence", e.typ, e.method, reason)
		}
		low := strings.ToLower(reason)
		for _, b := range banned {
			if strings.Contains(low, b) {
				t.Errorf("exemption %s.%s reason %q reads like deferred debt (contains %q) — exemptions must document a permanent divergence, not a TODO",
					e.typ, e.method, e.reason, b)
			}
		}
	}
}

// 7. go.mod carries no replace directive for either kit — the final step of
// phase-08 once both kits are tagged and published.
//
// Deliberately skipped rather than deleted: tagging + publishing a kit
// version is a release action the operator has not authorised in this pass,
// and go.mod's replace still points at the sibling checkouts (the consumer
// repo, multiprofile-v2-auto, also carries its own replace at these same
// kits). Written here so the remaining step has a named, discoverable check
// instead of living only in a report — remove the t.Skip once both kits are
// tagged and go.mod is repointed at the real versions.
func TestGoMod_NoReplaceDirective(t *testing.T) {
	t.Skip("replace directives intentionally retained — tagging chromekit/firefoxkit and repointing go.mod is a release action outside this phase's scope; see plans/260729-2027-native-foreground-and-kit-parity/reports for what remains")
}
