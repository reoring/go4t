package types

import "gofx/ast"

func (b *bodyChecker) recordLit(e *ast.RecordLit, env *locals, ann *Ann) Type {
	ctor := b.c.ctors[e.Ctor]
	if ctor == nil {
		fail(e.GetPos(), "unknown constructor %s", e.Ctor)
	}
	if ctor.FieldNames == nil {
		fail(e.GetPos(), "constructor %s is not a record", e.Ctor)
	}
	fields, result, inst := b.c.instantiateCtor(ctor)
	ann.Ctor, ann.Inst = ctor, inst
	ann.FieldIndexes = recordIndexes(e.GetPos(), ctor, e.Names, true)
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
			fail(pos, "unknown field %s in constructor %s", name, ctor.Name)
		}
		if seen[index] {
			fail(pos, "duplicate field %s in constructor %s", name, ctor.Name)
		}
		seen[index], indexes[i] = true, index
	}
	if complete {
		for i, name := range ctor.FieldNames {
			if !seen[i] {
				fail(pos, "missing field %s in constructor %s", name, ctor.Name)
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
		return e.Name
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
		if data := b.c.program.DataTypes[con.Name]; data != nil && len(data.Ctors) == 1 {
			ctor := data.Ctors[0]
			if ctor.FieldNames != nil && (field == "" || fieldIndex(ctor, field) >= 0) {
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
	}
	fail(pos, "field %s requires a single-constructor record type", field)
	return nil, nil
}

func (b *bodyChecker) field(e *ast.Field, env *locals, ann *Ann) Type {
	checkValueName(e.GetPos(), e.Name)
	typ := b.infer(e.X, env)
	ctor, fields := b.recordType(e.GetPos(), e.X, typ, e.Name)
	ann.Ctor, ann.FieldIndex = ctor, fieldIndex(ctor, e.Name)
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
	ann.Ctor = ctor
	ann.FieldIndexes = recordIndexes(e.GetPos(), ctor, e.Names, false)
	b.recordValues(e.Values, ann.FieldIndexes, fields, env)
	return typ
}

func (b *bodyChecker) recordValues(values []ast.Expr, indexes []int, fields []Type, env *locals) {
	for i, value := range values {
		unify(value.GetPos(), fields[indexes[i]], b.infer(value, env))
	}
}
