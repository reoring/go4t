package types

import "gofx/ast"

type locals struct {
	parent  *locals
	values  map[string]Type
	schemes map[string]*localScheme
	uses    map[string]*localUse
}

func newLocals(parent *locals) *locals {
	return &locals{parent: parent, values: make(map[string]Type)}
}

func (l *locals) lookup(name string) (Type, *localScheme) {
	for ; l != nil; l = l.parent {
		if t := l.values[name]; t != nil {
			if use := l.uses[name]; use != nil {
				use.used = true
			}
			return t, l.schemes[name]
		}
	}
	return nil, nil
}

type obligation struct {
	expr        ast.Expr
	constraints []Constraint
	monad       bool
	show        bool
	// Operators retain ordinary Unit defaulting before dictionary resolution.
	defaultable bool
}

type patternCheck struct {
	pos     ast.Pos
	pat     ast.Pattern
	typ     Type
	context string
}

type bodyChecker struct {
	c           *checker
	constraints []Constraint
	types       map[string]Type
	exprs       []ast.Expr
	obligations []obligation
	arithmetic  []ast.Expr
	patterns    []patternCheck
	matches     []*ast.Match
	lets        []*localScheme
	bindings    map[ast.Expr]defaultOrigin
	defaults    map[*TMeta]defaultOrigin
	localUses   []*localUse
}

func (c *checker) body(constraints []Constraint, scope map[string]Type) *bodyChecker {
	return &bodyChecker{c: c, constraints: constraints, types: scope}
}

func (b *bodyChecker) check(params []ast.Param, signature *TFunc, block *ast.Block) {
	b.functionBody(params, signature, block, nil)
	b.finish()
}

func (b *bodyChecker) functionBody(params []ast.Param, signature *TFunc, block *ast.Block, outer *locals) {
	env := newLocals(outer)
	for i, param := range params {
		checkValueName(block.Pos, param.Name)
		if env.values[param.Name] != nil {
			fail(block.Pos, "duplicate parameter %s", param.Name)
		}
		env.values[param.Name] = signature.Params[i]
		if outer != nil {
			b.trackLocal(env, param.Name, param.Pos, param.End)
		}
	}
	result := b.block(block, env)
	unify(block.Result.GetPos(), signature.Ret, result)
}

func (b *bodyChecker) block(block *ast.Block, outer *locals) Type {
	env := newLocals(outer)
	for i := 0; i < len(block.Stmts); i++ {
		switch stmt := block.Stmts[i].(type) {
		case *ast.FuncDecl:
			group := []*ast.FuncDecl{stmt}
			for i+1 < len(block.Stmts) {
				next, ok := block.Stmts[i+1].(*ast.FuncDecl)
				if !ok {
					break
				}
				group = append(group, next)
				i++
			}
			b.localFunctions(group, env)
		case *ast.LetStmt:
			b.let(stmt.Pos, stmt.NamePos, stmt.End, stmt.Name, stmt.Pat, stmt.Type, stmt.Value, env)
		case *ast.ExprStmt:
			b.infer(stmt.X, env)
		}
	}
	return b.infer(block.Result, env)
}

func (c *checker) instantiate(params []*TVar) (map[*TVar]Type, []Type) {
	sub := make(map[*TVar]Type, len(params))
	inst := make([]Type, len(params))
	for i, param := range params {
		inst[i] = c.fresh(param.Kind)
		sub[param] = inst[i]
	}
	return sub, inst
}

