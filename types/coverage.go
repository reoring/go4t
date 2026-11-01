package types

import (
	"strconv"
	"strings"

	"go4t/ast"
)

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
	rows := make([][]ast.Pattern, 0, len(expr.Arms))
	for _, arm := range expr.Arms {
		if arm.Guard == nil {
			rows = append(rows, []ast.Pattern{arm.Pat})
		}
	}
	missing := b.c.uncovered(rows, []Type{AnnOf(expr.Scrut).Type}, 5)
	if len(missing) != 0 {
		patterns := make([]string, len(missing))
		for i, row := range missing {
			patterns[i] = row[0]
		}
		fail(expr.GetPos(), "non-exhaustive match; missing: %s", strings.Join(patterns, ", "))
	}
}

// covers and diagnostic witnesses share one matrix algorithm.
func (c *checker) covers(rows [][]ast.Pattern, columns []Type) bool {
	return len(c.uncovered(rows, columns, 1)) == 0
}

type coverageCase struct {
	name    string
	fields  []Type
	tuple   bool
	list    bool
	literal bool
}

func (k coverageCase) render(fields []string, names nameContext) string {
	if k.list {
		if len(fields) == 0 {
			return "[]"
		}
		switch fields[1] {
		case "_":
			return "[" + fields[0] + ", ..]"
		case "[]":
			return "[" + fields[0] + "]"
		default:
			// The tail is itself a canonical List witness; splice its
			// elements without parsing the (possibly nested) head.
			return "[" + fields[0] + ", " + fields[1][1:]
		}
	}
	if k.tuple {
		return "(" + strings.Join(fields, ", ") + ")"
	}
	if len(fields) == 0 {
		return names.name(k.name, "")
	}
	return names.name(k.name, "") + "(" + strings.Join(fields, ", ") + ")"
}

func literalPattern(p ast.Pattern) string {
	lit, ok := patternShape(p).(*ast.PLit)
	if !ok {
		return ""
	}
	switch x := lit.X.(type) {
	case *ast.IntLit:
		return strconv.FormatInt(x.Value, 10)
	case *ast.FloatLit:
		return strconv.FormatFloat(x.Value, 'g', -1, 64)
	case *ast.StringLit:
		return strconv.Quote(x.Value)
	case *ast.BoolLit:
		return strconv.FormatBool(x.Value)
	}
	return ""
}

