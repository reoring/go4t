package types_test

import (
	"errors"
	"reflect"
	"testing"

	"gofx/ast"
	"gofx/modules"
	"gofx/parser"
	"gofx/types"
)

func TestFloatArithmeticInference(t *testing.T) {
	for _, op := range []string{"+", "-", "*", "/"} {
		for _, source := range []string{
			`func value() Float { 1.5 ` + op + ` 2e3 }`,
			`func value(a Float, b Float) Float { a ` + op + ` b }`,
			`func value() Float { let f = func(x) => x ` + op + ` x; f(1.5) }`,
			`func value() func(Float) Float { func(x) => x ` + op + ` x }`,
		} {
			t.Run(source, func(t *testing.T) {
				p, err := types.Check([]*ast.Module{{Files: parseFiles(t, source+pass5Main)}})
				if err != nil {
					t.Fatal(err)
				}
				walkExprs(reflect.ValueOf(p.Funcs["value"].Decl.Body), func(e ast.Expr) {
					assertZonked(t, e)
					if binary, ok := e.(*ast.Binary); ok {
						for _, part := range []ast.Expr{binary, binary.L, binary.R} {
							if types.AnnOf(part).Type != types.Float {
								t.Errorf("%T type = %s, want Float", part, types.TypeString(types.AnnOf(part).Type))
							}
						}
					}
				})
			})
		}
	}
	for _, source := range []string{
		`func value() Float { -1.5 }`,
		`func value() Float { let negate = func(x) => -x; negate(1.5) }`,
		`func value() func(Float) Float { func(x) => -x }`,
		`func value() Float { let t = ((1.5, 2.0), 3.0); t.0.1 + t.1 }`,
		`func value() Bool { match 1.5 { 1.5 => true; _ => false } }`,
	} {
		if _, err := types.Check([]*ast.Module{{Files: parseFiles(t, source+pass5Main)}}); err != nil {
			t.Errorf("%s: %v", source, err)
		}
	}
}

func TestFloatArithmeticDiagnostics(t *testing.T) {
	for _, tt := range []struct{ source, want string }{
		{`func value() Int { 1 + 1.5 }`, "type mismatch: expected Int, got Float"},
		{`func value() Float { 1.5 + 1 }`, "type mismatch: expected Float, got Int"},
		{`func value() Float { 1 }`, "type mismatch: expected Float, got Int"},
		{`func value() Float { 1.5 % 1.0 }`, "type mismatch: expected Int, got Float"},
		{`func value() Bool { true + false }`, "addition requires Int, Float or String, got Bool"},
		{`func value() String { "a" - "b" }`, "arithmetic requires Int or Float, got String"},
		{`func value() String { "a" * "b" }`, "arithmetic requires Int or Float, got String"},
		{`func value() String { "a" / "b" }`, "arithmetic requires Int or Float, got String"},
		{`func value() Bool { -true }`, "arithmetic requires Int or Float, got Bool"},
		{`func value[A](x A) A { x * x }`, "arithmetic requires Int or Float, got A"},
		{`func value() Float { let f = func(x) => x * x; f(1); f(1.5) }`, "type mismatch: expected Int, got Float"},
	} {
		t.Run(tt.source, func(t *testing.T) {
			files := parseFiles(t, tt.source+pass5Main)
			files[1].Name = "float.gfx"
			p, err := types.Check([]*ast.Module{{Files: files}})
			var typed *types.Error
			if p != nil || !errors.As(err, &typed) || typed.Msg != tt.want || typed.File != "float.gfx" || typed.Pos.Line != 2 || typed.Pos.Col == 0 {
				t.Fatalf("Check = (%v, %v), want positioned %q", p, err, tt.want)
			}
		})
	}
}

func TestFloatPreludeInstances(t *testing.T) {
	file, err := parser.Parse(`package main
func value() String { "${1.5} ${min(1.5, 2.0)} ${max(1, 2)} ${debug(1.5)}" }
func main() IO[Unit] { println(value()) }`)
	if err != nil {
		t.Fatal(err)
	}
	modules, err := modules.LoadFiles([]*ast.File{file}, ".")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := types.Check(modules); err != nil {
		t.Fatal(err)
	}
}