func (b *bodyChecker) ident(e *ast.Ident, env *locals, ann *Ann) Type {
	if capitalised(e.Name) {
		if ctor := b.c.ctors[e.Name]; ctor != nil {
			fields, ret, inst := b.c.instantiateCtor(ctor)
			ann.Ref, ann.Inst, ann.Ctor = RefCtor, inst, ctor
			if len(fields) == 0 {
				return ret
			}
			return &TFunc{Params: fields, Ret: ret}
		}
		fail(e.GetPos(), "unknown identifier %s", e.Name)
	}
	if t, scheme := env.lookup(e.Name); t != nil {
		if scheme == nil {
			return t
		}
		scheme.used = true
		sub, inst := b.c.instantiate(scheme.scheme.TParams)
		ann.Inst = inst
		b.obligations = append(b.obligations, obligation{expr: e, constraints: subConstraints(scheme.scheme.Constraints, sub)})
		return substitute(scheme.typ, sub)
	}
	if f := b.c.program.Funcs[e.Name]; f != nil {
		sub, inst := b.c.instantiate(f.Scheme.TParams)
		ann.Ref, ann.Inst = RefFunc, inst
		b.obligations = append(b.obligations, obligation{expr: e, constraints: subConstraints(f.Scheme.Constraints, sub)})
		return substitute(f.Scheme.Type, sub)
	}
	if m := b.c.methods[e.Name]; m != nil {
		sub, inst := b.c.instantiate(m.Scheme.TParams)
		arg := b.c.fresh(m.Class.Param.Kind)
		sub[m.Class.Param] = arg
		ann.Ref, ann.Inst = RefMethod, inst
		cs := make([]Constraint, 1, 1+len(m.Scheme.Constraints))
		cs[0] = Constraint{Class: m.Class.Name, Arg: arg}
		cs = append(cs, subConstraints(m.Scheme.Constraints, sub)...)
		b.obligations = append(b.obligations, obligation{expr: e, constraints: cs})
		return substitute(m.Scheme.Type, sub)
	}
	fail(e.GetPos(), "unknown identifier %s", e.Name)
	return nil
}

func (c *checker) instantiateCtor(ctor *Ctor) ([]Type, Type, []Type) {
	sub, inst := c.instantiate(ctor.Data.TParams)
	fields := substituteList(ctor.Fields, sub)
	if fields == nil {
		fields = ctor.Fields
	}
	return fields, App(c.cons[ctor.Data.Name], inst...), inst
}

func (b *bodyChecker) infer(e ast.Expr, env *locals) Type {
	return b.inferExpected(e, env, nil)
}

