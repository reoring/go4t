package types

import (
	"testing"

	"github.com/reoring/go4t/ast"
)

func TestAppNormalization(t *testing.T) {
	con := &TCon{Name: "Result", Kind: arityKind(3)}
	prefix := []Type{String}
	partial := App(con, prefix...)
	if App(con) != con || App(partial) != partial {
		t.Fatal("zero-argument application changed a normalized type")
	}
	for _, typ := range []Type{
		App(App(partial, Bool), Int),
		App(&TApp{Fun: partial, Args: []Type{Bool}}, Int),
		App(&TApp{Fun: &TApp{Fun: partial, Args: []Type{Bool}}, Args: []Type{Int}}),
	} {
		app, ok := typ.(*TApp)
		if !ok || app.Fun != con || len(app.Args) != 3 || app.Args[0] != String || app.Args[1] != Bool || app.Args[2] != Int {
			t.Fatalf("App = %#v, want Result[String, Bool, Int]", typ)
		}
		if !sameKind(kindOf(typ), KStar{}) {
			t.Fatalf("full application kind = %s", KindString(kindOf(typ)))
		}
	}
	if !sameKind(kindOf(partial), arityKind(2)) {
		t.Fatalf("partial application kind = %s", KindString(kindOf(partial)))
	}
	if len(prefix) != 1 || prefix[0] != String || len(partial.(*TApp).Args) != 1 {
		t.Fatal("flattening mutated shared prefix arguments")
	}
}

func TestUnifyPartialApplications(t *testing.T) {
	for _, reverse := range []bool{false, true} {
		for _, arity := range []int{1, 2} {
			c := &checker{}
			con := &TCon{Name: "Result", Kind: arityKind(arity + 2)}
			f := c.fresh(arityKind(arity))
			args := make([]Type, arity)
			for i := range args {
				args[i] = c.fresh(KStar{})
			}
			short := App(f, args...)
			longArgs := []Type{String, Bool, Int}
			if arity == 2 {
				longArgs = append(longArgs, Unit)
			}
			long := App(con, longArgs...)
			if reverse {
				unify(ast.Pos{}, long, short)
			} else {
				unify(ast.Pos{}, short, long)
			}
			head, ok := prune(f).(*TApp)
			if !ok || head.Fun != con || len(head.Args) != 2 || head.Args[0] != String || head.Args[1] != Bool {
				t.Fatalf("inferred head = %s", TypeString(f))
			}
			for i, arg := range args {
				if prune(arg) != longArgs[i+2] {
					t.Fatalf("argument %d = %s", i, TypeString(arg))
				}
			}
			resolved := zonk(ast.Pos{}, short, func(*TMeta) { t.Fatal("resolved application defaulted") }).(*TApp)
			if resolved.Fun != con || len(resolved.Args) != len(longArgs) {
				t.Fatalf("zonked application not flattened: %s", TypeString(resolved))
			}
		}
	}
}

func TestPartialApplicationSubstitutionAndKinds(t *testing.T) {
	con := &TCon{Name: "Result", Kind: arityKind(2)}
	f := &TVar{Name: "F", Kind: arityKind(1)}
	partial := App(con, String)
	got := substitute(App(f, Int), map[*TVar]Type{f: partial}).(*TApp)
	if got.Fun != con || len(got.Args) != 2 || got.Args[0] != String || got.Args[1] != Int {
		t.Fatalf("substitution not flattened: %s", TypeString(got))
	}
	// A rigid arrow-kinded variable remains rigid even for a kind-correct head.
	if unifyTypes(ast.Pos{}, f, partial) {
		t.Fatal("rigid type parameter unified with a concrete partial application")
	}
	c := &checker{}
	meta := c.fresh(arityKind(1))
	unify(ast.Pos{}, meta, partial)
	if prune(meta) != partial {
		t.Fatal("arrow-kinded meta did not accept a partial application")
	}
	defer func() {
		err, ok := recover().(*Error)
		if !ok || err.Msg != "kind mismatch: got (*) -> *, want *" {
			t.Fatalf("kind error = %v", err)
		}
	}()
	unify(ast.Pos{}, c.fresh(KStar{}), partial)
}
