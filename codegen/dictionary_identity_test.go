package codegen

import (
	"testing"

	"go4t/ast"
	"go4t/types"
)

func TestDictionaryRigidIdentity(t *testing.T) {
	source := qualityProgram(t, "dictionary-identity.gt", `package main
type Pair[A] = Pair(A, A)
class Render[X] { render[A Show + Ord](pair X, other A) String }
instance Show[Pair[A]] given Show[A] {
  show(pair) { match pair { Pair(x, y) => show(x) + ":" + show(y) } }
}
instance Render[Pair[A]] given Show[A], Ord[A] {
  render(pair, other) {
    match pair {
      Pair(x, y) => {
        let showOther = show
        let captured = func(value) => show(value) + ":" + show(x) + ":" + showOther(other)
        show(pair) + ":" + show(other) + ":" + show(x < y) + ":" + show(other == other) + ":" + captured(false)
      }
    }
  }
}
func main() IO[Unit] { println(render(Pair(1, 2), true)) }
`)
	vetQualityGo(t, source)
	output, err := runQualityGo(t, source, false)
	if err != nil || output != "1:2:true:true:true:false:1:true\n" {
		t.Fatalf("same-named instance and method dictionaries: %v\n%s", err, output)
	}
}

func TestDictionaryRigidIdentityScopes(t *testing.T) {
	g := &generator{}
	outer := &types.TVar{Name: "A"}
	middle := &types.TVar{Name: "A"}
	inner := &types.TVar{Name: "A"}
	constraint := func(param *types.TVar) types.Constraint {
		return types.Constraint{Class: "Show", Arg: param}
	}
	root := g.newDictionaryScope([]types.Constraint{constraint(outer)}, nil)
	child := g.newDictionaryScope([]types.Constraint{constraint(middle)}, &root)
	g.dictScope = g.newDictionaryScope([]types.Constraint{constraint(inner)}, &child)
	seen := make(map[string]bool)
	for _, param := range []*types.TVar{outer, middle, inner} {
		name := g.dict(&types.DictRef{Kind: types.DictParam, Param: param, ParamClass: "Show"})
		if name == "" || seen[name] {
			t.Fatalf("dictionary names collide: %q", name)
		}
		seen[name] = true
	}
	g.dictScope = g.newDictionaryScope([]types.Constraint{
		constraint(outer), {Class: "Ord", Arg: outer}, constraint(inner),
	}, nil)
	seen = make(map[string]bool)
	for _, bound := range g.dictScope.params {
		name := g.dict(&types.DictRef{Kind: types.DictParam, Param: bound.Arg.(*types.TVar), ParamClass: bound.Class})
		if name == "" || seen[name] {
			t.Fatalf("same-scope dictionary names collide: %q", name)
		}
		seen[name] = true
	}
}

func TestDictionaryRigidIdentitySpecialization(t *testing.T) {
	_, showInt, _ := showFixture()
	outer, inner := &types.TVar{Name: "A"}, &types.TVar{Name: "A"}
	concrete := instanceDict(showInt)
	g := &generator{special: &specialization{dicts: map[types.Constraint]types.DictRef{
		{Class: "Show", Arg: outer}: concrete,
	}}}
	g.dictScope = g.newDictionaryScope([]types.Constraint{{Class: "Show", Arg: inner}}, nil)
	outerRef := types.DictRef{Kind: types.DictParam, Class: "Show", Param: outer, ParamClass: "Show"}
	innerRef := types.DictRef{Kind: types.DictParam, Class: "Show", Param: inner, ParamClass: "Show"}
	if got := g.resolvedDict(outerRef); got.Kind != types.DictInstance || got.Instance != showInt {
		t.Fatalf("outer dictionary was not specialized: %#v", got)
	}
	if got := g.resolvedDict(innerRef); got.Kind != types.DictParam || got.Param != inner {
		t.Fatalf("independent same-named dictionary was specialized: %#v", got)
	}
	if got := g.dict(&innerRef); got != "dict_A" {
		t.Fatalf("inner dictionary = %q, want its own parameter", got)
	}
}

func TestDictionaryRigidIdentityRejectsUnboundParameter(t *testing.T) {
	p, showInt, _ := showFixture()
	bound, unbound := &types.TVar{Name: "A"}, &types.TVar{Name: "A"}
	ref := types.DictRef{Kind: types.DictParam, Class: "Show", Param: unbound, ParamClass: "Show"}
	f := addFunc(p, "invalid", []string{"x"}, []types.Type{bound},
		call(method("show", showInt.Class.Methods[0].Scheme.Type, ref), local("x", bound)))
	f.Scheme.Constraints = []types.Constraint{{Class: "Show", Arg: bound}}
	if _, err := generate(p, "main"); err == nil || err.Error() != (ast.Pos{}).String()+": invalid dictionary annotation" {
		t.Fatalf("unbound rigid dictionary error = %v", err)
	}
}