// Call arguments carry their callee's instantiated type into an untyped
// lambda before its body is inferred. Standalone lambdas remain infer-only:
// record and tuple receivers still need a type known at the point of field access.
func (b *bodyChecker) inferExpected(e ast.Expr, env *locals, expected Type) Type {
	ann := &Ann{}
	*e.AnnSlot() = ann
	b.exprs = append(b.exprs, e)
	pos := e.GetPos()
	var result Type
	switch e := e.(type) {
	case *ast.IntLit:
		result = Int
	case *ast.FloatLit:
		result = Float
	case *ast.StringLit:
		result = String
	case *ast.Interp:
		constraints := make([]Constraint, len(e.Exprs))
		for i, expr := range e.Exprs {
			constraints[i] = Constraint{Class: "Show", Arg: b.infer(expr, env)}
		}
		b.obligations = append(b.obligations, obligation{expr: e, constraints: constraints, show: true})
		result = String
	case *ast.RecordLit:
		result = b.recordLit(e, env, ann)
	case *ast.Field:
		result = b.field(e, env, ann)
	case *ast.RecordUpdate:
		result = b.recordUpdate(e, env, ann)
	case *ast.BoolLit:
		result = Bool
	case *ast.Ident:
		result = b.ident(e, env, ann)
	case *ast.TupleLit:
		elems := make([]Type, len(e.Elems))
		for i, elem := range e.Elems {
			elems[i] = b.infer(elem, env)
		}
		result = App(Tuple(len(elems)), elems...)
	case *ast.ListLit:
		data := b.c.program.DataTypes["List"]
		nilCtor, consCtor := b.c.ctors["Nil"], b.c.ctors["Cons"]
		if data == nil || len(data.TParams) != 1 || nilCtor == nil || nilCtor.Data != data || consCtor == nil || consCtor.Data != data {
			fail(pos, "list literal requires prelude List")
		}
		item := b.c.fresh(KStar{})
		for _, elem := range e.Elems {
			unify(elem.GetPos(), item, b.infer(elem, env))
		}
		result = App(b.c.cons[data.Name], item)
	case *ast.Call:
		fn := prune(b.infer(e.Fn, env))
		ft, ok := fn.(*TFunc)
		if !ok {
			ft = &TFunc{Params: make([]Type, len(e.Args)), Ret: b.c.fresh(KStar{})}
			for i := range ft.Params {
				ft.Params[i] = b.c.fresh(KStar{})
			}
			unify(e.Fn.GetPos(), ft, fn)
		}
		if len(ft.Params) != len(e.Args) {
			fail(pos, "arity mismatch: want %d arguments, got %d", len(ft.Params), len(e.Args))
		}
		for i, arg := range e.Args {
			unify(arg.GetPos(), ft.Params[i], b.inferExpected(arg, env, ft.Params[i]))
		}
		result = ft.Ret
	case *ast.Lambda:
		local := newLocals(env)
		ft := &TFunc{Params: make([]Type, len(e.Params))}
		contextual := expected != nil
		for i, param := range e.Params {
			var typ Type
			if param.Type == nil {
				typ = b.c.fresh(KStar{})
			} else {
				typ = b.c.valueType(param.Type, b.types)
				contextual = false
			}
			ft.Params[i] = typ
		}
		if contextual {
			ft.Ret = b.c.fresh(KStar{})
			unify(pos, expected, ft)
		}
		for i, param := range e.Params {
			if param.Pat != nil {
				b.irrefutablePattern(pos, param.Pat, ft.Params[i], local, "lambda parameter")
			} else {
				checkValueName(pos, param.Name)
				if local.values[param.Name] != nil {
					fail(pos, "duplicate parameter %s", param.Name)
				}
				local.values[param.Name] = ft.Params[i]
				b.trackLocal(local, param.Name, param.Pos, param.End)
			}
		}
		ret := b.infer(e.Body, local)
		if contextual && !unifyTypes(pos, ft.Ret, ret) {
			// Keep the existing outer-function diagnostic, rather than
			// reporting only the return types after contextual inference.
			actual := &TFunc{Params: ft.Params, Ret: ret}
			fail(pos, "type mismatch: expected %s, got %s", TypeString(expected), TypeString(actual))
		}
		ft.Ret = ret
		result = ft
	case *ast.BlockExpr:
		result = b.block(e.Block, env)
	case *ast.If:
		unify(e.Cond.GetPos(), Bool, b.infer(e.Cond, env))
		result = b.infer(e.Then, env)
		unify(e.Else.GetPos(), result, b.infer(e.Else, env))
	case *ast.Match:
		scrut := b.infer(e.Scrut, env)
		result = b.c.fresh(KStar{})
		for _, arm := range e.Arms {
			local := newLocals(env)
			b.pattern(arm.Pat, scrut, local)
			if arm.Guard != nil {
				unify(arm.Guard.GetPos(), Bool, b.infer(arm.Guard, local))
			}
			unify(arm.Body.GetPos(), result, b.infer(arm.Body, local))
		}
		b.matches = append(b.matches, e)
	case *ast.Do:
		local := newLocals(env)
		head := b.c.fresh(arityKind(1))
		b.obligations = append(b.obligations, obligation{expr: e, constraints: []Constraint{{Class: "Monad", Arg: head}}, monad: true})
		for i := 0; i < len(e.Stmts); i++ {
			switch stmt := e.Stmts[i].(type) {
			case *ast.FuncDecl:
				group := []*ast.FuncDecl{stmt}
				for i+1 < len(e.Stmts) {
					next, ok := e.Stmts[i+1].(*ast.FuncDecl)
					if !ok {
						break
					}
					group = append(group, next)
					i++
				}
				b.localFunctions(group, local)
			case *ast.DoBind:
				if stmt.Pat != nil {
					b.localPattern(stmt.Pos, stmt.Pat, b.monadic(stmt.X, head, local), local, "bind")
				} else {
					checkValueName(stmt.Pos, stmt.Name)
					local.values[stmt.Name] = b.monadic(stmt.X, head, local)
					delete(local.schemes, stmt.Name)
					b.trackLocal(local, stmt.Name, stmt.Pos, stmt.End)
				}
			case *ast.DoLet:
				b.let(stmt.Pos, stmt.NamePos, stmt.End, stmt.Name, stmt.Pat, stmt.Type, stmt.Value, local)
			case *ast.DoExpr:
				b.monadic(stmt.X, head, local)
			}
		}
		item := b.monadic(e.Result, head, local)
		result = App(head, item)
	case *ast.Unary:
		arg := b.infer(e.X, env)
		switch e.Op {
		case "-":
			b.arithmetic = append(b.arithmetic, e)
			result = arg
		case "!":
			result = Bool
		default:
			fail(pos, "unknown unary operator %s", e.Op)
		}
		unify(e.X.GetPos(), result, arg)
	case *ast.Binary:
		left, right := b.infer(e.L, env), b.infer(e.R, env)
		switch e.Op {
		case "+", "-", "*", "/":
			unify(e.R.GetPos(), left, right)
			b.arithmetic = append(b.arithmetic, e)
			result = left
		case "%":
			unify(e.L.GetPos(), Int, left)
			unify(e.R.GetPos(), Int, right)
			result = Int
		case "&&", "||":
			unify(e.L.GetPos(), Bool, left)
			unify(e.R.GetPos(), Bool, right)
			result = Bool
		case "==", "!=", "<", "<=", ">", ">=":
			unify(e.R.GetPos(), left, right)
			class := "Ord"
			if e.Op == "==" || e.Op == "!=" {
				class = "Eq"
			}
			if b.c.program.Classes[class] == nil {
				fail(pos, "operator %s requires prelude %s", e.Op, class)
			}
			ann.Inst = []Type{left}
			b.obligations = append(b.obligations, obligation{
				expr: e, constraints: []Constraint{{Class: class, Arg: left}}, defaultable: true,
			})
			result = Bool
		default:
			fail(pos, "unknown binary operator %s", e.Op)
		}
	default:
		panic("unknown expression node")
	}
	ann.Type = result
	return result
}

