package codegen

import (
	"strings"
	"testing"

	"gofx/ast"
	"gofx/types"
)

func TestBlockLetTuplePattern(t *testing.T) {
	p := fixture()
	pair := tupleLiteral(local("x", types.Int), integer(2))
	addFunc(p, "makePair", []string{"x"}, []types.Type{types.Int}, pair)
	p.Main.Decl.Body = &ast.Block{
		Stmts: []ast.Stmt{
			&ast.LetStmt{Name: "x", Value: integer(1)},
			&ast.LetStmt{Pat: &ast.PTuple{Elems: []ast.Pattern{&ast.PVar{Name: "x"}, &ast.PVar{Name: "y"}}},
				Value: call(function("makePair", fnType(types.Int)), local("x", types.Int))},
		},
		Result: tupleLiteral(local("x", types.Int), local("y", types.Int)),
	}
	out := assertSource(t, p, "any = makePair_(x_)", ".(*Tuple2)", "var x_ any =", "var y_ any =", "return &Tuple2{F0: x_, F1: y_}")
	if strings.Count(out, "= makePair_(x_)") != 1 {
		t.Fatalf("pattern initializer must be evaluated exactly once before shadowing x:\n%s", out)
	}
	if strings.Contains(out, "switch ") || strings.Contains(out, "panic(") {
		t.Fatalf("irrefutable let must not emit a match guard:\n%s", out)
	}
}

func TestDoLetRecordPattern(t *testing.T) {
	p, _, monad := classFixture()
	person := &types.DataType{Name: "Person"}
	person.Ctors = []*types.Ctor{{Name: "Person", Data: person, Fields: []types.Type{types.String, types.Int}, FieldNames: []string{"name", "age"}}}
	p.DataTypes[person.Name] = person
	pattern := &ast.PRecord{Ctor: "Person", Names: []string{"age", "name"}, Pats: []ast.Pattern{&ast.PVar{Name: "age"}, &ast.PVar{Name: "name"}}}
	p.RecordPats = map[*ast.PRecord]types.RecordPat{pattern: {Ctor: person.Ctors[0], Indexes: []int{1, 0}}}
	dict := instanceDict(monad)
	addFunc(p, "sequence", nil, nil, &ast.Do{
		ExprBase: annotated(&types.Ann{Type: types.Unit, Monad: &dict}),
		Stmts: []ast.DoStmt{
			&ast.DoLet{Pat: pattern, Value: call(ctor("Person"), text("Ada"), integer(42))},
		},
		Result: call(ctor("Some"), tupleLiteral(local("name", types.String), local("age", types.Int))),
	})
	out := assertSource(t, p, `any = &Person_Person{F0: "Ada", F1: int64(42)}`, ".(*Person_Person)", "var name_ any =", ".F0", "var age_ any =", ".F1", "return &Option_Some{F0: &Tuple2{F0: name_, F1: age_}}")
	if strings.Count(out, `= &Person_Person{F0: "Ada", F1: int64(42)}`) != 1 || strings.Contains(out, "case *Person_Person:") {
		t.Fatalf("record let must destructure one initializer without a match guard:\n%s", out)
	}
}

func TestDoBindTuplePattern(t *testing.T) {
	p, _, monad := classFixture()
	dict := instanceDict(monad)
	addFunc(p, "sequence", nil, nil, &ast.Do{
		ExprBase: annotated(&types.Ann{Type: types.Unit, Monad: &dict}),
		Stmts: []ast.DoStmt{
			&ast.DoBind{Pat: &ast.PTuple{Elems: []ast.Pattern{&ast.PVar{Name: "x"}, &ast.PVar{Name: "y"}}},
				X: call(ctor("Some"), tupleLiteral(integer(1), integer(2)))},
			&ast.DoBind{Pat: &ast.PAs{Name: "whole", Pat: &ast.PTuple{Elems: []ast.Pattern{&ast.PVar{Name: "x"}, &ast.PWild{}}}},
				X: call(ctor("Some"), tupleLiteral(local("x", types.Int), local("y", types.Int)))},
		},
		Result: call(ctor("Some"), tupleLiteral(local("whole", types.Unit), local("x", types.Int), local("y", types.Int))),
	})
	out := assertSource(t, p,
		"(Monad_Option{}).Bind(&Option_Some{F0: &Tuple2{F0: int64(1), F1: int64(2)}}, func(p any) any",
		"(Monad_Option{}).Bind(&Option_Some{F0: &Tuple2{F0: x_, F1: y_}}, func(p any) any",
		"p.(*Tuple2)", "var whole_ any = p", "var x_ any =", "var y_ any =",
		"return &Option_Some{F0: &Tuple3{F0: whole_, F1: x_, F2: y_}}")
	if strings.Count(out, "func(p any) any") != 2 || strings.Contains(out, "case *Tuple2:") {
		t.Fatalf("each pattern bind must destructure its own continuation parameter without a guard:\n%s", out)
	}
}

