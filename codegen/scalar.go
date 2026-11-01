package codegen

import (
	"fmt"
	"strings"

	"go4t/ast"
	"go4t/types"
)

// Only checked scalar types change representation. Data, functions and type
// variables keep the erased ABI, including dictionary and foreign boundaries.
func scalar(t types.Type) string {
	if c, ok := types.Unalias(t).(*types.TCon); ok {
		switch c.Name {
		case "Int":
			return "int64"
		case "Bool":
			return "bool"
		case "Float":
			return "float64"
		}
	}
	return "any"
}

func convert(code, from, to string) string {
	if to == "" || to == "any" || to == from {
		return code
	}
	return "any(" + code + ").(" + to + ")"
}

func (g *generator) rep(t types.Type) string { return scalar(g.substitute(t)) }
func (g *generator) exprRep(e ast.Expr) string {
	if a, ok := (*e.AnnSlot()).(*types.Ann); ok {
		return g.rep(a.Type)
	}
	return "any"
}

func (g *generator) substitute(t types.Type) types.Type {
	return substitute(t, g.typeArgs)
}

func substitute(t types.Type, args map[string]types.Type) types.Type {
	t = types.Unalias(t)
	switch t := t.(type) {
	case *types.TVar:
		if actual := args[t.Name]; actual != nil {
			return actual
		}
	case *types.TApp:
		values := make([]types.Type, len(t.Args))
		for i, value := range t.Args {
			values[i] = substitute(value, args)
		}
		return types.App(substitute(t.Fun, args), values...)
	case *types.TFunc:
		params := make([]types.Type, len(t.Params))
		for i, p := range t.Params {
			params[i] = substitute(p, args)
		}
		return &types.TFunc{Params: params, Ret: substitute(t.Ret, args)}
	}
	return t
}

func (g *generator) localScope() func() {
	old := g.locals
	g.locals = make(map[string]string, len(old))
	for name, rep := range old {
		g.locals[name] = rep
	}
	return func() { g.locals = old }
}

func (g *generator) bindPattern(p ast.Pattern, t types.Type) {
	if p == nil {
		return
	}
	if g.patternTypes == nil {
		g.patternTypes = make(map[ast.Pattern]types.Type)
	}
	t = g.substitute(t)
	g.patternTypes[p] = t
	switch p := p.(type) {
	case *ast.PVar:
		g.locals[p.Name] = scalar(t)
	case *ast.PAs:
		g.locals[p.Name] = scalar(t)
		g.bindPattern(p.Pat, t)
	case *ast.PTuple:
		for i, elem := range p.Elems {
			var field types.Type
			if app, ok := t.(*types.TApp); ok && i < len(app.Args) {
				field = app.Args[i]
			}
			g.bindPattern(elem, field)
		}
	case *ast.PCtor:
		g.bindFields(p.Args, g.ctors[p.Name], t)
	case *ast.PRecord:
		rec := g.recordPats[p]
		fields := g.fieldTypes(rec.Ctor, t)
		for i, pat := range p.Pats {
			g.bindPattern(pat, fields[rec.Indexes[i]])
		}
	}
}

func (g *generator) fieldTypes(c *types.Ctor, t types.Type) []types.Type {
	if c == nil {
		return nil
	}
	args := make(map[string]types.Type)
	if app, ok := types.Unalias(t).(*types.TApp); ok {
		for i, p := range c.Data.TParams {
			if i < len(app.Args) {
				args[p.Name] = app.Args[i]
			}
		}
	}
	fields := make([]types.Type, len(c.Fields))
	for i, f := range c.Fields {
		fields[i] = substitute(f, args)
	}
	return fields
}
func (g *generator) bindFields(ps []ast.Pattern, c *types.Ctor, t types.Type) {
	fields := g.fieldTypes(c, t)
	for i, p := range ps {
		if i < len(fields) {
			g.bindPattern(p, fields[i])
		}
	}
}
func (g *generator) bindLet(name string, p ast.Pattern, value ast.Expr) {
	if p != nil {
		g.bindPattern(p, g.ann(value).Type)
	} else {
		g.locals[name] = g.exprRep(value)
	}
}

func (g *generator) returning(rep string, body func() string) string {
	old := g.returnRep
	g.returnRep = rep
	result := body()
	g.returnRep = old
	return result
}

func (g *generator) functionRep(f *types.FuncInfo) *types.TFunc {
	if f.Decl.Extern != nil {
		return nil
	}
	return g.substitute(f.Scheme.Type).(*types.TFunc)
}

