package ast

// Alternatives expands the or-patterns inside p into the or-free patterns it
// stands for, in source order (ADR 0020). A pattern without POr yields itself.
// Nested alternatives multiply out left to right: `(A | B, C | D)` yields
// `(A, C)`, `(A, D)`, `(B, C)`, `(B, D)`.
//
// The results share every node that contains no POr with p, so call this only
// on patterns whose names are already resolved: a pass that rewrites pattern
// nodes in place must handle POr itself and run before any expansion.
func Alternatives(p Pattern) []Pattern {
	switch p := p.(type) {
	case *POr:
		var out []Pattern
		for _, alt := range p.Alts {
			out = append(out, Alternatives(alt)...)
		}
		return out
	case *PCtor:
		args := product(p.Args)
		if args == nil {
			return []Pattern{p}
		}
		out := make([]Pattern, len(args))
		for i, a := range args {
			out[i] = &PCtor{Pos: p.Pos, End: p.End, Name: p.Name, Args: a, List: p.List}
		}
		return out
	case *PTuple:
		elems := product(p.Elems)
		if elems == nil {
			return []Pattern{p}
		}
		out := make([]Pattern, len(elems))
		for i, e := range elems {
			out[i] = &PTuple{Pos: p.Pos, End: p.End, Elems: e}
		}
		return out
	case *PRecord:
		pats := product(p.Pats)
		if pats == nil {
			return []Pattern{p}
		}
		out := make([]Pattern, len(pats))
		for i, fields := range pats {
			out[i] = &PRecord{Pos: p.Pos, End: p.End, Ctor: p.Ctor, Names: p.Names, Pats: fields, Rest: p.Rest}
		}
		return out
	case *PAs:
		inner := Alternatives(p.Pat)
		if len(inner) == 1 && inner[0] == p.Pat {
			return []Pattern{p}
		}
		out := make([]Pattern, len(inner))
		for i, pat := range inner {
			out[i] = &PAs{Pos: p.Pos, End: p.End, Name: p.Name, Pat: pat}
		}
		return out
	}
	return []Pattern{p}
}

// product returns every combination of the alternatives of each pattern, or
// nil when no pattern contains an or-pattern (the caller then reuses its node).
func product(ps []Pattern) [][]Pattern {
	expanded := make([][]Pattern, len(ps))
	changed := false
	for i, p := range ps {
		expanded[i] = Alternatives(p)
		if len(expanded[i]) != 1 || expanded[i][0] != p {
			changed = true
		}
	}
	if !changed {
		return nil
	}
	rows := [][]Pattern{nil}
	for _, alts := range expanded {
		next := make([][]Pattern, 0, len(rows)*len(alts))
		for _, row := range rows {
			for _, alt := range alts {
				next = append(next, append(append(make([]Pattern, 0, len(row)+1), row...), alt))
			}
		}
		rows = next
	}
	return rows
}
