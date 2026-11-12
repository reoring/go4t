package parser_test

import (
	"strings"
	"testing"

	"github.com/reoring/go4t/ast"
	"github.com/reoring/go4t/parser"
)

const ambiguousLawBody = "ambiguous law body: == binds tighter than && and ||; write (lhs) == (rhs) for an equation, or parenthesize the comparison"

func TestLawSurface(t *testing.T) {
	f := parse(t, `package main
law implicit(a Bool) { a }
law inline(a Int) requires a > 0 requires a < 10 requires a != 4 { a > 0 }
proof
{ refl }
proof implicit(renamed) { refl }
`)
	if len(f.Decls) != 4 {
		t.Fatalf("declarations = %d", len(f.Decls))
	}
	law := f.Decls[1].(*ast.LawDecl)
	if !law.Predicate || !law.RHS.(*ast.BoolLit).Value || len(law.RequiresClauses) != 3 {
		t.Fatalf("law = %+v", law)
	}
	chain := law.Requires.(*ast.Binary)
	if chain.Op != "&&" || chain.L.(*ast.Binary).Op != "&&" || chain.R != law.RequiresClauses[2] || chain.GetEnd() != chain.R.GetEnd() {
		t.Fatalf("requires = %+v", chain)
	}
	proof := f.Decls[2].(*ast.ProofDecl)
	if !proof.Inline || proof.Implicit || proof.Name != law.Name || len(proof.Params) != 1 || proof.Params[0].Name != "a" || proof.Params[0].Type != nil {
		t.Fatalf("proof = %+v", proof)
	}
	if f.Decls[3].(*ast.ProofDecl).Inline {
		t.Fatal("separate proof became inline")
	}
}

func TestLawPredicateAmbiguity(t *testing.T) {
	for _, body := range []string{"a && b == b && a", "a == b || c", "a == b |> f", "(a && b == b)", "a || b && c == d"} {
		_, err := parser.Parse("package main; law l() { " + body + " }")
		if err == nil || !strings.HasSuffix(err.Error(), ambiguousLawBody) {
			t.Errorf("%s: %v", body, err)
		}
	}
	for _, body := range []string{"a", "1", "a != b", "a && (b == b) && a", "(a == b) |> f", "(a && b) == (b && a)", "a || (b && c == d)", "f(a == b)", "if a == b { a } else { b }"} {
		if _, err := parser.Parse("package main; law l() { " + body + " }"); err != nil {
			t.Errorf("%s: %v", body, err)
		}
	}
}

func TestLawInlineTactics(t *testing.T) {
	for _, body := range []string{"refl", "match x { None => refl; Some(y) => refl }", "if x { refl } else { refl }", "rewrite other(x) { refl }", "induction x", "induction x { Nil => refl; Cons(y, ys) => refl }", "sample(10)"} {
		f := parse(t, "package main; law l(x Int) { x == x } proof { "+body+" }")
		if len(f.Decls) != 2 || !f.Decls[1].(*ast.ProofDecl).Inline {
			t.Fatalf("%s: %+v", body, f.Decls)
		}
	}
}
