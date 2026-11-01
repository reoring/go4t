package codegen

import (
	"fmt"
	"strings"

	"gofx/ast"
	"gofx/types"
)

func (g *generator) lambda(lambda *ast.Lambda) string {
	return g.lambdaBody(lambda, nil)
}

func (g *generator) lambdaBody(lambda *ast.Lambda, plan *tailPlan) string {
	restore := g.localScope()
	defer restore()
	params := make([]string, len(lambda.Params))
	patterns := make([]ast.Pattern, len(lambda.Params))
	reps := make([]string, len(lambda.Params))
	var ft *types.TFunc
	if a, ok := (*lambda.AnnSlot()).(*types.Ann); ok {
		ft, _ = types.Unalias(a.Type).(*types.TFunc)
	}
	ret := g.exprRep(lambda.Body)
	if block, ok := lambda.Body.(*ast.BlockExpr); ok {
		ret = g.exprRep(block.Block.Result)
	}
	if ft != nil {
		ret = g.rep(ft.Ret)
	}
	native := ret != "any"
	for i, p := range lambda.Params {
		patterns[i] = p.Pat
		if patterns[i] == nil {
			patterns[i] = &ast.PVar{Name: p.Name}
		}
		typ := g.parameterType(p)
		if ft != nil {
			typ = ft.Params[i]
		}
		g.bindPattern(patterns[i], typ)
		reps[i] = g.rep(typ)
		native = native || reps[i] != "any"
		name := fmt.Sprintf("p%d", i)
		if plan != nil {
			name = plan.params[i]
		}
		params[i] = name + " " + reps[i]
	}
	body := g.returning(ret, func() string {
		if plan != nil {
			plan.reps = reps
			return g.loopBody(plan, lambda.Params, lambda.Body)
		}
		return g.result(lambda.Body, nil)
	})
	if plan == nil {
		for i := len(patterns) - 1; i >= 0; i-- {
			body, _ = g.patternAs(patterns[i], fmt.Sprintf("p%d", i), reps[i], body, true)
		}
	}
	code := "func(" + strings.Join(params, ", ") + ") " + ret + " {\n" + body + "}"
	if !native {
		return code
	}
	args := anonymousArgs(len(params))
	for i := range args {
		args[i] = convert(args[i], "any", reps[i])
	}
	return "func(" + strings.Join(anonymousParams(len(params)), ", ") + ") any { return " + g.sourceExpr(lambda.GetPos(), code) + "(" + strings.Join(args, ", ") + ") }"
}

func (g *generator) parameterType(p ast.Param) types.Type {
	return g.scalarAnnotation(p.Type, g.typeArgs)
}

// Local FuncDecl signatures are checked but not retained in Program.Funcs.
// Recognize only their scalar annotation leaves; checking has already expanded
// and validated these transparent aliases (including rejecting cycles).
func (g *generator) scalarAnnotation(t ast.Type, args map[string]types.Type) types.Type {
	n, ok := t.(*ast.TypeName)
	if !ok {
		return nil
	}
	if arg, found := args[n.Name]; found {
		return arg
	}
	switch n.Name {
	case "Int":
		return types.Int
	case "Bool":
		return types.Bool
	case "Float":
		return types.Float
	}
	if alias := g.aliases[n.Name]; alias != nil {
		bindings := make(map[string]types.Type, len(alias.TParams))
		for i, param := range alias.TParams {
			bindings[param] = g.scalarAnnotation(n.Args[i], args)
		}
		return g.scalarAnnotation(alias.Type, bindings)
	}
	return nil
}

func (g *generator) doStatements(e *ast.Do, index int) string {
	if index == len(e.Stmts) {
		return g.result(e.Result, nil)
	}
	stmt := e.Stmts[index]
	if f, ok := stmt.(*ast.FuncDecl); ok {
		group := []*ast.FuncDecl{f}
		next := index + 1
		for next < len(e.Stmts) {
			f, ok := e.Stmts[next].(*ast.FuncDecl)
			if !ok {
				break
			}
			group = append(group, f)
			next++
		}
		return g.localFunctions(group, g.doStatements(e, next))
	}
	restore := g.localScope()
	defer restore()
	var value ast.Expr
	var pattern ast.Pattern
	switch s := stmt.(type) {
	case *ast.DoLet:
		g.bindLet(s.Name, s.Pat, s.Value)
		body := g.doStatements(e, index+1)
		restore()
		return g.letStatement(s.Name, s.Pat, s.Value, body)
	case *ast.DoBind:
		value = s.X
		pattern = s.Pat
		if pattern == nil {
			pattern = &ast.PVar{Name: s.Name}
		}
		var typ types.Type
		if app, ok := types.Unalias(g.ann(value).Type).(*types.TApp); ok && len(app.Args) > 0 {
			typ = app.Args[len(app.Args)-1]
		}
		g.bindPattern(pattern, typ)
	case *ast.DoExpr:
		value = s.X
	default:
		g.fail(e.GetPos(), "unsupported do statement %T", stmt)
	}
	body := g.doStatements(e, index+1)
	name := "_"
	if pattern != nil {
		name = "p"
		if bind := stmt.(*ast.DoBind); bind.Pat == nil {
			name = mangle(bind.Name)
		}
		body, _ = g.pattern(pattern, name, body, true)
	}
	restore()
	continuation := g.sourceExpr(value.GetPos(), "func("+name+" any) any {\n"+body+"}")
	return "return (" + g.dict(g.ann(e).Monad) + ").Bind(" + g.expr(value) + ", " + continuation + ")\n"
}