// uncovered specializes one column at a time, returning at most limit witness
// vectors. An absent constructor uses the default suffix without unfolding its
// fields; this both yields concise wildcards and terminates on recursive types.
func (c *checker) uncovered(rows [][]ast.Pattern, columns []Type, limit int) [][]string {
	if limit == 0 {
		return nil
	}
	if len(columns) == 0 {
		if len(rows) == 0 {
			return [][]string{{}}
		}
		return nil
	}
	var defaults [][]ast.Pattern
	present := make(map[string]bool)
	for _, row := range rows {
		p := patternShape(row[0])
		if catchall(p) {
			defaults = append(defaults, row[1:])
			continue
		}
		switch p := p.(type) {
		case *ast.PCtor:
			present[p.Name] = true
		case *ast.PRecord:
			present[p.Ctor] = true
		case *ast.PTuple:
			present["tuple"] = true
		case *ast.PLit:
			present[literalPattern(p)] = true
		}
	}
	var suffixes [][]string
	if len(defaults) == 0 {
		// With no default rows every suffix is missing, so one wildcard
		// vector suffices; do not enumerate recursive suffix types.
		suffix := make([]string, len(columns)-1)
		for i := range suffix {
			suffix[i] = "_"
		}
		suffixes = [][]string{suffix}
	} else {
		suffixes = c.uncovered(defaults, columns[1:], limit)
	}
	if len(suffixes) == 0 {
		return nil
	}
	prepend := func(name string) [][]string {
		out := make([][]string, len(suffixes))
		for i, suffix := range suffixes {
			out[i] = append([]string{name}, suffix...)
		}
		return out
	}
	// An unconstrained column needs no subdivision, even when recursive.
	if len(present) == 0 && len(rows) != 0 {
		return prepend("_")
	}
	var con *TCon
	var args []Type
	switch typ := prune(columns[0]).(type) {
	case *TCon:
		con = typ
	case *TApp:
		con, _ = prune(typ.Fun).(*TCon)
		args = typ.Args
	}
	var cases []coverageCase
	if con != nil {
		if _, tuple := IsTuple(con); tuple {
			cases = append(cases, coverageCase{name: "tuple", fields: args, tuple: true})
		} else if data := c.program.DataTypes[con.Name]; data != nil {
			sub := make(map[*TVar]Type, len(data.TParams))
			for i, param := range data.TParams {
				sub[param] = args[i]
			}
			for _, ctor := range data.Ctors {
				fields := make([]Type, len(ctor.Fields))
				for i, field := range ctor.Fields {
					fields[i] = substitute(field, sub)
				}
				cases = append(cases, coverageCase{name: ctor.Name, fields: fields, list: con.Name == "List"})
			}
		} else if con == Bool && len(present) != 0 {
			cases = []coverageCase{{name: "false", literal: true}, {name: "true", literal: true}}
		}
	}
	var out [][]string
	if len(cases) == 0 {
		// Int, Float and String have an open literal domain. Pick a value not in the
		// matrix, then inspect mentioned values for holes in later columns.
		other := "_"
		if len(present) != 0 {
			if con == Int {
				for n := int64(0); ; n++ {
					other = strconv.FormatInt(n, 10)
					if !present[other] {
						break
					}
				}
			} else if con == Float {
				for n := float64(0); ; n++ {
					other = strconv.FormatFloat(n, 'g', -1, 64)
					if !present[other] {
						break
					}
				}
			} else if con == String {
				for s := ""; ; s += "_" {
					other = strconv.Quote(s)
					if !present[other] {
						break
					}
				}
			}
		}
		out = prepend(other)
		for _, row := range rows {
			name := literalPattern(row[0])
			if name != "" && present[name] {
				cases = append(cases, coverageCase{name: name, literal: true})
				delete(present, name)
			}
		}
		// These cases are present; the map was used only to deduplicate them.
		for _, k := range cases {
			present[k.name] = true
		}
	}
	for _, k := range cases {
		if len(out) == limit {
			break
		}
		if !present[k.name] {
			fields := make([]string, len(k.fields))
			for i := range fields {
				fields[i] = "_"
			}
			for _, suffix := range suffixes {
				out = append(out, append([]string{k.render(fields, c.program.namesFor(c.file))}, suffix...))
				if len(out) == limit {
					break
				}
			}
			continue
		}
		var specialized [][]ast.Pattern
		for _, row := range rows {
			var fields []ast.Pattern
			p := patternShape(row[0])
			switch {
			case catchall(p):
				fields = make([]ast.Pattern, len(k.fields))
			case k.literal:
				if literalPattern(p) != k.name {
					continue
				}
			case k.tuple:
				tuple, ok := p.(*ast.PTuple)
				if !ok {
					continue
				}
				fields = tuple.Elems
			default:
				switch p := p.(type) {
				case *ast.PCtor:
					if p.Name != k.name {
						continue
					}
					fields = p.Args
				case *ast.PRecord:
					if p.Ctor != k.name {
						continue
					}
					fields = make([]ast.Pattern, len(k.fields))
					for i, index := range c.program.RecordPats[p].Indexes {
						fields[index] = p.Pats[i]
					}
				default:
					continue
				}
			}
			next := make([]ast.Pattern, 0, len(fields)+len(row)-1)
			next = append(next, fields...)
			specialized = append(specialized, append(next, row[1:]...))
		}
		nextColumns := make([]Type, 0, len(k.fields)+len(columns)-1)
		nextColumns = append(nextColumns, k.fields...)
		nextColumns = append(nextColumns, columns[1:]...)
		for _, witness := range c.uncovered(specialized, nextColumns, limit-len(out)) {
			out = append(out, append([]string{k.render(witness[:len(k.fields)], c.program.namesFor(c.file))}, witness[len(k.fields):]...))
		}
	}
	return out
}
