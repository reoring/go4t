package parser_test

import (
	"reflect"
	"strings"
	"testing"

	"go4t/ast"
	"go4t/parser"
)

func TestMultiConstraintParameters(t *testing.T) {
	f := parse(t, `package main
func showSorted[A Show + Ord](xs List[A]) String { show(xs) }
class Render[X] { render[F[_] Monad, A Eq + Show](x X, fa F[A]) F[String] }
extern func combine[F[_, _] First + Second + Third, A](x F[A, A]) A = "foreign".Combine
`)
	want := [][]ast.TypeParam{
		{{Name: "A", Constraints: []string{"Show", "Ord"}}},
		{{Name: "F", Arity: 1, Constraints: []string{"Monad"}}, {Name: "A", Constraints: []string{"Eq", "Show"}}},
		{{Name: "F", Arity: 2, Constraints: []string{"First", "Second", "Third"}}, {Name: "A"}},
	}
	got := [][]ast.TypeParam{
		f.Decls[0].(*ast.FuncDecl).TParams,
		f.Decls[1].(*ast.ClassDecl).Methods[0].TParams,
		f.Decls[2].(*ast.FuncDecl).TParams,
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("type parameters = %#v, want %#v", got, want)
	}
}

func TestMultiConstraintSyntaxErrors(t *testing.T) {
	for _, tc := range []struct{ params, want string }{
		{"A Show Ord", `expected "]"`},
		{"A Show +", "expected identifier"},
		{"A + Show", `expected "]"`},
		{"A Show + + Ord", "expected identifier"},
	} {
		t.Run(tc.params, func(t *testing.T) {
			_, err := parser.Parse("package main\nfunc f[" + tc.params + "](x A) A { x }")
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("Parse error = %v, want %q", err, tc.want)
			}
		})
	}
}
