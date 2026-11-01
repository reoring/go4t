package types

import "gofx/ast"

type locals struct {
	parent *locals
	values map[string]Type
}

func newLocals(parent *locals) *locals { return &locals{parent: parent, values: make(map[string]Type)} }

func (l *locals) lookup(name string) Type {
	for ; l != nil; l = l.parent {
		if t := l.values[name]; t != nil {
			return t
		}
	}
	return nil
}

type obligation struct {
	expr        ast.Expr
	constraints []Constraint
	monad       bool
}

type bodyChecker struct {
	c           *checker
	constraints []Constraint
	types       map[string]Type
	exprs       []ast.Expr
	obligations []obligation
	additions   []*ast.Binary
	comparisons []*ast.Binary
	matches     []*ast.Match
}

func (c *checker) body(constraints []Constraint, scope map[string]Type) *bodyChecker {
	return &bodyChecker{c: c, constraints: constraints, types: scope}
}

func (b *bodyChecker) check(params []ast.Param, signature *TFunc, block *ast.Block) {
	env := newLocals(nil)
	for i, param := range params {
		if env.values[param.Name] != nil {
			fail(block.Pos, "duplicate parameter %s", param.Name)
		}
		env.values[param.Name] = signature.Params[i]
	}
	result := b.block(block, env)
	unify(block.Result.GetPos(), signature.Ret, result)
	b.finish()
}

func (b *bodyChecker) block(block *ast.Block, outer *locals) Type {
	env := newLocals(outer)
	for _, stmt := range block.Stmts {
		switch stmt := stmt.(type) {
		case *ast.LetStmt:
			checkName(stmt.Pos, stmt.Name)
			env.values[stmt.Name] = b.infer(stmt.Value, env)
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
	if t := env.lookup(e.Name); t != nil {
		return t
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
	if ctor := b.c.ctors[e.Name]; ctor != nil {
		fields, ret, inst := b.c.instantiateCtor(ctor)
		ann.Ref, ann.Inst = RefCtor, inst
		if len(fields) == 0 {
			return ret
		}
		return &TFunc{Params: fields, Ret: ret}
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
	var ret Type = c.cons[ctor.Data.Name]
	if len(inst) != 0 {
		ret = &TApp{Fun: ret, Args: inst}
	}
	return fields, ret, inst
}

func (b *bodyChecker) infer(e ast.Expr, env *locals) Type {
	ann := &Ann{}
	*e.AnnSlot() = ann
	b.exprs = append(b.exprs, e)
	pos := e.GetPos()
	var result Type
	switch e := e.(type) {
	case *ast.IntLit:
		result = Int
	case *ast.StringLit:
		result = String
	case *ast.BoolLit:
		result = Bool
	case *ast.Ident:
		result = b.ident(e, env, ann)
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
		result = &TApp{Fun: b.c.cons[data.Name], Args: []Type{item}}
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
			unify(arg.GetPos(), ft.Params[i], b.infer(arg, env))
		}
		result = ft.Ret
	case *ast.Lambda:
		local := newLocals(env)
		ft := &TFunc{Params: make([]Type, len(e.Params))}
		for i, param := range e.Params {
			checkName(pos, param.Name)
			if local.values[param.Name] != nil {
				fail(pos, "duplicate parameter %s", param.Name)
			}
			var typ Type
			if param.Type == nil {
				typ = b.c.fresh(KStar{})
			} else {
				typ = b.c.valueType(param.Type, b.types)
			}
			ft.Params[i], local.values[param.Name] = typ, typ
		}
		ft.Ret = b.infer(e.Body, local)
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
			unify(arm.Body.GetPos(), result, b.infer(arm.Body, local))
		}
		b.matches = append(b.matches, e)
	case *ast.Do:
		local := newLocals(env)
		head := b.c.fresh(arityKind(1))
		b.obligations = append(b.obligations, obligation{expr: e, constraints: []Constraint{{Class: "Monad", Arg: head}}, monad: true})
		for _, stmt := range e.Stmts {
			switch stmt := stmt.(type) {
			case *ast.DoBind:
				checkName(stmt.Pos, stmt.Name)
				local.values[stmt.Name] = b.monadic(stmt.X, head, local)
			case *ast.DoLet:
				checkName(stmt.Pos, stmt.Name)
				local.values[stmt.Name] = b.infer(stmt.Value, local)
			case *ast.DoExpr:
				b.monadic(stmt.X, head, local)
			}
		}
		item := b.monadic(e.Result, head, local)
		result = &TApp{Fun: head, Args: []Type{item}}
	case *ast.Unary:
		arg := b.infer(e.X, env)
		switch e.Op {
		case "-":
			result = Int
		case "!":
			result = Bool
		default:
			fail(pos, "unknown unary operator %s", e.Op)
		}
		unify(e.X.GetPos(), result, arg)
	case *ast.Binary:
		left, right := b.infer(e.L, env), b.infer(e.R, env)
		switch e.Op {
		case "+":
			unify(e.R.GetPos(), left, right)
			b.additions = append(b.additions, e)
			result = left
		case "-", "*", "/":
			unify(e.L.GetPos(), Int, left)
			unify(e.R.GetPos(), Int, right)
			result = Int
		case "&&", "||":
			unify(e.L.GetPos(), Bool, left)
			unify(e.R.GetPos(), Bool, right)
			result = Bool
		case "==", "!=", "<", "<=", ">", ">=":
			unify(e.R.GetPos(), left, right)
			b.comparisons = append(b.comparisons, e)
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
	unify(e.GetPos(), &TApp{Fun: head, Args: []Type{item}}, b.infer(e, env))
	return item
}

func (b *bodyChecker) pattern(p ast.Pattern, expected Type, env *locals) {
	switch p := p.(type) {
	case *ast.PWild:
	case *ast.PVar:
		checkName(p.Pos, p.Name)
		if env.values[p.Name] != nil {
			fail(p.Pos, "duplicate pattern variable %s", p.Name)
		}
		env.values[p.Name] = expected
	case *ast.PLit:
		unify(p.Pos, expected, b.infer(p.X, env))
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
