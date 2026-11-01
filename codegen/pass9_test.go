package codegen

import (
	"strings"
	"testing"

	"gofx/ast"
	"gofx/types"
)

func TestShortCircuitOperators(t *testing.T) {
	for _, op := range []string{"&&", "||"} {
		t.Run(op, func(t *testing.T) {
			p := fixture()
			addFunc(p, "right", nil, nil, boolean(true))
			p.Main.Decl.Body.Result = &ast.Binary{Op: op, L: boolean(false), R: call(function("right", fnType()))}
			out := assertSource(t, p, "return any(any(false).(bool) "+op+" any(right_()).(bool))")
			if strings.Contains(out, "rt.And") || strings.Contains(out, "rt.Or") || strings.Count(out, "right_()") != 2 {
				t.Fatalf("boolean operator must evaluate its RHS only in Go's short-circuit expression:\n%s", out)
			}
		})
	}
}

func TestComparisonDictionaries(t *testing.T) {
	p, eqInt, ordInt := comparisonFixture()
	for _, tt := range []struct {
		op, name, want string
		dict           *types.Instance
	}{
		{"==", "equal", "(Eq_Int{}).Eq(int64(1), int64(2))", eqInt},
		{"!=", "unequal", "rt.Not((Eq_Int{}).Eq(int64(1), int64(2)))", eqInt},
		{"<", "less", "any(ord_lt((Ord_Int{}).Compare(int64(1), int64(2))))", ordInt},
		{"<=", "lessEqual", "any(ord_le((Ord_Int{}).Compare(int64(1), int64(2))))", ordInt},
		{">", "greater", "any(ord_gt((Ord_Int{}).Compare(int64(1), int64(2))))", ordInt},
		{">=", "greaterEqual", "any(ord_ge((Ord_Int{}).Compare(int64(1), int64(2))))", ordInt},
	} {
		addFunc(p, tt.name, nil, nil, comparison(tt.op, integer(1), integer(2), instanceDict(tt.dict)))
		assertSource(t, p, "return "+tt.want)
	}
	out := assertSource(t, p,
		"func ord_lt(o any) bool { _, ok := o.(*Ordering_LT); return ok }",
		"func ord_le(o any) bool { _, ok := o.(*Ordering_GT); return !ok }",
		"func ord_gt(o any) bool { _, ok := o.(*Ordering_GT); return ok }",
		"func ord_ge(o any) bool { _, ok := o.(*Ordering_LT); return !ok }")
	for _, name := range []string{"ord_lt", "ord_le", "ord_gt", "ord_ge"} {
		if strings.Count(out, "func "+name+"(") != 1 {
			t.Errorf("%s was not emitted exactly once", name)
		}
	}
}

