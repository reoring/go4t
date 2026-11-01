package types

import (
	"strings"

	"gofx/ast"
)

func (b *bodyChecker) recordLit(e *ast.RecordLit, env *locals, ann *Ann, expected Type) Type {
	ctor := b.c.ctors[e.Ctor]
	if ctor == nil {
		fail(e.GetPos(), "unknown constructor %s", diagnosticName(e.Ctor))
	}
	if ctor.FieldNames == nil {
		fail(e.GetPos(), "constructor %s is not a record", diagnosticName(e.Ctor))
	}
	fields, result, inst := b.c.instantiateCtor(ctor)
	ann.Ctor, ann.Inst = ctor, inst
	ann.FieldIndexes = recordIndexes(e.GetPos(), ctor, e.Names, true)
	if expected != nil {
		unify(e.GetPos(), expected, result)
	}
	b.recordValues(e.Values, ann.FieldIndexes, fields, env)
	return result
}

func recordIndexes(pos ast.Pos, ctor *Ctor, names []string, complete bool) []int {
	indexes := make([]int, len(names))
	seen := make([]bool, len(ctor.Fields))
	for i, name := range names {
		checkValueName(pos, name)
		index := fieldIndex(ctor, name)
		if index < 0 {
			fail(pos, "unknown field %s in constructor %s", name, diagnosticName(ctor.Name))
		}
		if seen[index] {
			fail(pos, "duplicate field %s in constructor %s", name, diagnosticName(ctor.Name))
		}
		seen[index], indexes[i] = true, index
	}
	if complete {
		for i, name := range ctor.FieldNames {
			if !seen[i] {
				fail(pos, "missing field %s in constructor %s", name, diagnosticName(ctor.Name))
			}
		}
	}
	return indexes
}

func fieldIndex(ctor *Ctor, name string) int {
	for i, field := range ctor.FieldNames {
		if field == name {
			return i
		}
	}
	return -1
}

func recordReceiverName(e ast.Expr) string {
	switch e := e.(type) {
	case *ast.Ident:
		return e.DisplayName()
	case *ast.Field:
		return recordReceiverName(e.X) + "." + e.Name
	default:
		return expressionKind(e)
	}
}

func (b *bodyChecker) recordType(pos ast.Pos, receiver ast.Expr, typ Type, field string) (*Ctor, []Type) {
	typ = normalize(typ)
	if _, ok := typ.(*TMeta); ok {
		fail(pos, "cannot infer record type of %s.%s; annotate", recordReceiverName(receiver), field)
	}
	head := typ
	var args []Type
	if app, ok := typ.(*TApp); ok {
		head, args = prune(app.Fun), app.Args
	}
	if con, ok := head.(*TCon); ok {
		if data := b.c.program.DataTypes[con.Name]; data != nil {
			ctor := data.Ctors[0]
			sub := make(map[*TVar]Type, len(data.TParams))
			for i, param := range data.TParams {
				sub[param] = args[i]
			}
			fields := substituteList(ctor.Fields, sub)
			if fields == nil {
				fields = ctor.Fields
			}
			return ctor, fields
		}
	}
	fail(pos, "field %s requires a record type", field)
	return nil, nil
}

func commonRecordFields(pos ast.Pos, ctor *Ctor, names []string, ann *Ann) {
	ann.Ctor = ctor
	ann.FieldIndexes = recordIndexes(pos, ctor, names, false)
	if len(ctor.Data.Ctors) == 1 {
		return
	}
	ann.RecordCases = make([]RecordCase, len(ctor.Data.Ctors))
	ann.RecordCases[0] = RecordCase{Ctor: ctor, Indexes: ann.FieldIndexes}
	// Check presence first, in declaration order, so the diagnostic always
	// names the first constructor missing one of the requested fields.
	for i, other := range ctor.Data.Ctors[1:] {
		ann.RecordCases[i+1] = RecordCase{Ctor: other, Indexes: recordIndexes(pos, other, names, false)}
	}
	for _, record := range ann.RecordCases[1:] {
		for i, index := range record.Indexes {
			expected, actual := ctor.Fields[ann.FieldIndexes[i]], record.Ctor.Fields[index]
			// Declaration types contain no inference metas: this comparison
			// cannot specialize the receiver to make incompatible fields agree.
			if !unifyTypes(pos, expected, actual) {
				fail(pos, "field %s has type %s in constructor %s; expected %s from constructor %s",
					names[i], actual, diagnosticName(record.Ctor.Name), expected, diagnosticName(ctor.Name))
			}
		}
	}
}

func (b *bodyChecker) field(e *ast.Field, env *locals, ann *Ann) Type {
	if e.Index >= 0 {
		typ := b.infer(e.X, env)
		shape := normalize(typ)
		if _, ok := shape.(*TMeta); ok {
			fail(e.GetPos(), "cannot infer tuple type of %s.%s; annotate", recordReceiverName(e.X), e.Name)
		}
		if app, ok := shape.(*TApp); ok {
			if con, ok := prune(app.Fun).(*TCon); ok {
				if n, tuple := IsTuple(con); tuple {
					if e.Index >= n {
						fields := make([]any, len(app.Args))
						for i, field := range app.Args {
							fields[i] = field
						}
						format := strings.TrimSuffix(strings.Repeat("%s, ", len(fields)), ", ")
						fail(e.GetPos(), "tuple index %d out of range for (%s)", e.Index, messagef(format, fields...))
					}
					ann.Ctor, ann.FieldIndex = nil, e.Index
					return app.Args[e.Index]
				}
			}
		}
		fail(e.GetPos(), "tuple index requires a tuple type, got %s", typ)
	}
	checkValueName(e.GetPos(), e.Name)
	typ := b.infer(e.X, env)
	ctor, fields := b.recordType(e.GetPos(), e.X, typ, e.Name)
	commonRecordFields(e.GetPos(), ctor, []string{e.Name}, ann)
	ann.FieldIndex = ann.FieldIndexes[0]
	return fields[ann.FieldIndex]
}

func (b *bodyChecker) recordUpdate(e *ast.RecordUpdate, env *locals, ann *Ann) Type {
	typ := b.infer(e.X, env)
	name := ""
	if len(e.Names) > 0 {
		name = e.Names[0]
		checkValueName(e.GetPos(), name)
	}
	ctor, fields := b.recordType(e.GetPos(), e.X, typ, name)
	commonRecordFields(e.GetPos(), ctor, e.Names, ann)
	b.recordValues(e.Values, ann.FieldIndexes, fields, env)
	return typ
}

func (b *bodyChecker) recordValues(values []ast.Expr, indexes []int, fields []Type, env *locals) {
	for i, value := range values {
		unify(value.GetPos(), fields[indexes[i]], b.inferExpected(value, env, fields[indexes[i]]))
	}
}
