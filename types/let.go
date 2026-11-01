package types

import (
	"fmt"

	"gofx/ast"
)

// Local schemes retain the inferred metas until body finalization. Used schemes
// make those metas rigid for the erased value's annotations; an unused,
// unconstrained value instead follows ordinary defaulting (and warns).
type localScheme struct {
	scheme *Scheme
	typ    Type
	metas  []*TMeta
	used   bool
}

func visitMetas(t Type, visit func(*TMeta)) {
	switch t := prune(t).(type) {
	case *TMeta:
		visit(t)
	case *TApp:
		visitMetas(t.Fun, visit)
		for _, arg := range t.Args {
			visitMetas(arg, visit)
		}
	case *TFunc:
		for _, param := range t.Params {
			visitMetas(param, visit)
		}
		visitMetas(t.Ret, visit)
	}
}

func replaceMetas(t Type, sub map[*TMeta]Type) Type {
	if a, ok := t.(*TAlias); ok {
		args, body := replaceMetaList(a.Args, sub), replaceMetas(a.Body, sub)
		if args == nil && body == a.Body {
			return a
		}
		if args == nil {
			args = a.Args
		}
		return &TAlias{Name: a.Name, Args: args, Body: body}
	}
	t = prune(t)
	switch t := t.(type) {
	case *TMeta:
		if replacement := sub[t]; replacement != nil {
			return replacement
		}
	case *TApp:
		fun := replaceMetas(t.Fun, sub)
		args := replaceMetaList(t.Args, sub)
		if fun != t.Fun || args != nil {
			if args == nil {
				args = t.Args
			}
			return App(fun, args...)
		}
	case *TFunc:
		params, ret := replaceMetaList(t.Params, sub), replaceMetas(t.Ret, sub)
		if params != nil || ret != t.Ret {
			if params == nil {
				params = t.Params
			}
			return &TFunc{Params: params, Ret: ret}
		}
	}
	return t
}

func replaceMetaList(ts []Type, sub map[*TMeta]Type) []Type {
	var changed []Type
	for i, t := range ts {
		r := replaceMetas(t, sub)
		if r != t && changed == nil {
			changed = append([]Type(nil), ts...)
		}
		if changed != nil {
			changed[i] = r
		}
	}
	return changed
}

// Value restriction: evaluating a generalized value cannot perform a call or
// effect. Constructor arguments and literal elements obey the same restriction.
func (b *bodyChecker) generalizable(e ast.Expr) bool {
	switch e := e.(type) {
	case *ast.Lambda, *ast.IntLit, *ast.FloatLit, *ast.StringLit, *ast.BoolLit:
		return true
	case *ast.Ident:
		return AnnOf(e).Ref == RefCtor
	case *ast.Call:
		id, ok := e.Fn.(*ast.Ident)
		if !ok || AnnOf(id).Ref != RefCtor {
			return false
		}
		return b.generalizableValues(e.Args)
	case *ast.RecordLit:
		return b.generalizableValues(e.Values)
	case *ast.TupleLit:
		return b.generalizableValues(e.Elems)
	case *ast.ListLit:
		return b.generalizableValues(e.Elems)
	}
	return false
}

func (b *bodyChecker) generalizableValues(es []ast.Expr) bool {
	for _, e := range es {
		if !b.generalizable(e) {
			return false
		}
	}
	return true
}

func (b *bodyChecker) let(pos, namePos, end ast.Pos, name string, pat ast.Pattern, annotation ast.Type, value ast.Expr, env *locals) {
	if pat == nil {
		checkValueName(pos, name)
	}
	start := len(b.obligations)
	var expected Type
	if annotation != nil {
		expected = b.c.valueType(annotation, b.types)
	}
	valueEnv := env
	var self Type
	var recursive localUse
	if _, lambda := value.(*ast.Lambda); lambda && pat == nil {
		self = b.c.fresh(KStar{})
		valueEnv = newLocals(env)
		valueEnv.values[name] = self
		valueEnv.uses = map[string]*localUse{name: &recursive}
	}
	typ := b.inferExpected(value, valueEnv, expected)
	if self != nil {
		unify(value.GetPos(), self, typ)
		AnnOf(value).Recursive = recursive.used
	}
	if expected != nil {
		unify(value.GetPos(), expected, typ)
		typ = expected
		AnnOf(value).Type = expected
	}
	if pat != nil {
		b.localPattern(pos, pat, typ, env, "let")
		return
	}
	if b.bindings == nil {
		b.bindings = make(map[ast.Expr]defaultOrigin)
	}
	b.bindings[value] = defaultOrigin{pos: pos, what: "let " + name, priority: 2}
	// Capture the old environment before replacing a shadowed binding.
	var local *localScheme
	if annotation == nil && b.generalizable(value) {
		local = b.generalize(typ, env, start, pos)
	}
	b.bindLocal(name, typ, local, env)
	b.trackLocal(env, name, namePos, end)
	if use := env.uses[name]; use != nil {
		use.used = recursive.used
	}
	if local != nil && len(local.scheme.Constraints) != 0 {
		AnnOf(value).LetScheme = local.scheme
	}
}