func TestComparisonDictionaryParametersAndInstances(t *testing.T) {
	p, eqInt, ordInt := comparisonFixture()
	a, b := &types.TVar{Name: "A"}, &types.TVar{Name: "B"}
	box := &types.DataType{Name: "Box", TParams: []*types.TVar{a}}
	box.Ctors = []*types.Ctor{{Name: "Box", Data: box, Fields: []types.Type{a}}}
	p.DataTypes[box.Name] = box
	boxCon := &types.TCon{Name: "Box"}
	eqParam := types.DictRef{Kind: types.DictParam, Class: "Eq", Param: "A", ParamClass: "Eq"}
	ordParam := types.DictRef{Kind: types.DictParam, Class: "Ord", Param: "B", ParamClass: "Ord"}
	super := types.DictRef{Kind: types.DictSuper, Class: "Eq", Super: &ordParam}
	unbox := func(body ast.Expr) *ast.Match {
		return &ast.Match{Scrut: tupleLiteral(local("left", types.Unit), local("right", types.Unit)), Arms: []ast.Arm{{
			Pat: &ast.PTuple{Elems: []ast.Pattern{
				&ast.PCtor{Name: "Box", Args: []ast.Pattern{&ast.PVar{Name: "x"}}},
				&ast.PCtor{Name: "Box", Args: []ast.Pattern{&ast.PVar{Name: "y"}}},
			}}, Body: body,
		}}}
	}
	eqBox := &types.Instance{
		Class: eqInt.Class, TypeCon: boxCon, Params: []*types.TVar{a}, Context: []types.Constraint{{Class: "Eq", Arg: a}},
		Decl: &ast.InstanceDecl{Methods: []ast.InstMethod{{Name: "eq", Params: []string{"left", "right"}, Body: &ast.Block{
			Result: unbox(comparison("==", local("x", a), local("y", a), eqParam)),
		}}}},
	}
	ordBox := &types.Instance{
		Class: ordInt.Class, TypeCon: boxCon, Params: []*types.TVar{b}, Context: []types.Constraint{{Class: "Ord", Arg: b}}, SuperArgs: []types.DictRef{super},
		Decl: &ast.InstanceDecl{Methods: []ast.InstMethod{{Name: "compare", Params: []string{"left", "right"}, Body: &ast.Block{
			Result: unbox(call(method("compare", fnType(b, b), ordParam), local("x", b), local("y", b))),
		}}}},
	}
	p.Instances = append(p.Instances, eqBox, ordBox)
	equal := addFunc(p, "equal", []string{"left", "right"}, []types.Type{a, a}, comparison("==", local("left", a), local("right", a), eqParam))
	equal.Scheme.Constraints = eqBox.Context
	ordered := addFunc(p, "ordered", []string{"left", "right"}, []types.Type{b, b}, comparison("<=", local("left", b), local("right", b), ordParam))
	ordered.Scheme.Constraints = ordBox.Context
	equalSuper := addFunc(p, "equalSuper", []string{"left", "right"}, []types.Type{b, b}, comparison("!=", local("left", b), local("right", b), super))
	equalSuper.Scheme.Constraints = ordBox.Context
	boxed := call(ctor("Box"), integer(1))
	addFunc(p, "boxedEqual", nil, nil, comparison("==", boxed, boxed, instanceDict(eqBox, instanceDict(eqInt))))
	addFunc(p, "boxedOrdered", nil, nil, comparison("<", boxed, boxed, instanceDict(ordBox, instanceDict(ordInt))))
	nested := call(ctor("Box"), boxed)
	addFunc(p, "nestedOrdered", nil, nil, comparison(">", nested, nested, instanceDict(ordBox, instanceDict(ordBox, instanceDict(ordInt)))))
	assertSource(t, p,
		"return (dict_A).Eq(left_, right_)",
		"return any(ord_le((dict_B).Compare(left_, right_)))",
		"return rt.Not((dict_B).Eq(left_, right_))",
		"return (dictself.dict_A).Eq(x_, y_)",
		"return (dictself.dict_B).Compare(x_, y_)",
		"(Eq_Box{dict_A: Eq_Int{}}).Eq(",
		"(Ord_Box{Eq_Box: Eq_Box{dict_A: Ord_Int{}}, dict_B: Ord_Int{}}).Compare(",
		"(Ord_Box{Eq_Box: Eq_Box{dict_A: Ord_Box{Eq_Box: Eq_Box{dict_A: Ord_Int{}}, dict_B: Ord_Int{}}}, dict_B: Ord_Box{Eq_Box: Eq_Box{dict_A: Ord_Int{}}, dict_B: Ord_Int{}}}).Compare(")
}

func TestComparisonRequiresDictionary(t *testing.T) {
	p := fixture()
	expr := comparison("==", integer(1), integer(1), types.DictRef{})
	expr.ExprBase.Pos = ast.Pos{Line: 3, Col: 7}
	types.AnnOf(expr).Dicts = nil
	p.Main.Decl.Body.Result = expr
	if _, err := generate(p, "main"); err == nil || err.Error() != "3:7: comparison requires one dictionary annotation" {
		t.Fatalf("missing comparison dictionary error = %v", err)
	}
}

