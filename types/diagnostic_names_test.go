package types_test

import (
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"gofx/ast"
	"gofx/modules"
	"gofx/types"
)

var internalDiagnosticName = regexp.MustCompile(`placeholder\d|\b[A-Z][a-z]+_[A-Z]\w*\b`)

func assertDiagnosticNames(t *testing.T, p *types.Program, err error) {
	t.Helper()
	check := func(e *types.Error) {
		for _, full := range []bool{false, true} {
			if message := e.Message(full); internalDiagnosticName.MatchString(message) {
				t.Errorf("internal name in diagnostic: %s", message)
			}
		}
	}
	var diagnostic *types.Error
	if errors.As(err, &diagnostic) {
		check(diagnostic)
	}
	if p != nil {
		for _, warning := range p.Warnings {
			check(warning)
		}
	}
}

func TestNegativeFixtureDiagnosticNames(t *testing.T) {
	files, err := filepath.Glob("testdata/diagnostics/*.gfx")
	if err != nil || len(files) == 0 {
		t.Fatalf("negative fixtures: %v (%d files)", err, len(files))
	}
	for _, file := range files {
		t.Run(filepath.Base(file), func(t *testing.T) {
			source, err := os.ReadFile(file)
			if err != nil {
				t.Fatal(err)
			}
			first, _, _ := strings.Cut(string(source), "\n")
			want, ok := strings.CutPrefix(first, "// want: ")
			if !ok || want == "" {
				t.Fatal("fixture must declare its expected diagnostic")
			}
			loaded, err := modules.Load([]string{file})
			if err != nil {
				t.Fatal(err)
			}
			p, err := types.Check(loaded)
			assertDiagnosticNames(t, p, err)
			var diagnostic *types.Error
			if p != nil || !errors.As(err, &diagnostic) {
				t.Fatalf("Check = %v, %v; want type error", p, err)
			}
			for _, full := range []bool{false, true} {
				if !strings.Contains(diagnostic.Message(full), want) {
					t.Errorf("diagnostic = %q, want %q", diagnostic.Message(full), want)
				}
			}
		})
	}
}

func TestTypeNamesUseDiagnosingFile(t *testing.T) {
	root := moduleFixture(t, map[string]string{
		"main.gfx": "package main\nimport first \"model\"\nfunc one(x first.Box) first.Label { first.label() }\nfunc main() IO[Unit] { ioPure(unit()) }",
		"other.gfx": "package main\nimport second \"model\"\nfunc two(x second.Box) second.Label { second.label() }",
		"model/model.gfx": "package model\ntype Box = Box{value: Int}\nalias Label = String\nfunc label() Label { \"ok\" }\nfunc own(x Box) Label { label() }",
	})
	p, err := checkModuleFixture(t, root)
	if err != nil {
		t.Fatal(err)
	}
	typ := p.Funcs["one"].Scheme.Type
	for file, want := range map[string]string{
		"main.gfx": "func(first.Box) first.Label",
		"other.gfx": "func(second.Box) second.Label",
		"model/model.gfx": "func(Box) Label",
	} {
		for _, print := range []func(types.Type, string) string{p.TypeString, p.TypeStringFull} {
			if got := print(typ, filepath.Join(root, file)); got != want {
				t.Errorf("%s: got %q, want %q", file, got, want)
			}
		}
	}
	if got := types.TypeString(typ); got != "func(Box) Label" {
		t.Errorf("context-free names: %s", got)
	}
}

func TestImportedDeclarationDiagnosticNames(t *testing.T) {
	for _, tt := range []struct{ source, want string }{
		{"type Box = Box{value: Int}; func bad(x Box) Int { x }", "type mismatch: expected Int, got Box"},
		{"type Choice = Left | Right; func bad(x Choice) Int { match x { Left => 1 } }", "non-exhaustive match; missing: Right"},
		{"type Box = Box deriving Missing", "cannot derive Missing for Box: unknown class"},
		{"type Box = Box deriving Debug, Debug", "duplicate deriving class Debug for Box"},
		{"type Box = Box(func(Int) Int) deriving Eq", "cannot derive Eq for Box: field of function type"},
		{"type Box = Box deriving Functor", "cannot derive Functor for Box: requires a last type parameter"},
		{"alias Loop = Loop", "type alias cycle involving Loop"},
	} {
		t.Run(tt.want, func(t *testing.T) {
			root := moduleFixture(t, map[string]string{
				"main.gfx": "package main\nimport m \"model\"\nfunc main() IO[Unit] { ioPure(unit()) }",
				"model/model.gfx": "package model\n" + tt.source,
			})
			p, err := checkModuleFixture(t, root)
			assertDiagnosticNames(t, p, err)
			var diagnostic *types.Error
			if !errors.As(err, &diagnostic) || diagnostic.Msg != tt.want {
				t.Fatalf("diagnostic = %v, want %q", err, tt.want)
			}
		})
	}
}

func TestSyntheticParametersNeverWarnUnused(t *testing.T) {
	files := parseFiles(t, "func value() Int { (_ + 1)(2) }"+pass5Main)
	lambda := files[1].Decls[0].(*ast.FuncDecl).Body.Result.(*ast.Call).Fn.(*ast.Lambda)
	// Model a lowering that removes the only reference to its synthetic input.
	lambda.Body = &ast.IntLit{Value: 1}
	p, err := types.Check([]*ast.Module{{Files: files}})
	if err != nil {
		t.Fatal(err)
	}
	if len(p.Warnings) != 0 {
		t.Fatalf("synthetic parameter warned: %v", p.Warnings)
	}
}

func TestDefaultWarningUsesFileAlias(t *testing.T) {
	root := moduleFixture(t, map[string]string{
		"main.gfx": "package main\nimport m \"model\"\nfunc main() IO[Unit] { m.Empty; ioPure(unit()) }",
		"model/model.gfx": "package model\ntype Phantom[A] = Empty",
	})
	p, err := checkModuleFixture(t, root)
	assertDiagnosticNames(t, p, err)
	if err != nil {
		t.Fatal(err)
	}
	if len(p.Warnings) != 1 || p.Warnings[0].Msg != "type of m.Empty defaulted to Unit; add an annotation or use the value" {
		t.Fatalf("warnings = %v", p.Warnings)
	}
}

func TestOccursCheckUsesFileAlias(t *testing.T) {
	loaded, err := modules.Load([]string{"testdata/diagnostics/occurs.gfx"})
	if err != nil {
		t.Fatal(err)
	}
	p, err := types.Check(loaded)
	assertDiagnosticNames(t, p, err)
	if err == nil || !strings.Contains(err.Error(), "m.Box") {
		t.Fatalf("occurs check lost its source type: %v", err)
	}
}
