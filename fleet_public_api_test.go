package automationfleet

// Phase 3 — compatibility gates (Test Spec 23-24).
//
// Both are REGRESSION GUARDS: they are expected to be green and to go red the
// day someone renames a field or drops an exported symbol the consumer builds
// against.

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"reflect"
	"strings"
	"testing"
)

// 23. Snapshot's pre-existing fields keep their NAME and TYPE. The consumer
// maps this struct 1:1 into a Wails DTO, so a rename is a breaking change
// wearing a refactor's clothes. New fields are additive and must exist too.
func TestSnapshot_LegacyFieldsUnchanged(t *testing.T) {
	legacy := map[string]string{
		"HasNative":  "bool",
		"AutoPaused": "bool",
	}
	added := map[string]string{
		"NativeUsed":    "int",
		"NativeMax":     "int",
		"RemoteWorkers": "int",
	}

	st := reflect.TypeOf(Snapshot{})
	check := func(name, want, kind string) {
		fld, ok := st.FieldByName(name)
		if !ok {
			t.Errorf("Snapshot.%s (%s) is missing", name, kind)
			return
		}
		if got := fld.Type.String(); got != want {
			t.Errorf("Snapshot.%s type = %s, want %s", name, got, want)
		}
	}
	for name, want := range legacy {
		check(name, want, "legacy field")
	}
	for name, want := range added {
		check(name, want, "new field")
	}

	// Sanity: the struct must not have lost fields wholesale.
	if st.NumField() < len(legacy)+len(added) {
		t.Errorf("Snapshot has %d fields, want at least %d", st.NumField(), len(legacy)+len(added))
	}
}

// consumerSymbols is every automationfleet identifier the downstream consumer
// (multiprofile-v2-auto) actually references. Regenerate with:
//
//	grep -rhoE "automationfleet\.[A-Za-z_]+" <consumer-repo> --include=*.go \
//	  | sort -u | sed 's/^automationfleet\.//'
//
// Hardcoded on purpose: the consumer repo is not present in CI, and sourcing
// the list from pkg/fleet/types.go instead would make the gate fake — that file
// is missing more than 30 of these.
var consumerSymbols = []string{
	"Action", "ClickAction", "ErrFleetStopped", "ErrUnknownBrowser", "Fleet",
	"Hotkey", "Job", "JobID", "JobResult", "JobStatus",
	"Key", "KeyA", "KeyF", "KeyQ", "KeyS",
	"Logger", "ModAlt", "ModCtrl", "ModShift", "ModWin",
	"Modifier", "MouseButton", "MouseLeft", "MouseMiddle", "MouseRight",
	"NavigateAction", "New", "OnPause", "OnResume", "Option",
	"ParseHotkey", "Register", "RegisterChrome", "RegisterFirefox", "ScrollAction",
	"StatusCancelled", "StatusDone", "StatusFailed", "StatusRejected", "Stop",
	"TypeAction", "WaitAction", "WithCDPWorkers", "WithDefaultTimeout", "WithLogger",
	"WithPauseHotkey", "WithPauseHotkeyDisabled", "WithResumeHotkey", "WithResumeHotkeyDisabled", "WithStopHotkey",
}

// 24. Every symbol the consumer imports must still exist and be exported.
// Method names on Fleet count — the consumer reaches most of the API through a
// *Fleet value, so Register/Stop/… are part of the public surface too.
func TestPublicAPI_CoversConsumerImports(t *testing.T) {
	exported := exportedSurface(t)

	var missing []string
	for _, sym := range consumerSymbols {
		if !ast.IsExported(sym) {
			t.Errorf("consumer symbol %q is not an exported identifier", sym)
			continue
		}
		if _, ok := exported[sym]; !ok {
			missing = append(missing, sym)
		}
	}
	if len(missing) > 0 {
		t.Fatalf("%d consumer symbol(s) no longer exist in automationfleet: %s\n"+
			"the downstream build breaks the moment this lands",
			len(missing), strings.Join(missing, ", "))
	}
}

// exportedSurface parses this package's own sources and returns every exported
// package-level name plus every exported method on Fleet. Reflection cannot
// enumerate a package, and go/ast is stdlib — no build tooling, no network, and
// it works with the consumer repo absent.
func exportedSurface(t *testing.T) map[string]struct{} {
	t.Helper()
	fset := token.NewFileSet()
	pkgs, err := parser.ParseDir(fset, ".", func(fi fs.FileInfo) bool {
		return !strings.HasSuffix(fi.Name(), "_test.go")
	}, 0)
	if err != nil {
		t.Fatalf("parse package: %v", err)
	}

	out := make(map[string]struct{})
	add := func(name string) {
		if ast.IsExported(name) {
			out[name] = struct{}{}
		}
	}
	for _, pkg := range pkgs {
		for _, file := range pkg.Files {
			for _, decl := range file.Decls {
				switch d := decl.(type) {
				case *ast.FuncDecl:
					if d.Recv == nil {
						add(d.Name.Name)
						continue
					}
					if receiverTypeName(d.Recv) == "Fleet" {
						add(d.Name.Name)
					}
				case *ast.GenDecl:
					for _, spec := range d.Specs {
						switch s := spec.(type) {
						case *ast.TypeSpec:
							add(s.Name.Name)
						case *ast.ValueSpec:
							for _, n := range s.Names {
								add(n.Name)
							}
						}
					}
				}
			}
		}
	}
	return out
}

// receiverTypeName returns the bare type name of a method receiver, stripping
// the pointer.
func receiverTypeName(recv *ast.FieldList) string {
	if recv == nil || len(recv.List) == 0 {
		return ""
	}
	expr := recv.List[0].Type
	if star, ok := expr.(*ast.StarExpr); ok {
		expr = star.X
	}
	if ident, ok := expr.(*ast.Ident); ok {
		return ident.Name
	}
	return ""
}
