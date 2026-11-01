package parser_test

import (
	"reflect"
	"strings"
	"testing"

	"gofx/ast"
	"gofx/parser"
)

func TestLetPatterns(t *testing.T) {
	for _, tc := range []struct {
		lhs, pattern, value string
		annotated           bool
	}{
		{"(a, b)", "(a, b)", "p", false},
		{"(a, b): (Int, String)", "(a, b)", "p", true},
		{"_", "_", "e", false},
		{"Point{x: x, ..}", "Point{x: x, ..}", "pt", false},
		{"whole @ (a, b)", "whole @ (a, b)", "p", false},
		{"Box(x)", "Box(x)", "p", false},
	} {
		for _, kind := range []string{"block", "do"} {
			t.Run(kind+"/"+tc.lhs, func(t *testing.T) {
				src := "{ let " + tc.lhs + " = " + tc.value + "; done }"
				var name string
				var pat ast.Pattern
				var typ ast.Type
				var value ast.Expr
				if kind == "do" {
					d := result(t, "do "+src).(*ast.Do)
					if len(d.Stmts) != 1 {
						t.Fatalf("statements = %#v", d.Stmts)
					}
					s := d.Stmts[0].(*ast.DoLet)
					name, pat, typ, value = s.Name, s.Pat, s.Type, s.Value
				} else {
					b := result(t, src).(*ast.BlockExpr).Block
					if len(b.Stmts) != 1 {
						t.Fatalf("statements = %#v", b.Stmts)
					}
					s := b.Stmts[0].(*ast.LetStmt)
					name, pat, typ, value = s.Name, s.Pat, s.Type, s.Value
				}
				if name != "" || patternShape(pat) != tc.pattern || exprShape(value) != tc.value {
					t.Fatalf("binding = %q / %s = %s", name, patternShape(pat), exprShape(value))
				}
				if tc.annotated {
					tupleType(t, typ, "Int", "String")
				} else if typ != nil {
					t.Fatalf("unexpected annotation: %#v", typ)
				}
			})
		}
	}
}

func TestDoBindPatterns(t *testing.T) {
	for _, tc := range []struct{ lhs, pattern string }{
		{"(a, s)", "(a, s)"}, {"_", "_"}, {"Box(x)", "Box(x)"},
		{"Point{x: x, ..}", "Point{x: x, ..}"}, {"whole @ (a, s)", "whole @ (a, s)"},
		{"(a)", "a"}, {"None", "None()"},
	} {
		t.Run(tc.lhs, func(t *testing.T) {
			d := result(t, "do { "+tc.lhs+" <- runState(m, 0); pure(a) }").(*ast.Do)
			if len(d.Stmts) != 1 {
				t.Fatalf("statements = %#v", d.Stmts)
			}
			b := d.Stmts[0].(*ast.DoBind)
			if b.Name != "" || patternShape(b.Pat) != tc.pattern || exprShape(b.X) != "runState(m, 0)" {
				t.Fatalf("bind = %#v, pattern = %s", b, patternShape(b.Pat))
			}
		})
	}
}

func TestBareBindingNames(t *testing.T) {
	b := result(t, "{ let x = p; let y: Int = x; y }").(*ast.BlockExpr).Block
	for i, want := range []string{"x", "y"} {
		s := b.Stmts[i].(*ast.LetStmt)
		if s.Name != want || s.Pat != nil {
			t.Fatalf("let = %#v", s)
		}
	}
	d := result(t, "do { x <- p; let y = x; let z: Int = y; pure(z) }").(*ast.Do)
	bind := d.Stmts[0].(*ast.DoBind)
	if bind.Name != "x" || bind.Pat != nil {
		t.Fatalf("bind = %#v", bind)
	}
	for i, want := range []string{"y", "z"} {
		s := d.Stmts[i+1].(*ast.DoLet)
		if s.Name != want || s.Pat != nil {
			t.Fatalf("do let = %#v", s)
		}
	}
}

func TestDoBindBacktracking(t *testing.T) {
	for _, src := range []string{"(a + 1, s)", "Box(a + 1)", "Point{x: f(a), y: b + 1}", "f()", "x.y", "[1, 2]", "func(x) => x", "if ok { a } else { b }"} {
		t.Run(src, func(t *testing.T) {
			expected := exprShape(result(t, src))
			d := result(t, "do { "+src+"; pure(0) }").(*ast.Do)
			if len(d.Stmts) != 1 {
				t.Fatalf("statements = %#v", d.Stmts)
			}
			s, ok := d.Stmts[0].(*ast.DoExpr)
			if !ok {
				t.Fatalf("statement = %T, want DoExpr", d.Stmts[0])
			}
			if got := exprShape(s.X); got != expected {
				t.Fatalf("expression = %s, want %s", got, expected)
			}
			final := result(t, "do { "+src+" }").(*ast.Do)
			if len(final.Stmts) != 0 || exprShape(final.Result) != expected {
				t.Fatalf("final expression = %#v", final)
			}
		})
	}
}

func TestDeriving(t *testing.T) {
	for _, tc := range []struct {
		src  string
		want []string
	}{
		{"type T = A deriving Eq", []string{"Eq"}},
		{"type T[A] = C(A) deriving Eq, Ord, Show", []string{"Eq", "Ord", "Show"}},
		{"type T = A\n | B\n deriving Eq, Ord, Show", []string{"Eq", "Ord", "Show"}},
		{"type T = C{x: Int}\n // continuation\n deriving Show", []string{"Show"}},
	} {
		t.Run(tc.src, func(t *testing.T) {
			f := parse(t, "package main\n"+tc.src+"\nfunc f() Int { 1 }")
			d := f.Decls[0].(*ast.TypeDecl)
			if !reflect.DeepEqual(d.Deriving, tc.want) || len(f.Decls) != 2 {
				t.Fatalf("declaration = %#v, file = %#v", d, f)
			}
		})
	}
}

func TestDerivingRequiresClasses(t *testing.T) {
	for _, src := range []string{"type T = A deriving", "type T = A deriving\n", "type T = A deriving Eq,"} {
		_, err := parser.Parse("package main\n" + src)
		if err == nil || !strings.Contains(err.Error(), "expected identifier") {
			t.Fatalf("source %q: error = %v", src, err)
		}
	}
}
