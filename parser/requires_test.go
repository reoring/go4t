package parser_test

import (
	"github.com/reoring/go4t/ast"
	"testing"
)

func TestLawRequires(t *testing.T) {
	f := parse(t, "package main\nlaw bounded(requires Int)\n requires requires < 10 && requires >= 0 { requires + 1 == 1 + requires }\nproof bounded(x) { refl }\n")
	law := f.Decls[0].(*ast.LawDecl)
	if got := exprShape(law.Requires); got != "((requires < 10) && (requires >= 0))" {
		t.Fatalf("requires = %s", got)
	}
	found := false
	ast.Walk(law, func(n ast.Node) {
		if n == law.Requires {
			found = true
		}
	})
	if !found || law.Requires.GetEnd() == (ast.Pos{}) || ast.EndAt(law, law.Requires.GetPos()) == (ast.Pos{}) {
		t.Fatal("precondition missing from walk/spans")
	}
}