func TestLambdaParameterPatterns(t *testing.T) {
	for _, name := range []string{"name", "wildcard", "tuple", "constructor", "record", "as", "nested", "multiple", "zero"} {
		t.Run(name, func(t *testing.T) {
			p := fixture()
			box := &types.DataType{Name: "Box"}
			box.Ctors = []*types.Ctor{{Name: "Box", Data: box, Fields: []types.Type{types.Int}, FieldNames: []string{"value"}}}
			p.DataTypes[box.Name] = box
			record := &ast.PRecord{Ctor: "Box", Names: []string{"value"}, Pats: []ast.Pattern{&ast.PVar{Name: "x"}}}
			p.RecordPats = map[*ast.PRecord]types.RecordPat{record: {Ctor: box.Ctors[0], Indexes: []int{0}}}
			params := []ast.Param{{Pat: &ast.PVar{Name: "x"}}}
			body := ast.Expr(local("x", types.Int))
			fragments := []string{"func(p0 any) any", "var x_ any ="}
			switch name {
			case "name":
				params[0] = ast.Param{Name: "x"}
			case "wildcard":
				params[0].Pat = &ast.PWild{}
				body = integer(7)
				fragments = []string{"func(p0 any) any", "return int64(7)"}
			case "tuple":
				params[0].Pat = &ast.PTuple{Elems: []ast.Pattern{&ast.PVar{Name: "x"}, &ast.PWild{}}}
				fragments = append(fragments, "p0.(*Tuple2)")
			case "constructor":
				params[0].Pat = &ast.PCtor{Name: "Box", Args: []ast.Pattern{&ast.PVar{Name: "x"}}}
				fragments = append(fragments, "p0.(*Box_Box)")
			case "record":
				params[0].Pat = record
				fragments = append(fragments, "p0.(*Box_Box)")
			case "as":
				params[0].Pat = &ast.PAs{Name: "whole", Pat: &ast.PTuple{Elems: []ast.Pattern{&ast.PVar{Name: "x"}, &ast.PWild{}}}}
				body = tupleLiteral(local("whole", types.Unit), body)
				fragments = append(fragments, "var whole_ any = p0", "p0.(*Tuple2)")
			case "nested":
				params[0].Pat = &ast.PTuple{Elems: []ast.Pattern{record, &ast.PWild{}}}
				fragments = append(fragments, "p0.(*Tuple2)", ".F0.(*Box_Box)")
			case "multiple":
				params = append(params, ast.Param{Pat: &ast.PTuple{Elems: []ast.Pattern{&ast.PVar{Name: "y"}, &ast.PWild{}}}}, ast.Param{Pat: &ast.PWild{}})
				body = tupleLiteral(body, local("y", types.Int))
				fragments = []string{"func(p0 any, p1 any, p2 any) any", "var x_ any = p0", "p1.(*Tuple2)", "var y_ any ="}
			case "zero":
				params = nil
				body = integer(7)
				fragments = []string{"return func() any", "return int64(7)"}
			}
			p.Main.Decl.Body.Result = &ast.Lambda{Params: params, Body: body}
			out := assertSource(t, p, fragments...)
			if strings.Contains(out, "switch ") || strings.Contains(out, "panic(") {
				t.Fatalf("irrefutable lambda pattern still has a match guard:\n%s", out)
			}
		})
	}
}

func TestMatchGuards(t *testing.T) {
	p := fixture()
	option(p)
	match := &ast.Match{Scrut: local("value", types.Unit), Arms: []ast.Arm{
		{Pat: &ast.PCtor{Name: "Some", Args: []ast.Pattern{&ast.PVar{Name: "first"}}}, Guard: local("first", types.Bool), Body: integer(1)},
		{Pat: &ast.PCtor{Name: "Some", Args: []ast.Pattern{&ast.PVar{Name: "second"}}}, Guard: &ast.Unary{Op: "!", X: local("second", types.Bool)}, Body: integer(2)},
		{Pat: &ast.PVar{Name: "whole"}, Guard: boolean(false), Body: local("whole", types.Unit)},
		{Pat: &ast.PWild{}, Body: integer(3)},
	}}
	addFunc(p, "guarded", []string{"value"}, []types.Type{types.Unit}, match)
	out := assertSource(t, p, "if any(first_).(bool)", "if any(rt.Not(second_)).(bool)", "if any(false).(bool)", "return whole_", "return int64(3)")
	if strings.Count(out, "case *Option_Some:") != 2 || strings.Contains(out, `panic("non-exhaustive match")`) {
		t.Fatalf("guarded arms must retain independent continuations before the total arm:\n%s", out)
	}
}

