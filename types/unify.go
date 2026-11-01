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

func unify(pos ast.Pos, a, b Type) {
	a, b = prune(a), prune(b)
	if a == b {
		return
	}
	requireKind(pos, kindOf(a), kindOf(b))
	if m, ok := a.(*TMeta); ok {
		if occurs(m, b) {
			fail(pos, "occurs check: %s occurs in %s", TypeString(m), TypeString(b))
		}
		m.Ref = &b
		return
	}
	if _, ok := b.(*TMeta); ok {
		unify(pos, b, a)
		return
	}
	switch a := a.(type) {
	case *TCon:
		if b, ok := b.(*TCon); ok && a.Name == b.Name {
			return
		}
	case *TApp:
		if b, ok := b.(*TApp); ok {
			if len(a.Args) != len(b.Args) {
				fail(pos, "type application arity mismatch")
			}
			unify(pos, a.Fun, b.Fun)
			for i, arg := range a.Args {
				unify(pos, arg, b.Args[i])
			}
			return
		}
	case *TFunc:
		if b, ok := b.(*TFunc); ok {
			if len(a.Params) != len(b.Params) {
				fail(pos, "arity mismatch: want %d arguments, got %d", len(a.Params), len(b.Params))
			}
			for i, param := range a.Params {
				unify(pos, param, b.Params[i])
			}
			unify(pos, a.Ret, b.Ret)
			return
		}
	}
	// TVars are rigid and scoped by identity, never just by their printed name.
	fail(pos, "cannot unify %s with %s", TypeString(a), TypeString(b))
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
