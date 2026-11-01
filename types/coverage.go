package types

import "gofx/ast"

func catchall(p ast.Pattern) bool {
	switch p.(type) {
	case nil, *ast.PWild, *ast.PVar:
		return true
	default:
		return false
	}
}

func (b *bodyChecker) checkExhaustive(expr *ast.Match) {
	if len(expr.Arms) == 0 {
		fail(expr.GetPos(), "non-exhaustive match")
	}
	if catchall(expr.Arms[len(expr.Arms)-1].Pat) {
		return
	}
	rows := make([][]ast.Pattern, len(expr.Arms))
	for i, arm := range expr.Arms {
		if _, literal := arm.Pat.(*ast.PLit); literal {
			fail(expr.GetPos(), "non-exhaustive match: literal patterns require a trailing catch-all")
		}
		rows[i] = []ast.Pattern{arm.Pat}
	}
	if !b.c.covers(rows, []Type{AnnOf(expr.Scrut).Type}) {
		fail(expr.GetPos(), "non-exhaustive match")
	}
}

// covers specializes a pattern matrix one column at a time. Wildcard rows
// cover every constructor, but a constructor with only partial field patterns
// does not count as covered. Checking the default matrix first both avoids
// expanding recursive data types unnecessarily and handles primitive types,
// whose literal patterns can never exhaust their domain in this language.
func (c *checker) covers(rows [][]ast.Pattern, columns []Type) bool {
	if len(rows) == 0 {
		return false
	}
	if len(columns) == 0 {
		return true
	}
	var defaults [][]ast.Pattern
	for _, row := range rows {
		if catchall(row[0]) {
			defaults = append(defaults, row[1:])
		}
	}
	if c.covers(defaults, columns[1:]) {
		return true
	}
	var con *TCon
	var args []Type
	switch typ := columns[0].(type) {
	case *TCon:
		con = typ
	case *TApp:
		con, _ = typ.Fun.(*TCon)
		args = typ.Args
	}
	if con == nil {
		return false
	}
	data := c.program.DataTypes[con.Name]
	if data == nil {
		return false
	}
	// If a constructor is absent, only default rows can cover it. Their
	// suffixes failed above, so specialization cannot make this exhaustive.
	// In particular, do not unfold a recursive wildcard column forever.
	present := make(map[string]bool)
	for _, row := range rows {
		if p, ok := row[0].(*ast.PCtor); ok {
			present[p.Name] = true
		}
	}
	for _, ctor := range data.Ctors {
		if !present[ctor.Name] {
			return false
		}
	}
	sub := make(map[*TVar]Type, len(data.TParams))
	for i, param := range data.TParams {
		sub[param] = args[i]
	}
	for _, ctor := range data.Ctors {
		var specialized [][]ast.Pattern
		for _, row := range rows {
			if catchall(row[0]) {
				next := make([]ast.Pattern, len(ctor.Fields), len(ctor.Fields)+len(row)-1)
				next = append(next, row[1:]...)
				specialized = append(specialized, next)
			} else if p, ok := row[0].(*ast.PCtor); ok && p.Name == ctor.Name {
				next := make([]ast.Pattern, 0, len(p.Args)+len(row)-1)
				next = append(next, p.Args...)
				next = append(next, row[1:]...)
				specialized = append(specialized, next)
			}
		}
		fields := make([]Type, 0, len(ctor.Fields)+len(columns)-1)
		for _, field := range ctor.Fields {
			fields = append(fields, substitute(field, sub))
		}
		fields = append(fields, columns[1:]...)
		if !c.covers(specialized, fields) {
			return false
		}
	}
	return true
}