func TestLetWildcardAndNestedPatterns(t *testing.T) {
	p := fixture()
	box := &types.DataType{Name: "Box"}
	box.Ctors = []*types.Ctor{{Name: "Box", Data: box, Fields: []types.Type{types.Int}}}
	p.DataTypes[box.Name] = box
	addFunc(p, "value", nil, nil, integer(7))
	p.Main.Decl.Body = &ast.Block{
		Stmts: []ast.Stmt{
			&ast.LetStmt{Pat: &ast.PWild{}, Value: call(function("value", fnType()))},
			&ast.LetStmt{Pat: &ast.PAs{Name: "whole", Pat: &ast.PTuple{Elems: []ast.Pattern{
				&ast.PCtor{Name: "Box", Args: []ast.Pattern{&ast.PVar{Name: "x"}}}, &ast.PWild{},
			}}}, Value: tupleLiteral(call(ctor("Box"), integer(3)), integer(4))},
		},
		Result: tupleLiteral(local("whole", types.Unit), local("x", types.Int)),
	}
	out := assertSource(t, p, "any = value_()", "var whole_ any =", ".(*Tuple2)", ".F0.(*Box_Box)", "var x_ any =", "return &Tuple2{F0: whole_, F1: x_}")
	if strings.Contains(out, "switch ") || strings.Contains(out, "panic(") {
		t.Fatalf("nested irrefutable patterns must not emit match guards:\n%s", out)
	}
}

func TestSynthesizedShowInstanceInterpolation(t *testing.T) {
	// Deriving appends ordinary InstanceDecls before checking. Build its checked
	// representation directly, just like the other independent codegen fixtures.
	p, showInt, _ := showFixture()
	a := &types.TVar{Name: "A", Kind: types.KStar{}}
	box := &types.DataType{Name: "Box", TParams: []*types.TVar{a}}
	box.Ctors = []*types.Ctor{{Name: "Box", Data: box, Fields: []types.Type{a}, FieldNames: []string{"value"}}}
	p.DataTypes[box.Name] = box
	param := types.DictRef{Kind: types.DictParam, Class: "Show", Param: "A", ParamClass: "Show"}
	pos := ast.Pos{Line: 2, Col: 1}
	decl := &ast.InstanceDecl{
		Pos: pos, Class: "Show", TypeCon: "Box", Params: []string{"A"}, Context: []ast.Constraint{{Class: "Show", Param: "A"}},
		Methods: []ast.InstMethod{{Pos: pos, Name: "show", Params: []string{"x"}, Body: &ast.Block{Result: &ast.Match{
			Scrut: local("x", types.Unit), Arms: []ast.Arm{{
				Pat: &ast.PCtor{Pos: pos, Name: "Box", Args: []ast.Pattern{&ast.PVar{Name: "field"}}},
				Body: &ast.Interp{
					ExprBase: annotated(&types.Ann{Type: types.String, ShowDicts: []types.DictRef{param}}),
					Texts:    []string{"Box{value: ", "}"}, Exprs: []ast.Expr{local("field", a)},
				},
			}},
		}}}},
	}
	instance := &types.Instance{Class: p.Classes["Show"], TypeCon: &types.TCon{Name: "Box"}, Params: []*types.TVar{a}, Context: []types.Constraint{{Class: "Show", Arg: a}}, Decl: decl}
	p.Instances = append(p.Instances, instance)
	concrete := instanceDict(instance, instanceDict(showInt))
	addFunc(p, "render", nil, nil, call(method("show", fnType(types.Unit), concrete), call(ctor("Box"), integer(7))))
	assertSource(t, p,
		"type Show_Box struct", "dict_A Show", "func (dictself Show_Box) Show(x_ any) any",
		`return rt.Concat("Box{value: ", (dictself.dict_A).Show(field_), "}")`,
		"return (Show_Box{dict_A: Show_Int{}}).Show(&Box_Box{F0: int64(7)})")
}
