package types_test

import (
	"strings"
	"testing"

	"github.com/reoring/go4t/ast"
	"github.com/reoring/go4t/format"
	"github.com/reoring/go4t/types"
)

func TestLawImplicitProof(t *testing.T) {
	for _, named := range []bool{false, true} {
		p, err := checkLawSource(t, "law identity(original Int) { original == original }", named)
		if err != nil {
			t.Fatal(err)
		}
		var law *types.LawInfo
		for _, info := range p.Laws {
			law = info
		}
		proof := law.Proof
		if proof != p.Proofs[law.Decl.Name] || proof.Law != law || !proof.Decl.Implicit || proof.Decl.Inline || proof.Decl.Params[0].Name != "original" || proof.Decl.Params[0].Type != nil {
			t.Fatalf("implicit proof = %+v", proof)
		}
		refl, ok := proof.Decl.Body.(*ast.TRefl)
		if !ok || refl.Pos != law.Decl.LHS.GetPos() || refl.End != law.Decl.RHS.GetEnd() {
			t.Fatalf("implicit tactic = %+v", proof.Decl.Body)
		}
		for _, file := range p.Files {
			for _, decl := range file.Decls {
				if _, ok := decl.(*ast.ProofDecl); ok {
					t.Fatal("implicit proof became a declaration")
				}
			}
			if strings.Contains(string(format.File(file)), "proof identity") {
				t.Fatal("implicit proof was printed")
			}
		}
		for _, symbol := range p.PositionIndex().Symbols("laws.gt") {
			if symbol.Kind == types.ProofSymbol {
				t.Fatal("implicit proof became a symbol")
			}
		}
	}
}

func TestLawPredicateTypes(t *testing.T) {
	for _, tc := range []struct{ body, want string }{
		{"1", "Int"}, {"1 + 2", "Int"}, {`"text"`, "String"}, {"func(x Int) => x", "func(Int) Int"}, {"Some(1)", "Option[Int]"},
	} {
		_, err := checkLawSource(t, "law predicate() { "+tc.body+" }", false)
		want := "law body must be an equation (lhs == rhs) or a Bool expression, got " + tc.want
		if err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("%s: %v, want %s", tc.body, err, want)
		}
	}
}

func TestLawInlineParameterIndex(t *testing.T) {
	const source = "law l(a Bool) { a || !a } proof { if a { refl } else { refl } }"
	p, err := checkLawSource(t, source, false)
	if err != nil {
		t.Fatal(err)
	}
	full := "package main\n" + source + lawMain
	binding := p.PositionIndex().At("laws.gt", indexPos(full, "if a", 3))
	if binding == nil || binding.Definition == nil || binding.Definition.NamePos != indexPos(full, "l(a", 2) {
		t.Fatalf("inline parameter binding = %+v", binding)
	}
}
