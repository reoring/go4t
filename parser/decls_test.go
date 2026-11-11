package parser_test

import (
	"strings"
	"testing"

	"github.com/reoring/go4t/ast"
	"github.com/reoring/go4t/parser"
)

func TestDeclsSurface(t *testing.T) {
	file, err := parser.Parse(`package main
type Row[A,] = {qty: A} deriving Eq
alias Pair[A, B,] = (A, B,)
class C[A,] { value(x A) Int }
func id[A,](x A) A { x }
let limit = -10
func pair() Pair[Int, Int,] { (1, 2,) }
`)
	if err != nil {
		t.Fatal(err)
	}
	record := file.Decls[0].(*ast.TypeDecl)
	if !record.Shorthand || len(record.Ctors) != 1 || record.Ctors[0].Name != "Row" {
		t.Fatalf("record = %#v", record)
	}
	if c, ok := file.Decls[4].(*ast.ConstDecl); !ok || c.Name != "limit" || c.Type != nil {
		t.Fatalf("constant = %#v", file.Decls[4])
	}
	for _, source := range []string{"f(_ + 1)", "f(0 < _)", "f(_ + _)", "xs |> map(_ + 1)", "f(_ + 2 * 3)", "f(1 + 2 < _)"} {
		if _, err := parser.ParseExpr(source, ast.Pos{Line: 1, Col: 1}); err != nil {
			t.Errorf("%s: %v", source, err)
		}
	}
}

func TestDeclsDiagnostics(t *testing.T) {
	for _, tc := range []struct{ source, token, want string }{
		{`func f(x: Int) Int { x }`, ":", `parameters are written "x Int", without ":"`},
		{`func f() Int { func(x: Int) => x }`, ":", `parameters are written "x Int", without ":"`},
		{`let x Int = 1`, "Int", `let annotations are written "let x: Int = …"`},
		{`func f() Int { let x Int = 1; x }`, "Int =", `let annotations are written "let x: Int = …"`},
		{`type R = R{n Int}`, "Int", `record fields are written "n: Int"`},
		{`type T = A(n Int)`, "Int", `positional constructor fields have no names; write A{n: Int} to name them`},
		{`type R = {n: Int} | Other`, "|", `the record shorthand declares the only constructor of R; name the constructors to add alternatives`},
		{`type R = Other | {n: Int}`, "{", `the record shorthand declares the only constructor of R; name the constructors to add alternatives`},
		{`func f() Int { (1,) }`, ")", `a tuple needs at least two elements`},
		{`alias T = (Int,)`, ")", `a tuple needs at least two elements`},
		{`let x[A] = 1`, "[", `expected "=", found "["`},
	} {
		t.Run(tc.source, func(t *testing.T) {
			_, err := parser.Parse("package main\n" + tc.source)
			if err == nil || !strings.HasSuffix(err.Error(), ": "+tc.want) {
				t.Fatalf("error = %v; want %s", err, tc.want)
			}
			positioned, ok := err.(interface{ GetPos() ast.Pos })
			column := strings.Index(tc.source, tc.token) + 1
			if tc.source == `func f() Int { (1,) }` {
				column = strings.LastIndex(tc.source, ")") + 1
			}
			if !ok || positioned.GetPos() != (ast.Pos{Line: 2, Col: column}) {
				t.Fatalf("position = %v, want 2:%d", err, column)
			}
		})
	}
}