func TestGuardedFinalMatchArm(t *testing.T) {
	// Check excludes guarded arms from exhaustiveness. Hand-built programs still
	// need both the final pattern test and a panic if the source guard is false.
	for _, tt := range []struct {
		name    string
		pattern ast.Pattern
		want    string
	}{
		{"wildcard", &ast.PWild{}, "if any(false).(bool)"},
		{"variable", &ast.PVar{Name: "x"}, "var x_ any ="},
		{"literal", &ast.PLit{X: boolean(true)}, ".(bool) == true"},
		{"constructor", &ast.PCtor{Name: "Some", Args: []ast.Pattern{&ast.PWild{}}}, "case *Option_Some:"},
		{"tuple", &ast.PTuple{Elems: []ast.Pattern{&ast.PWild{}, &ast.PWild{}}}, "case *Tuple2:"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			p := fixture()
			option(p)
			addFunc(p, "guarded", []string{"value"}, []types.Type{types.Unit}, &ast.Match{Scrut: local("value", types.Unit), Arms: []ast.Arm{
				{Pat: tt.pattern, Guard: boolean(false), Body: integer(1)},
			}})
			assertSource(t, p, tt.want, "if any(false).(bool)", `panic("non-exhaustive match")`)
		})
	}
}

func TestIntBoundExterns(t *testing.T) {
	p := fixture()
	extern(p, "minInt", nil, nil, "gofx/rt", "MinInt")
	extern(p, "maxInt", nil, nil, "gofx/rt", "MaxInt")
	p.Main.Decl.Body.Result = tupleLiteral(call(function("minInt", fnType())), call(function("maxInt", fnType())))
	assertSource(t, p, "return ext_1.MinInt()", "return ext_1.MaxInt()", "return &Tuple2{F0: minInt_(), F1: maxInt_()}")
}

func comparison(op string, left, right ast.Expr, dict types.DictRef) *ast.Binary {
	return &ast.Binary{ExprBase: annotated(&types.Ann{Type: types.Bool, Dicts: []types.DictRef{dict}}), Op: op, L: left, R: right}
}

func comparisonFixture() (*types.Program, *types.Instance, *types.Instance) {
	p := fixture()
	ordering := &types.DataType{Name: "Ordering"}
	for i, name := range []string{"LT", "EQ", "GT"} {
		ordering.Ctors = append(ordering.Ctors, &types.Ctor{Name: name, Data: ordering, Index: i})
	}
	p.DataTypes[ordering.Name] = ordering
	a := &types.TVar{Name: "A"}
	eq := &types.Class{Name: "Eq", Param: a}
	ord := &types.Class{Name: "Ord", Param: a, Super: "Eq"}
	eq.Methods = []*types.Method{{Name: "eq", Class: eq, Scheme: &types.Scheme{Type: &types.TFunc{Params: []types.Type{a, a}, Ret: types.Bool}}}}
	ord.Methods = []*types.Method{{Name: "compare", Class: ord, Scheme: &types.Scheme{Type: &types.TFunc{Params: []types.Type{a, a}, Ret: &types.TCon{Name: "Ordering"}}}}}
	p.Classes[eq.Name], p.Classes[ord.Name] = eq, ord
	extern(p, "eqInt", []string{"a", "b"}, []types.Type{types.Int, types.Int}, "gofx/rt", "EqInt")
	extern(p, "ltInt", []string{"a", "b"}, []types.Type{types.Int, types.Int}, "gofx/rt", "LtInt")
	primitive := func(name string) ast.Expr {
		return call(function(name, fnType(types.Int, types.Int)), local("a", types.Int), local("b", types.Int))
	}
	eqInt := &types.Instance{Class: eq, TypeCon: types.Int, Decl: &ast.InstanceDecl{Methods: []ast.InstMethod{{
		Name: "eq", Params: []string{"a", "b"}, Body: &ast.Block{Result: primitive("eqInt")},
	}}}}
	ordInt := &types.Instance{Class: ord, TypeCon: types.Int, Decl: &ast.InstanceDecl{Methods: []ast.InstMethod{{
		Name: "compare", Params: []string{"a", "b"}, Body: &ast.Block{Result: &ast.If{Cond: primitive("eqInt"), Then: ctor("EQ"), Else: &ast.If{Cond: primitive("ltInt"), Then: ctor("LT"), Else: ctor("GT")}}},
	}}}}
	p.Instances = append(p.Instances, eqInt, ordInt)
	return p, eqInt, ordInt
}

