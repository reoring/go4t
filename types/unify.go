package types

import "gofx/ast"

func sameKind(a, b Kind) bool {
	switch a := a.(type) {
	case KStar:
		_, ok := b.(KStar)
		return ok
	case KArrow:
		b, ok := b.(KArrow)
		if !ok || len(a.Params) != len(b.Params) || !sameKind(a.Ret, b.Ret) {
			return false
		}
		for i, param := range a.Params {
			if !sameKind(param, b.Params[i]) {
				return false
			}
		}
		return true
	default:
		return false
	}
}

func requireKind(pos ast.Pos, got, want Kind) {
	if !sameKind(got, want) {
		fail(pos, "kind mismatch: got %s, want %s", KindString(got), KindString(want))
	}
}

func kindOf(t Type) Kind {
	switch t := prune(t).(type) {
	case *TCon:
		return t.Kind
	case *TVar:
		return t.Kind
	case *TMeta:
		return t.Kind
	case *TApp:
		return kindOf(t.Fun).(KArrow).Ret
	case *TFunc:
		return KStar{}
	default:
		panic("invalid semantic type")
	}
}

// prune follows and compresses union-find links. Binding always performs an
// occurs check first, so neither links nor structural types can contain cycles.
func prune(t Type) Type {
	if m, ok := t.(*TMeta); ok && m.Ref != nil {
		*m.Ref = prune(*m.Ref)
		return *m.Ref
	}
	return t
}

func (c *checker) fresh(kind Kind) *TMeta {
	c.nextMeta++
	return &TMeta{ID: c.nextMeta, Kind: kind}
}

func occurs(m *TMeta, t Type) bool {
	t = prune(t)
	if t == m {
		return true
	}
	switch t := t.(type) {
	case *TApp:
		if occurs(m, t.Fun) {
			return true
		}
		for _, arg := range t.Args {
			if occurs(m, arg) {
				return true
			}
		}
	case *TFunc:
		for _, param := range t.Params {
			if occurs(m, param) {
				return true
			}
		}
		return occurs(m, t.Ret)
	}
	return false
}

// unify reports structural mismatches using the outermost expected and actual
// types supplied by the context, not the nested pair that failed. Types are
// rendered only on failure, following any inference links resolved along the
// way. Kind errors and occurs checks retain their more specific diagnostics.
func unify(pos ast.Pos, expected, actual Type) {
	if !unifyTypes(pos, expected, actual) {
		fail(pos, "type mismatch: expected %s, got %s", TypeString(expected), TypeString(actual))
	}
}

func bindMeta(pos ast.Pos, meta *TMeta, typ Type) {
	if occurs(meta, typ) {
		fail(pos, "occurs check: %s occurs in %s", TypeString(meta), TypeString(typ))
	}
	meta.Ref = &typ
}

func unifyTypes(pos ast.Pos, expected, actual Type) bool {
	expected, actual = prune(expected), prune(actual)
	if expected == actual {
		return true
	}
	requireKind(pos, kindOf(actual), kindOf(expected))
	if meta, ok := expected.(*TMeta); ok {
		bindMeta(pos, meta, actual)
		return true
	}
	if meta, ok := actual.(*TMeta); ok {
		bindMeta(pos, meta, expected)
		return true
	}
	switch expected := expected.(type) {
	case *TCon:
		actual, ok := actual.(*TCon)
		return ok && expected.Name == actual.Name
	case *TApp:
		if actual, ok := actual.(*TApp); ok && len(expected.Args) == len(actual.Args) {
			if !unifyTypes(pos, expected.Fun, actual.Fun) {
				return false
			}
			for i, arg := range expected.Args {
				if !unifyTypes(pos, arg, actual.Args[i]) {
					return false
				}
			}
			return true
		}
	case *TFunc:
		if actual, ok := actual.(*TFunc); ok && len(expected.Params) == len(actual.Params) {
			for i, param := range expected.Params {
				if !unifyTypes(pos, param, actual.Params[i]) {
					return false
				}
			}
			return unifyTypes(pos, expected.Ret, actual.Ret)
		}
	}
	// TVars are rigid and scoped by identity, never just by their printed name.
	return false
}

func substitute(t Type, sub map[*TVar]Type) Type {
	switch t := t.(type) {
	case *TVar:
		if replacement := sub[t]; replacement != nil {
			return replacement
		}
	case *TApp:
		fun, args := substitute(t.Fun, sub), substituteList(t.Args, sub)
		if fun != t.Fun || args != nil {
			if args == nil {
				args = t.Args
			}
			return &TApp{Fun: fun, Args: args}
		}
	case *TFunc:
		params, ret := substituteList(t.Params, sub), substitute(t.Ret, sub)
		if params != nil || ret != t.Ret {
			if params == nil {
				params = t.Params
			}
			return &TFunc{Params: params, Ret: ret}
		}
	}
	return t
}

// nil means the original list is unchanged and can be shared with its scheme.
func substituteList(ts []Type, sub map[*TVar]Type) []Type {
	var changed []Type
	for i, t := range ts {
		r := substitute(t, sub)
		if r != t && changed == nil {
			changed = append([]Type(nil), ts...)
		}
		if changed != nil {
			changed[i] = r
		}
	}
	return changed
}

func subConstraints(cs []Constraint, sub map[*TVar]Type) []Constraint {
	if len(cs) == 0 {
		return nil
	}
	out := make([]Constraint, len(cs))
	for i, c := range cs {
		out[i] = Constraint{Class: c.Class, Arg: substitute(c.Arg, sub)}
	}
	return out
}

func unresolved(t Type) bool {
	switch t := prune(t).(type) {
	case *TMeta:
		return true
	case *TApp:
		if unresolved(t.Fun) {
			return true
		}
		for _, arg := range t.Args {
			if unresolved(arg) {
				return true
			}
		}
	case *TFunc:
		for _, param := range t.Params {
			if unresolved(param) {
				return true
			}
		}
		return unresolved(t.Ret)
	}
	return false
}

// zonk is called only after checking every class obligation for unresolved
// metas. Unconstrained ordinary metas then have the canonical default Unit.
func zonk(pos ast.Pos, t Type) Type {
	t = prune(t)
	switch t := t.(type) {
	case *TMeta:
		if !sameKind(t.Kind, KStar{}) {
			fail(pos, "cannot infer type constructor of kind %s", KindString(t.Kind))
		}
		var unit Type = Unit
		t.Ref = &unit
		return Unit
	case *TApp:
		// Instances of generic types are private to this body; rigid types in
		// declaration schemes contain no metas and can be shared unchanged.
		fun := zonk(pos, t.Fun)
		var args []Type
		for i, arg := range t.Args {
			z := zonk(pos, arg)
			if z != arg && args == nil {
				args = append([]Type(nil), t.Args...)
			}
			if args != nil {
				args[i] = z
			}
		}
		if fun != t.Fun || args != nil {
			if args == nil {
				args = t.Args
			}
			return &TApp{Fun: fun, Args: args}
		}
	case *TFunc:
		var params []Type
		for i, param := range t.Params {
			z := zonk(pos, param)
			if z != param && params == nil {
				params = append([]Type(nil), t.Params...)
			}
			if params != nil {
				params[i] = z
			}
		}
		ret := zonk(pos, t.Ret)
		if params != nil || ret != t.Ret {
			if params == nil {
				params = t.Params
			}
			return &TFunc{Params: params, Ret: ret}
		}
	}
	return t
}
