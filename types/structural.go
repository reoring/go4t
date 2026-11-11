package types

import "github.com/reoring/go4t/ast"

// BindStructuralPattern propagates provenance through a checked pattern. child
// selects a known tuple field when available, otherwise a strict subterm. In
// particular, unpacking a freshly constructed tuple must not invent descent.
func BindStructuralPattern[V any](pattern ast.Pattern, value V, records map[*ast.PRecord]RecordPat, child func(V, int) V, bind func(string, V)) {
	var children []ast.Pattern
	var indexes []int
	switch p := pattern.(type) {
	case *ast.PVar:
		bind(p.Name, value)
		return
	case *ast.PAs:
		bind(p.Name, value)
		BindStructuralPattern(p.Pat, value, records, child, bind)
		return
	case *ast.PCtor:
		children = p.Args
	case *ast.PTuple:
		children = p.Elems
	case *ast.PRecord:
		children = p.Pats
		indexes = records[p].Indexes
	default:
		return
	}
	for i, p := range children {
		index := i
		if indexes != nil {
			index = indexes[i]
		}
		BindStructuralPattern(p, child(value, index), records, child, bind)
	}
}