func TestSuperclassContextImplication(t *testing.T) {
	p, showInt, _ := showFixture()
	debug := &types.Class{Name: "Debug", Param: &types.TVar{Name: "D"}, Super: "Show"}
	inspect := &types.Class{Name: "Inspect", Param: &types.TVar{Name: "I"}, Super: "Debug"}
	p.Classes[debug.Name], p.Classes[inspect.Name] = debug, inspect
	debugInt := &types.Instance{Class: debug, TypeCon: types.Int, Decl: &ast.InstanceDecl{}}
	inspectInt := &types.Instance{Class: inspect, TypeCon: types.Int, Decl: &ast.InstanceDecl{}}
	a, b := &types.TVar{Name: "A"}, &types.TVar{Name: "B"}
	x, y := &types.TVar{Name: "X"}, &types.TVar{Name: "Y"}
	pair := &types.DataType{Name: "Pair", TParams: []*types.TVar{a, b}}
	pair.Ctors = []*types.Ctor{{Name: "Pair", Data: pair, Fields: []types.Type{a, b}}}
	p.DataTypes[pair.Name] = pair
	pairCon := &types.TCon{Name: "Pair"}
	showParam := types.DictRef{Kind: types.DictParam, Class: "Show", Param: "B", ParamClass: "Show"}
	showPair := &types.Instance{
		Class: showInt.Class, TypeCon: pairCon, Params: []*types.TVar{a, b}, Context: []types.Constraint{{Class: "Show", Arg: b}},
		Decl: &ast.InstanceDecl{Methods: []ast.InstMethod{{Name: "show", Params: []string{"pair"}, Body: &ast.Block{Result: &ast.Match{
			Scrut: local("pair", types.Unit), Arms: []ast.Arm{{
				Pat:  &ast.PCtor{Name: "Pair", Args: []ast.Pattern{&ast.PWild{}, &ast.PVar{Name: "second"}}},
				Body: call(method("show", fnType(b), showParam), local("second", b)),
			}},
		}}}}},
	}
	inspectParam := types.DictRef{Kind: types.DictParam, Class: "Inspect", Param: "Y", ParamClass: "Inspect"}
	debugSuper := types.DictRef{Kind: types.DictSuper, Class: "Debug", Super: &inspectParam}
	showSuper := types.DictRef{Kind: types.DictSuper, Class: "Show", Super: &debugSuper}
	debugPair := &types.Instance{
		Class: debug, TypeCon: pairCon, Params: []*types.TVar{x, y},
		Context:   []types.Constraint{{Class: "Show", Arg: x}, {Class: "Inspect", Arg: y}},
		SuperArgs: []types.DictRef{showSuper}, Decl: &ast.InstanceDecl{},
	}
	p.Instances = append(p.Instances, debugInt, inspectInt, showPair, debugPair)
	dict := instanceDict(debugPair, instanceDict(showInt), instanceDict(inspectInt))
	super := types.DictRef{Kind: types.DictSuper, Class: "Show", Super: &dict}
	addFunc(p, "showPair", nil, nil, call(method("show", fnType(types.Unit), super), call(ctor("Pair"), integer(1), integer(2))))
	assertSource(t, p,
		"(Debug_Pair{Show_Pair: Show_Pair{dict_B: Inspect_Int{}}, dict_X: Show_Int{}, dict_Y: Inspect_Int{}}).Show(")
}

func TestSuperclassEmptyContext(t *testing.T) {
	p, showInt, showOption := showFixture()
	showOption.Context = nil
	showOption.Decl.Methods[0].Body.Result = text("Option")
	debug := &types.Class{Name: "Debug", Param: &types.TVar{Name: "A"}, Super: "Show"}
	p.Classes[debug.Name] = debug
	b := &types.TVar{Name: "B"}
	debugOption := &types.Instance{
		Class: debug, TypeCon: showOption.TypeCon, Params: []*types.TVar{b},
		Context: []types.Constraint{{Class: "Show", Arg: b}}, Decl: &ast.InstanceDecl{},
	}
	p.Instances = append(p.Instances, debugOption)
	dict := instanceDict(debugOption, instanceDict(showInt))
	super := types.DictRef{Kind: types.DictSuper, Class: "Show", Super: &dict}
	addFunc(p, "showOption", nil, nil, call(method("show", fnType(types.Unit), super), ctor("None")))
	assertSource(t, p, "(Debug_Option{Show_Option: Show_Option{}, dict_B: Show_Int{}}).Show(")
}
