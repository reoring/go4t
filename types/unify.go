package types

import "github.com/reoring/go4t/ast"

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
		k := kindOf(t.Fun).(KArrow)
		if len(t.Args) < len(k.Params) {
			return KArrow{Params: k.Params[len(t.Args):], Ret: k.Ret}
		}
		return k.Ret
	case *TFunc:
		return KStar{}
	default:
		panic("invalid semantic type")
	}
}

// prune follows and compresses union-find links and unwraps alias provenance.
// Binding performs an occurs check first, so semantic types cannot contain cycles.
func prune(t Type) Type {
	t = Unalias(t)
	if m, ok := t.(*TMeta); ok && m.Ref != nil {
		*m.Ref = prune(*m.Ref)
		return *m.Ref
	}
	return t
}

// Inference may resolve an application's head to another application after its
// construction. Flatten only that case; unchanged types remain shared.
func normalize(t Type) Type {
	t = prune(t)
	if app, ok := t.(*TApp); ok {
		if _, nested := prune(app.Fun).(*TApp); nested {
			return App(app.Fun, app.Args...)
		}
	}
	return t
}

func (c *checker) fresh(kind Kind) *TMeta {
	c.nextMeta++
	return &TMeta{ID: c.nextMeta, Kind: kind}
}

// bindingType performs the occurs check and removes alias provenance in one
// traversal. Inferred metas must not inherit source spellings, even inside a
// structural type. Copy only changed branches; rigid TVar identities are shared.
func bindingType(pos ast.Pos, meta *TMeta, root, typ Type) Type {
	typ = prune(typ)
	if typ == meta {
		fail(pos, "occurs check: %s occurs in %s", meta, root)
	}
	switch typ := typ.(type) {
	case *TApp:
		fun := bindingType(pos, meta, root, typ.Fun)
		args := bindingTypes(pos, meta, root, typ.Args)
		if fun != typ.Fun || args != nil {
			if args == nil {
				args = typ.Args
			}
			return App(fun, args...)
		}
	case *TFunc:
		params := bindingTypes(pos, meta, root, typ.Params)
		ret := bindingType(pos, meta, root, typ.Ret)
		if params != nil || ret != typ.Ret {
			if params == nil {
				params = typ.Params
			}
			return &TFunc{Params: params, Ret: ret}
		}
	}
	return typ
}

func bindingTypes(pos ast.Pos, meta *TMeta, root Type, ts []Type) []Type {
	var changed []Type
	for i, t := range ts {
		r := bindingType(pos, meta, root, t)
		if r != t && changed == nil {
			changed = append([]Type(nil), ts...)
		}
		if changed != nil {
			changed[i] = r
		}
	}
	return changed
}

// unify reports structural mismatches using the outermost expected and actual
// types supplied by the context, not the nested pair that failed. Types are
// rendered only on failure, following any inference links resolved along the
// way. Kind errors and occurs checks retain their more specific diagnostics.
func unify(pos ast.Pos, expected, actual Type) {
	if !unifyTypes(pos, expected, actual) {
		fail(pos, "type mismatch: expected %s, got %s", expected, actual)
	}
}

func bindMeta(pos ast.Pos, meta *TMeta, typ Type) {
	typ = bindingType(pos, meta, typ, typ)
	meta.Ref = &typ
}

func unifyTypes(pos ast.Pos, expected, actual Type) bool {
	expected, actual = normalize(expected), normalize(actual)
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
		if actual, ok := actual.(*TApp); ok {
			ef, ea, af, aa := expected.Fun, expected.Args, actual.Fun, actual.Args
			if extra := len(ea) - len(aa); extra > 0 {
				ef, ea = App(ef, ea[:extra]...), ea[extra:]
			} else if extra < 0 {
				af, aa = App(af, aa[:-extra]...), aa[-extra:]
			}
			if !unifyTypes(pos, ef, af) {
				return false
			}
			for i, arg := range ea {
				if !unifyTypes(pos, arg, aa[i]) {
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
	if a, ok := t.(*TAlias); ok {
		args, body := substituteList(a.Args, sub), substitute(a.Body, sub)
		if args == nil && body == a.Body {
			return a
		}
		if args == nil {
			args = a.Args
		}
		return &TAlias{Name: a.Name, Display: a.Display, Args: args, Body: body}
	}
	t = prune(t)
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
			return App(fun, args...)
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

// zonk runs after class ambiguity checks. Each ordinary meta is defaulted once,
// notifying the body checker before following the new Unit link.
func zonk(pos ast.Pos, t Type, onDefault func(*TMeta)) Type {
	if a, ok := t.(*TAlias); ok {
		var args []Type
		for i, arg := range a.Args {
			z := zonk(pos, arg, onDefault)
			if z != arg && args == nil {
				args = append([]Type(nil), a.Args...)
			}
			if args != nil {
				args[i] = z
			}
		}
		body := zonk(pos, a.Body, onDefault)
		if args == nil && body == a.Body {
			return a
		}
		if args == nil {
			args = a.Args
		}
		return &TAlias{Name: a.Name, Display: a.Display, Args: args, Body: body}
	}
	t = prune(t)
	switch t := t.(type) {
	case *TMeta:
		if !sameKind(t.Kind, KStar{}) {
			fail(pos, "cannot infer type constructor of kind %s", KindString(t.Kind))
		}
		onDefault(t)
		var unit Type = Unit
		t.Ref = &unit
		return Unit
	case *TApp:
		// Instances of generic types are private to this body; rigid types in
		// declaration schemes contain no metas and can be shared unchanged.
		fun := zonk(pos, t.Fun, onDefault)
		var args []Type
		for i, arg := range t.Args {
			z := zonk(pos, arg, onDefault)
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
			return App(fun, args...)
		}
	case *TFunc:
		var params []Type
		for i, param := range t.Params {
			z := zonk(pos, param, onDefault)
			if z != param && params == nil {
				params = append([]Type(nil), t.Params...)
			}
			if params != nil {
				params[i] = z
			}
		}
		ret := zonk(pos, t.Ret, onDefault)
		if params != nil || ret != t.Ret {
			if params == nil {
				params = t.Params
			}
			return &TFunc{Params: params, Ret: ret}
		}
	}
	return t
}