func (b *bodyChecker) bindLocal(name string, typ Type, local *localScheme, env *locals) {
	env.values[name] = typ
	delete(env.schemes, name)
	delete(env.uses, name)
	if local != nil {
		if env.schemes == nil {
			env.schemes = make(map[string]*localScheme)
		}
		env.schemes[name] = local
		b.lets = append(b.lets, local)
	}
}

func (b *bodyChecker) generalize(typ Type, env *locals, start int, pos ast.Pos) *localScheme {
	if !unresolved(typ) {
		return nil
	}
	free := make(map[*TMeta]bool)
	mark := func(m *TMeta) { free[m] = true }
	visible := make(map[string]bool)
	for scope := env; scope != nil; scope = scope.parent {
		for name, t := range scope.values {
			if visible[name] {
				continue
			}
			visible[name] = true
			if local := scope.schemes[name]; local != nil {
				t = local.typ
			}
			visitMetas(t, mark)
		}
	}
	// Arithmetic dispatches statically, not via a type-class dictionary. Keep
	// its operand metas monomorphic so later context can select Int/Float/String.
	for _, expr := range b.arithmetic {
		visitMetas(AnnOf(expr).Type, mark)
	}
	var metas []*TMeta
	seen := make(map[*TMeta]bool)
	visitMetas(typ, func(m *TMeta) {
		if !free[m] && !seen[m] {
			metas = append(metas, m)
			seen[m] = true
		}
	})
	if len(metas) == 0 {
		return nil
	}
	sub := make(map[*TMeta]Type, len(metas))
	s := &Scheme{TParams: make([]*TVar, len(metas))}
	for i, meta := range metas {
		name := fmt.Sprintf("Let%d", meta.ID)
		for b.types[name] != nil {
			name += "_"
		}
		param := &TVar{Name: name, Kind: meta.Kind}
		s.TParams[i], sub[meta] = param, param
	}
	for _, o := range b.obligations[start:] {
		for _, constraint := range o.constraints {
			b.generalizeConstraint(pos, constraint, sub, s)
		}
	}
	generic := replaceMetas(typ, sub)
	if fn, ok := generic.(*TFunc); ok {
		s.Type = fn
	} else {
		// Scheme.Type is fixed to *TFunc. Non-function local values use a
		// zero-argument wrapper; localScheme.typ remains their actual value type.
		s.Type = &TFunc{Ret: generic}
	}
	return &localScheme{scheme: s, typ: generic, metas: metas}
}

func (b *bodyChecker) generalizeConstraint(pos ast.Pos, constraint Constraint, sub map[*TMeta]Type, scheme *Scheme) {
	arg := normalize(constraint.Arg)
	if meta, ok := arg.(*TMeta); ok {
		param := sub[meta]
		if param == nil {
			return
		}
		for i, existing := range scheme.Constraints {
			if existing.Arg != param {
				continue
			}
			if b.subclass(existing.Class, constraint.Class) {
				return
			}
			if b.subclass(constraint.Class, existing.Class) {
				scheme.Constraints[i].Class = constraint.Class
				return
			}
		}
		scheme.Constraints = append(scheme.Constraints, Constraint{Class: constraint.Class, Arg: param})
		return
	}
	// A structured obligation such as Show[List[A]] generalizes its instance
	// prerequisites Show[A], not an unsupported dictionary for a composite var.
	app, ok := arg.(*TApp)
	if !ok {
		return
	}
	con, ok := prune(app.Fun).(*TCon)
	if !ok {
		return
	}
	inst := b.c.instances[instanceKey{constraint.Class, con.Name}]
	if inst == nil || len(inst.headParams()) != len(app.Args) {
		return
	}
	args := make(map[*TVar]Type, len(app.Args))
	for i, param := range inst.headParams() {
		args[param] = app.Args[i]
	}
	for _, context := range inst.Context {
		b.generalizeConstraint(pos, Constraint{Class: context.Class, Arg: substitute(context.Arg, args)}, sub, scheme)
	}
}

func (b *bodyChecker) subclass(child, parent string) bool {
	for class := b.c.program.Classes[child]; class != nil; class = b.c.program.Classes[class.Super] {
		if class.Name == parent {
			return true
		}
	}
	return false
}

func (b *bodyChecker) finishLets() {
	for _, local := range b.lets {
		if !local.used && len(local.scheme.Constraints) == 0 {
			continue
		}
		for i, meta := range local.metas {
			var param Type = local.scheme.TParams[i]
			meta.Ref = &param
		}
		b.constraints = append(b.constraints, local.scheme.Constraints...)
	}
}
