package parser_test

import (
	"fmt"
	"strings"
	"testing"

	"github.com/reoring/go4t/ast"
	"github.com/reoring/go4t/parser"
)

func TestPipeDesugaring(t *testing.T) {
	for _, tc := range []struct{ src, want string }{
		{"x |> f", "f(x)"},
		{"x |> f(a, b)", "f(x, a, b)"},
		{"x |> f()", "f(x)"},
		{"x |> f(a) |> g(b)", "g(f(x, a), b)"},
		{"a || b |> f", "f((a || b))"},
		{"a |> f || g", "(f || g)(a)"},
		{"a && b || c |> f", "f(((a && b) || c))"},
		{"x |> (func(y) => y + 1)", "lambda(x)"},
		{"x\n |> f(a)\n |> g", "g(f(x, a))"},
		{"x |> object.method(a)", "object.method(x, a)"},
		{"x |> f(_, a)", "lambda(x)"},
		{"xs |> join(\"\\n\", _)", "lambda(xs)"},
	} {
		t.Run(tc.src, func(t *testing.T) {
			if got := exprShape(result(t, tc.src)); got != tc.want {
				t.Fatalf("shape = %s, want %s", got, tc.want)
			}
		})
	}
}

func TestPlaceholderDesugaring(t *testing.T) {
	for _, tc := range []struct {
		src, body string
		arity     int
	}{
		{"add(1, _)", "add(1, $0)", 1},
		{"f(_, x, _)", "f($0, x, $1)", 2},
		{"f(_, _, _)", "f($0, $1, $2)", 3},
		{"f(_,)", "f($0)", 1},
		{"(_ + 1)", "($0 + 1)", 1},
		{"(2 * _)", "(2 * $0)", 1},
		{"(_ == x)", "($0 == x)", 1},
		{"(_ < _)", "($0 < $1)", 2},
		{"(_ + 2 * 3)", "($0 + (2 * 3))", 1},
		{"(1 + 2 < _)", "((1 + 2) < $0)", 1},
		{"(_ |> f)", "f($0)", 1},
		{"(_ |> _)", "$1($0)", 2},
		{"f(_, g(_))", "f($0, lambda)", 1},
	} {
		t.Run(tc.src, func(t *testing.T) {
			e := result(t, tc.src)
			lambda, ok := e.(*ast.Lambda)
			if !ok || len(lambda.Params) != tc.arity {
				t.Fatalf("expression = %#v, want %d-ary lambda", e, tc.arity)
			}
			shape := exprShape(lambda.Body)
			for i, param := range lambda.Params {
				if param.Type != nil || param.Pat != nil || param.Name == "" {
					t.Fatalf("parameter = %#v, want inferred plain name", param)
				}
				shape = strings.ReplaceAll(shape, param.Name, fmt.Sprintf("$%d", i))
			}
			if shape != tc.body {
				t.Fatalf("body = %s, want %s", shape, tc.body)
			}
		})
	}
}

func TestPlaceholderMisusePositions(t *testing.T) {
	for _, src := range []string{
		"_", "(_)", "_ + 1", "1 + _", "f(_ + 1)", "f(1 + _)",
		"[_]", "(_, 1)", "Point{x: _}", "x{value: _}", "_(1)",
		"func(x) => _", "{ _ }", "do { _ }", "if _ { 1 } else { 2 }",
		"(_ + 1 + 2)", "(1 + _ * 2)", "(-_)", "(f(_) + _ + 1)",
	} {
		t.Run(src, func(t *testing.T) {
			_, err := parser.ParseExpr(src, ast.Pos{Line: 4, Col: 7})
			// In the nested call case the call's placeholder is valid; the
			// operator chain's placeholder is the offending one.
			index := strings.Index(src, "_")
			if src == "(f(_) + _ + 1)" {
				index = strings.LastIndex(src, "_")
			}
			want := fmt.Sprintf("4:%d: _ is only valid as a placeholder in a call or operator section", 7+index)
			if err == nil || err.Error() != want {
				t.Fatalf("error = %v, want %q", err, want)
			}
		})
	}
}

func TestPlaceholderScopesAndPatterns(t *testing.T) {
	e := result(t, "f(_, g(_))").(*ast.Lambda)
	outer := e.Body.(*ast.Call)
	inner := outer.Args[1].(*ast.Lambda)
	if e.Params[0].Name == inner.Params[0].Name {
		t.Fatal("nested placeholders reused a binding")
	}
	call := result(t, "f(g(_), (_ + 1))").(*ast.Call)
	if _, ok := call.Args[0].(*ast.Lambda); !ok {
		t.Fatalf("inner call did not become a lambda: %#v", call.Args[0])
	}
	wild := result(t, "func(_) => match 1 { _ => 2 }").(*ast.Lambda)
	if _, ok := wild.Params[0].Pat.(*ast.PWild); !ok {
		t.Fatalf("wildcard parameter changed: %#v", wild.Params[0])
	}
	if _, ok := wild.Body.(*ast.Match).Arms[0].Pat.(*ast.PWild); !ok {
		t.Fatal("wildcard pattern changed")
	}
}

func TestPlaceholderNamesAvoidCapture(t *testing.T) {
	original, err := parser.ParseExpr("f(_, value)", ast.Pos{Line: 1, Col: 1})
	if err != nil {
		t.Fatal(err)
	}
	name := original.(*ast.Lambda).Params[0].Name
	for _, value := range []string{name, `"${` + name + `}"`} {
		expr, err := parser.ParseExpr("f(_, "+value+")", ast.Pos{Line: 1, Col: 1})
		if err != nil {
			t.Fatal(err)
		}
		lambda := expr.(*ast.Lambda)
		if lambda.Params[0].Name == name {
			t.Fatalf("placeholder captured free name %s in %s", name, value)
		}
		call := lambda.Body.(*ast.Call)
		if call.Args[0].(*ast.Ident).Name != lambda.Params[0].Name {
			t.Fatal("placeholder reference does not use its parameter")
		}
		free := call.Args[1]
		if interp, ok := free.(*ast.Interp); ok {
			free = interp.Exprs[0]
		}
		if free.(*ast.Ident).Name != name {
			t.Fatal("free identifier was renamed")
		}
	}
}