// Function values adapt to the erased higher-order ABI; direct calls stay native.
func (g *generator) functionValue(id *ast.Ident) string {
	f := g.funcs[id.Name]
	needsAdapter := len(g.ann(id).Dicts) > 0
	var target *types.TFunc
	if f != nil && f.Decl.Extern == nil {
		target = f.Scheme.Type
		if spec := g.specialized(id); spec != nil {
			target = substitute(target, spec.types).(*types.TFunc)
		}
		needsAdapter = needsAdapter || scalar(target.Ret) != "any"
		for _, p := range target.Params {
			needsAdapter = needsAdapter || scalar(p) != "any"
		}
	}
	if !needsAdapter {
		return mangle(id.Name)
	}
	ft := g.funcType(id)
	args := anonymousArgs(len(ft.Params))
	if target != nil {
		for i, t := range target.Params {
			args[i] = convert(args[i], "any", scalar(t))
		}
	}
	return "func(" + strings.Join(anonymousParams(len(ft.Params)), ", ") + ") any { return " + g.identCall(id, args) + " }"
}

// Literal arithmetic must execute at runtime: Go constant folding would reject
// overflow/division by zero and would lose Float negative zero and IEEE rounding.
func literalExpr(e ast.Expr) bool {
	switch e.(type) {
	case *ast.IntLit, *ast.FloatLit, *ast.BoolLit:
		return true
	}
	return false
}
func (g *generator) nativeBinary(e *ast.Binary, left, right string) string {
	if !(literalExpr(e.L) && literalExpr(e.R)) && !(literalExpr(e.R) && (e.Op == "/" || e.Op == "%")) {
		return "(" + left + " " + e.Op + " " + right + ")"
	}
	rep := g.exprRep(e.L)
	if rep == "any" {
		rep = g.exprRep(e)
	}
	ret := g.exprRep(e)
	if ret == "any" {
		ret = rep
	}
	return g.sourceExpr(e.GetPos(), "func(a, b "+rep+") "+ret+" { return a "+e.Op+" b }") + "(" + left + ", " + right + ")"
}

func (g *generator) nativeUnary(e *ast.Unary) string {
	value := g.expr(e.X)
	if literalExpr(e.X) {
		rep := g.exprRep(e.X)
		return g.sourceExpr(e.GetPos(), "func(a "+rep+") "+rep+" { return "+e.Op+"a }") + "(" + value + ")"
	}
	return "(" + e.Op + value + ")"
}

func (g *generator) condition(e ast.Expr) string {
	return convert(g.expr(e), g.exprRep(e), "bool")
}

// Ord[Float] is NaN-low, unlike Go's relational operators. Evaluate each
// operand once and preserve that total ordering using native scalar operations.
func (g *generator) nativeOrdering(e *ast.Binary, rep, left, right string) string {
	var comparison string
	if rep == "float64" {
		switch e.Op {
		case "<":
			comparison = "(a != a && b == b) || a < b"
		case "<=":
			comparison = "a != a || (b == b && a <= b)"
		case ">":
			comparison = "(b != b && a == a) || a > b"
		case ">=":
			comparison = "b != b || (a == a && a >= b)"
		}
	} else {
		switch e.Op {
		case "<":
			comparison = "!a && b"
		case "<=":
			comparison = "!a || b"
		case ">":
			comparison = "a && !b"
		case ">=":
			comparison = "a || !b"
		}
	}
	return g.sourceExpr(e.GetPos(), "func(a, b "+rep+") bool { return "+comparison+" }") + "(" + left + ", " + right + ")"
}

func (g *generator) emitFunction(f *types.FuncInfo, name string) string {
	restore := g.localScope()
	defer restore()
	g.fileName = g.sources[f.Decl]
	g.dictScope = g.newDictionaryScope(f.Scheme.Constraints, nil)
	g.collectType(f.Scheme.Type)
	params := g.dictScope.declarations()
	if g.special != nil {
		params = nil
	}
	var body ast.Expr
	var plan *tailPlan
	if f.Decl.Body != nil {
		body = &ast.BlockExpr{Block: f.Decl.Body}
		plan = g.planTail(f.Name, types.RefFunc, f.Decl.Params, f.Scheme.Constraints, body)
	}
	ft := g.functionRep(f)
	ret := "any"
	if ft != nil {
		ret = scalar(ft.Ret)
	}
	for i, p := range f.Decl.Params {
		rep := "any"
		if ft != nil {
			rep = scalar(ft.Params[i])
		}
		g.locals[p.Name] = rep
		paramName := mangle(p.Name)
		if plan != nil {
			paramName = plan.params[i]
			plan.reps = append(plan.reps, rep)
		}
		params = append(params, paramName+" "+rep)
	}
	code := g.returning(ret, func() string {
		if f.Decl.Extern != nil {
			return g.emitExtern(f)
		}
		if plan != nil {
			return g.loopBody(plan, f.Decl.Params, body)
		}
		return g.block(f.Decl.Body)
	})
	return g.line(f.Decl.Pos) + fmt.Sprintf("func %s(%s) %s {\n%s}\n", name, strings.Join(params, ", "), ret, code)
}
