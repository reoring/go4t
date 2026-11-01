package types

import (
	"strings"
	"testing"

	"gofx/ast"
)

func TestAliasesDoNotChangeUnification(t *testing.T) {
	c := &checker{}
	a := &TVar{Name: "A", Kind: KStar{}}
	alias := &TAlias{Name: "Identity", Args: []Type{a}, Body: a}
	if !unifyTypes(ast.Pos{}, alias, a) || unifyTypes(ast.Pos{}, alias, &TVar{Name: "A", Kind: KStar{}}) {
		t.Fatal("alias changed rigid type-parameter identity")
	}
	meta := c.fresh(KStar{})
	name := &TAlias{Name: "Count", Body: Int}
	nested := &TFunc{Params: []Type{App(Tuple(2), name, Bool)}, Ret: name}
	unify(ast.Pos{}, meta, nested)
	if got := TypeString(meta); got != "func(Tuple2[Int, Bool]) Int" {
		t.Fatalf("inference retained source provenance: %s", got)
	}
	if got := TypeString(nested); got != "func(Tuple2[Count, Bool]) Count" {
		t.Fatalf("binding rewrote the source type: %s", got)
	}
	if got := substitute(alias, map[*TVar]Type{a: Bool}); TypeString(got) != "Identity[Bool]" || Unalias(got) != Bool {
		t.Fatalf("substitution lost alias/semantic body: %#v", got)
	}
}

func TestAliasOccursCheckUsesBody(t *testing.T) {
	c := &checker{}
	ignored := c.fresh(KStar{})
	// Phantom printed arguments cannot create a semantic occurs-check failure.
	unify(ast.Pos{}, ignored, &TAlias{Name: "Ignore", Args: []Type{ignored}, Body: Int})
	if prune(ignored) != Int {
		t.Fatal("phantom argument changed unification")
	}
	meta := c.fresh(KStar{})
	defer func() {
		err, ok := recover().(*Error)
		if !ok || !strings.HasPrefix(err.Msg, "occurs check:") {
			t.Fatalf("recursive alias body was accepted: %v", err)
		}
	}()
	unify(ast.Pos{}, meta, &TAlias{Name: "Recursive", Body: App(Array, meta)})
}
