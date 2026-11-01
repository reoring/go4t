package types

import "gofx/ast"

// As-bindings do not change the covered shape of their inner pattern.
func patternShape(p ast.Pattern) ast.Pattern {
	for {
		as, ok := p.(*ast.PAs)
		if !ok {
			return p
		}
		p = as.Pat
	}
}

func catchall(p ast.Pattern) bool {
	switch patternShape(p).(type) {
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
		if _, literal := patternShape(arm.Pat).(*ast.PLit); literal {
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
	if n, tuple := IsTuple(con); tuple {
		var specialized [][]ast.Pattern
		for _, row := range rows {
			var elems []ast.Pattern
			if catchall(row[0]) {
				elems = make([]ast.Pattern, n, n+len(row)-1)
			} else if p, ok := patternShape(row[0]).(*ast.PTuple); ok {
				elems = make([]ast.Pattern, n, n+len(row)-1)
				copy(elems, p.Elems)
			} else {
				continue
			}
			specialized = append(specialized, append(elems, row[1:]...))
		}
		fields := make([]Type, 0, n+len(columns)-1)
		fields = append(fields, args...)
		fields = append(fields, columns[1:]...)
		return c.covers(specialized, fields)
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
		switch p := patternShape(row[0]).(type) {
		case *ast.PCtor:
			present[p.Name] = true
		case *ast.PRecord:
			present[c.program.RecordPats[p].Ctor.Name] = true
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
			} else if p, ok := patternShape(row[0]).(*ast.PCtor); ok && p.Name == ctor.Name {
				next := make([]ast.Pattern, 0, len(p.Args)+len(row)-1)
				next = append(next, p.Args...)
				next = append(next, row[1:]...)
				specialized = append(specialized, next)
			} else if p, ok := patternShape(row[0]).(*ast.PRecord); ok && c.program.RecordPats[p].Ctor == ctor {
				next := make([]ast.Pattern, len(ctor.Fields), len(ctor.Fields)+len(row)-1)
				for i, index := range c.program.RecordPats[p].Indexes {
					next[index] = p.Pats[i]
				}
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
