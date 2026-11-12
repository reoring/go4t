package parser_test

import (
	"testing"

	"github.com/reoring/go4t/ast"
)

func TestTacticSurfaceSyntax(t *testing.T) {
	for _, body := range []string{
		"absurd", "match x", "match f(x)", "match true { true => refl; false => absurd }",
		"match f(x) { A | B => refl | C => absurd }", "induction x",
	} {
		t.Run(body, func(t *testing.T) {
			proof := parse(t, "package main\nproof p(x) { "+body+" }").Decls[0].(*ast.ProofDecl)
			if proof.Body.GetEnd().Col <= proof.Body.GetPos().Col {
				t.Fatal("missing tactic span")
			}
		})
	}
	proof := parse(t, "package main\nproof p(x) { match f(x) { A | B => refl | C => absurd } }").Decls[0].(*ast.ProofDecl)
	match := proof.Body.(*ast.TMatch)
	if match.Var != nil || match.Scrut == nil || len(match.Arms) != 2 {
		t.Fatalf("expression match: %#v", match)
	}
	if alternatives, ok := match.Arms[0].Pat.(*ast.POr); !ok || len(alternatives.Alts) != 2 {
		t.Fatalf("alternatives: %#v", match.Arms[0].Pat)
	}
	if _, ok := match.Arms[1].Body.(*ast.TAbsurd); !ok {
		t.Fatalf("absurd: %T", match.Arms[1].Body)
	}
	for _, source := range []string{"x", "(x)", "f(x)", "True", "true"} {
		proof := parse(t, "package main\nproof p(x) { match "+source+" }").Decls[0].(*ast.ProofDecl)
		match := proof.Body.(*ast.TMatch)
		if !match.Bare || (match.Var != nil) != (source == "x") {
			t.Fatalf("scrutinee %s: %#v", source, match)
		}
	}
}