func (b *bodyChecker) monadic(e ast.Expr, head Type, env *locals) Type {
	item := b.c.fresh(KStar{})
	unify(e.GetPos(), App(head, item), b.infer(e, env))
	return item
}

func (b *bodyChecker) irrefutablePattern(pos ast.Pos, pat ast.Pattern, typ Type, env *locals, context string) {
	b.pattern(pat, typ, env)
	b.patterns = append(b.patterns, patternCheck{pos: pos, pat: pat, typ: typ, context: context})
}

// A binding shadows earlier locals, but names within its own pattern must be
// unique. Reuse parameter pattern inference in a fresh scope, then publish the
// monomorphic variables and discard any schemes from shadowed bindings.
func (b *bodyChecker) localPattern(pos ast.Pos, pat ast.Pattern, typ Type, env *locals, context string) {
	local := newLocals(nil)
	b.irrefutablePattern(pos, pat, typ, local, context)
	for name, typ := range local.values {
		env.values[name] = typ
		delete(env.schemes, name)
		delete(env.uses, name)
		if use := local.uses[name]; use != nil {
			if env.uses == nil {
				env.uses = make(map[string]*localUse)
			}
			env.uses[name] = use
		}
	}
}

func bindPattern(pos ast.Pos, name string, expected Type, env *locals) {
	checkValueName(pos, name)
	if env.values[name] != nil {
		fail(pos, "duplicate pattern variable %s", name)
	}
	env.values[name] = expected
}

func (b *bodyChecker) pattern(p ast.Pattern, expected Type, env *locals) {
	switch p := p.(type) {
	case *ast.PWild:
	case *ast.PVar:
		bindPattern(p.Pos, p.Name, expected, env)
		b.trackLocal(env, p.Name, p.Pos, p.End)
	case *ast.PAs:
		bindPattern(p.Pos, p.Name, expected, env)
		b.trackLocal(env, p.Name, p.Pos, p.End)
		b.pattern(p.Pat, expected, env)
	case *ast.PTuple:
		elems := make([]Type, len(p.Elems))
		for i := range elems {
			elems[i] = b.c.fresh(KStar{})
		}
		unify(p.Pos, expected, App(Tuple(len(elems)), elems...))
		for i, elem := range p.Elems {
			b.pattern(elem, elems[i], env)
		}
	case *ast.PLit:
		unify(p.Pos, expected, b.infer(p.X, env))
	case *ast.PRecord:
		ctor := b.c.ctors[p.Ctor]
		if ctor == nil {
			fail(p.Pos, "unknown constructor %s", p.Ctor)
		}
		if ctor.FieldNames == nil {
			fail(p.Pos, "%s is not a record constructor", p.Ctor)
		}
		fields, ret, _ := b.c.instantiateCtor(ctor)
		unify(p.Pos, expected, ret)
		indexes := recordIndexes(p.Pos, ctor, p.Names, !p.Rest)
		for i, pat := range p.Pats {
			b.pattern(pat, fields[indexes[i]], env)
		}
		b.c.program.RecordPats[p] = RecordPat{Ctor: ctor, Indexes: indexes}
	case *ast.PCtor:
		ctor := b.c.ctors[p.Name]
		if ctor == nil {
			fail(p.Pos, "unknown constructor %s", p.Name)
		}
		fields, ret, _ := b.c.instantiateCtor(ctor)
		unify(p.Pos, expected, ret)
		if len(fields) != len(p.Args) {
			fail(p.Pos, "arity mismatch for pattern %s: want %d arguments, got %d", p.Name, len(fields), len(p.Args))
		}
		for i, arg := range p.Args {
			b.pattern(arg, fields[i], env)
		}
	}
}
