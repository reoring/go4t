package codegen

import (
	"strings"
	"testing"

	"gofx/ast"
	"gofx/types"
)

func TestRecordPatterns(t *testing.T) {
	for _, tt := range []struct {
		name     string
		partial  bool
		nested   bool
		final    bool
		multiple bool
		literal  bool
		guarded  bool
	}{
		{name: "reordered"},
		{name: "partial", partial: true},
		{name: "nested in tuple", partial: true, nested: true},
		{name: "final arm", partial: true, final: true, multiple: true},
		{name: "final nested single constructor", partial: true, nested: true, final: true},
		{name: "final nested multiple constructors", partial: true, nested: true, final: true, multiple: true, guarded: true},
		{name: "final nested literal", partial: true, final: true, literal: true, guarded: true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			p := fixture()
			data := &types.DataType{Name: "Person"}
			ctor := &types.Ctor{Name: "Person", Data: data, Fields: []types.Type{types.String, types.Int}, FieldNames: []string{"name", "age"}}
			data.Ctors = []*types.Ctor{ctor}
			if tt.multiple {
				data.Ctors = append(data.Ctors, &types.Ctor{Name: "Nobody", Data: data, Index: 1})
			}
			p.DataTypes[data.Name] = data
			var age ast.Pattern = &ast.PVar{Name: "age"}
			if tt.literal {
				age = &ast.PLit{X: integer(42)}
			}
			record := &ast.PRecord{Ctor: "Person", Names: []string{"age"}, Pats: []ast.Pattern{age}, Rest: tt.partial}
			indexes := []int{1}
			var name ast.Pattern = &ast.PWild{}
			if !tt.partial {
				name = &ast.PVar{Name: "name"}
				record.Names = append(record.Names, "name")
				record.Pats = append(record.Pats, name)
				indexes = append(indexes, 0)
			}
			p.RecordPats = map[*ast.PRecord]types.RecordPat{record: {Ctor: ctor, Indexes: indexes}}
			wrap := func(pattern ast.Pattern) ast.Pattern {
				if tt.nested {
					return &ast.PTuple{Elems: []ast.Pattern{pattern, &ast.PWild{}}}
				}
				return pattern
			}
			match := &ast.Match{Scrut: local("x", types.Unit), Arms: []ast.Arm{{Pat: wrap(record), Body: integer(1)}}}
			if !tt.final {
				match.Arms = append(match.Arms, ast.Arm{Pat: &ast.PWild{}, Body: integer(0)})
			}
			addFunc(p, "match", []string{"x"}, []types.Type{types.Unit}, match)
			out := assertSource(t, p)
			if tt.final {
				if got := strings.Contains(out, `panic("non-exhaustive match")`); got != tt.guarded {
					t.Fatalf("final record arm panic = %t, want %t:\n%s", got, tt.guarded, out)
				}
				if !tt.guarded && strings.Contains(out, "case *Person_Person:") {
					t.Fatalf("final record head still guarded:\n%s", out)
				}
			}
			if !tt.literal && !strings.Contains(out, "var age_ ") {
				t.Fatalf("record field binding missing:\n%s", out)
			}
			// The checker side map alone determines the positional order. The
			// record lowering must preserve all positional continuation rules.
			match.Arms[0].Pat = wrap(&ast.PCtor{Name: "Person", Args: []ast.Pattern{name, age}})
			if positional := assertSource(t, p); out != positional {
				t.Fatalf("record lowering differs from positional lowering:\nrecord:\n%s\npositional:\n%s", out, positional)
			}
		})
	}
}

func TestTupleFields(t *testing.T) {
	p := fixture()
	inner := &types.TApp{Fun: types.Tuple(2), Args: []types.Type{types.Int, types.String}}
	outer := &types.TApp{Fun: types.Tuple(3), Args: []types.Type{types.Bool, inner, types.Int}}
	field := func(x ast.Expr, index int, typ types.Type) *ast.Field {
		return &ast.Field{ExprBase: annotated(&types.Ann{Type: typ}), X: x, Index: index}
	}
	addFunc(p, "first", []string{"pair"}, []types.Type{inner}, field(local("pair", inner), 0, types.Int))
	addFunc(p, "chain", []string{"triple"}, []types.Type{outer}, field(field(local("triple", outer), 1, inner), 1, types.String))
	addFunc(p, "literal", nil, nil, field(tupleLiteral(integer(7), text("value")), 1, types.String))
	assertSource(t, p,
		"any(pair_).(*Tuple2).F0",
		"return any(any(triple_).(*Tuple3).F1).(*Tuple2).F1",
		`return any(&Tuple2{F0: int64(7), F1: "value"}).(*Tuple2).F1`,
		"type Tuple2 struct {", "type Tuple3 struct {")
}
