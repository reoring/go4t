package laws

import (
	"math/rand/v2"
	"strings"
	"testing"

	"go4t/types"
)

func propertyMapReference(t *testing.T, e *evaluator, runtime string) string {
	t.Helper()
	for _, f := range e.p.Funcs {
		if f.Decl.Extern != nil && f.Decl.Extern.Name == runtime && e.trustedExtern(f) {
			return propertyReference(e, f.Name)
		}
	}
	t.Fatalf("missing modeled map extern %s", runtime)
	return ""
}

func propertyMapSource(t *testing.T, e *evaluator, value *term, render func(*testing.T, *evaluator, *term) string) string {
	entries := make([]string, len(value.args)/2)
	for i := range entries {
		entries[i] = "(" + render(t, e, value.args[2*i]) + ", " + render(t, e, value.args[2*i+1]) + ")"
	}
	if len(entries) == 0 {
		return propertyMapReference(t, e, "EmptyMap") + "()"
	}
	return propertyMapReference(t, e, "MapFromList") + "([" + strings.Join(entries, ", ") + "], func(a, b) => compare(a, b))"
}

func propertyNeutralMap(value *term) bool {
	if value.kind == "mapop" || value.kind == "mapfrom" || value.kind == "insert" || value.kind == "primitive" && modeledMap(strings.TrimPrefix(value.name, "rt.")) {
		return true
	}
	for _, arg := range value.args {
		if propertyNeutralMap(arg) {
			return true
		}
	}
	for _, arm := range value.arms {
		if propertyNeutralMap(arm.guard) || propertyNeutralMap(arm.body) {
			return true
		}
	}
	return false
}

func propertyDictionary(t *testing.T, e *evaluator, class string, typ types.Type) *dictionary {
	t.Helper()
	con, args := typeHead(typ)
	for _, inst := range e.p.Instances {
		if inst.Class.Name != class || inst.TypeCon != con {
			continue
		}
		sub := make(map[*types.TVar]types.Type)
		for i, param := range inst.Params {
			sub[param] = args[i]
		}
		dicts := make([]*dictionary, len(inst.Context))
		for i, context := range inst.Context {
			dicts[i] = propertyDictionary(t, e, context.Class, substituteType(context.Arg, sub))
		}
		return &dictionary{instance: inst, args: dicts}
	}
	t.Fatalf("no %s dictionary for generated %s", class, typ)
	return nil
}

func propertyMapValue(t *testing.T, e *evaluator, rng *rand.Rand, args []types.Type, depth, sample, offset int) *term {
	t.Helper()
	list := &term{kind: "ctor", ctor: e.ctors["Nil"]}
	if depth >= 0 && propertyInhabited(e, args[0], depth-1) && propertyInhabited(e, args[1], depth-1) {
		for range rng.IntN(5) {
			// Scalar leaves share the small domain used by even-numbered
			// substitutions, ensuring collisions with separately bound keys.
			key := propertyValue(t, e, rng, args[0], depth-1, 6, offset)
			value := propertyValue(t, e, rng, args[1], depth-1, sample, offset+1)
			pair := &term{kind: "tuple", args: []*term{key, value}}
			list = &term{kind: "ctor", ctor: e.ctors["Cons"], args: []*term{pair, list}}
		}
	}
	ord := propertyDictionary(t, e, "Ord", args[0])
	if e.eligibleMapDictionary(ord, make(map[*types.Instance]bool)) {
		value, err := e.mapFromList(&term{kind: "map"}, list, ord)
		if err != nil {
			t.Fatal(err)
		}
		return value
	}
	cmp, err := e.normal(&term{kind: "method", name: "compare", arity: 2, dicts: []*dictionary{ord}})
	if err != nil {
		t.Fatal(err)
	}
	return &term{kind: "primitive", name: "rt.MapFromList", args: []*term{list, cmp}}
}

func TestMapKeyCollisionSampling(t *testing.T) {
	e := newEvaluator(loadFixture(t, "map.gt"))
	for _, typ := range []types.Type{types.Int, types.String, types.Bool, types.App(types.Tuple(2), types.Int, types.Bool)} {
		rng := rand.New(rand.NewPCG(2119, 4))
		equal, different := false, false
		for sample := range propertySamples {
			a := propertyValue(t, e, rng, typ, propertyDepth, sample, 0)
			b := propertyValue(t, e, rng, typ, propertyDepth, sample, 1)
			if alphaEqual(a, b, nil) {
				equal = true
			} else {
				different = true
			}
		}
		if !equal || !different {
			t.Errorf("%s: equality=%t inequality=%t", typ, equal, different)
		}
	}
}
