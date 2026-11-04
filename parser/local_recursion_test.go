package parser_test

import (
	"strings"
	"testing"

	"github.com/reoring/go4t/ast"
	"github.com/reoring/go4t/parser"
)

func TestLocalFunctionStatements(t *testing.T) {
	for _, prefix := range []string{"", "do "} {
		t.Run(prefix+"block", func(t *testing.T) {
			expr := result(t, prefix+`{
  func first(x Int, f func(Int) Int) Int { second(f(x)) }
  func second(x Int) Int { x }
  func(x) => first(x, func(y) => y)
}`)
			var funcs []*ast.FuncDecl
			var last ast.Expr
			switch e := expr.(type) {
			case *ast.BlockExpr:
				for _, stmt := range e.Block.Stmts {
					funcs = append(funcs, stmt.(*ast.FuncDecl))
				}
				last = e.Block.Result
			case *ast.Do:
				for _, stmt := range e.Stmts {
					funcs = append(funcs, stmt.(*ast.FuncDecl))
				}
				last = e.Result
			}
			if len(funcs) != 2 || funcs[0].Name != "first" || funcs[1].Name != "second" {
				t.Fatalf("local functions = %#v", funcs)
			}
			first := funcs[0]
			if first.Pos.Line != 3 || first.Pos.Col != 3 || len(first.TParams) != 0 || first.Extern != nil || len(first.Params) != 2 {
				t.Fatalf("first = %#v", first)
			}
			if first.Params[0].Name != "x" || first.Params[0].Type.(*ast.TypeName).Name != "Int" || first.Ret.(*ast.TypeName).Name != "Int" {
				t.Fatalf("signature = %#v", first)
			}
			if _, ok := first.Params[1].Type.(*ast.FuncType); !ok {
				t.Fatalf("function parameter = %#v", first.Params[1])
			}
			if _, ok := last.(*ast.Lambda); !ok {
				t.Fatalf("final expression = %T, want lambda", last)
			}
		})
	}
}

func TestLocalFunctionSyntaxErrors(t *testing.T) {
	for _, tc := range []struct{ source, want string }{
		{`func helper[A](x A) A { x }; 0`, "local functions cannot have type parameters"},
		{`func helper(x) Int { x }; 0`, `expected identifier, found ")"`},
		{`func helper(x Int) { x }; 0`, `expected identifier, found "{"`},
		{`func helper() Int { 1 }`, "block must end with an expression"},
	} {
		for _, prefix := range []string{"", "do "} {
			_, err := parser.ParseExpr(prefix+"{ "+tc.source+" }", ast.Pos{Line: 1, Col: 1})
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("%s%s: error = %v, want %q", prefix, tc.source, err, tc.want)
			}
		}
	}
}
