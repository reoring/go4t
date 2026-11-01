package codegen

import (
	"fmt"
	"strings"

	"gofx/ast"
	"gofx/types"
)

// A plan contains only direct self calls in result position. Calls through
// aliases, inside arguments, or behind a newly shadowing binding stay calls.
// Dictionary-changing polymorphic recursion also stays a call: loop iterations
// share the original dictionaries.
type tailPlan struct {
	params []string
	calls  map[*ast.Call]bool
}

func (g *generator) planTail(name string, ref types.RefKind, params []ast.Param, constraints []types.Constraint, body ast.Expr) *tailPlan {
	plan := &tailPlan{calls: make(map[*ast.Call]bool)}
	var visit func(ast.Expr)
	visit = func(expr ast.Expr) {
		switch e := expr.(type) {
		case *ast.Call:
			id, ok := e.Fn.(*ast.Ident)
			if !ok || id.Name != name || len(e.Args) != len(params) {
				return
			}
			ann := g.ann(id)
			if ann.Ref != ref || len(ann.Dicts) != len(constraints) {
				return
			}
			for i, constraint := range constraints {
				if g.dict(&ann.Dicts[i]) != g.dictField(constraint) {
					return
				}
			}
			plan.calls[e] = true
		case *ast.If:
			visit(e.Then)
			visit(e.Else)
		case *ast.Match:
			for _, arm := range e.Arms {
				if ref != types.RefLocal || !patternBinds(arm.Pat, name) {
					visit(arm.Body)
				}
			}
		case *ast.BlockExpr:
			if ref == types.RefLocal {
				for _, stmt := range e.Block.Stmts {
					if let, ok := stmt.(*ast.LetStmt); ok && (let.Name == name || patternBinds(let.Pat, name)) {
						return
					}
				}
			}
			visit(e.Block.Result)
		}
	}
	for _, param := range params {
		if ref == types.RefLocal && (param.Name == name || patternBinds(param.Pat, name)) {
			return nil
		}
	}
	visit(body)
	if len(plan.calls) == 0 {
		return nil
	}
	plan.params = make([]string, len(params))
	for i := range plan.params {
		plan.params[i] = g.fresh()
	}
	return plan
}

func patternBinds(pattern ast.Pattern, name string) bool {
	switch p := pattern.(type) {
	case *ast.PVar:
		return p.Name == name
	case *ast.PAs:
		return p.Name == name || patternBinds(p.Pat, name)
	case *ast.PTuple:
		for _, elem := range p.Elems {
			if patternBinds(elem, name) {
				return true
			}
		}
	case *ast.PCtor:
		for _, arg := range p.Args {
			if patternBinds(arg, name) {
				return true
			}
		}
	case *ast.PRecord:
		for _, field := range p.Pats {
			if patternBinds(field, name) {
				return true
			}
		}
	}
	return false
}

func (g *generator) loopBody(plan *tailPlan, params []ast.Param, result ast.Expr) string {
	body := g.result(result, plan)
	for i := len(params) - 1; i >= 0; i-- {
		pattern := params[i].Pat
		if pattern == nil {
			pattern = &ast.PVar{Name: params[i].Name}
		}
		// Fresh lexical bindings on every iteration are essential: a closure
		// passed to the next iteration must retain the old parameter values.
		body, _ = g.pattern(pattern, plan.params[i], body, true)
	}
	return "for {\n" + body + "}\n"
}

// namedLambda is used only for bindings known to be recursive. An ordinary let
// initializer can refer to an outer binding of the same name, not to itself.
func (g *generator) namedLambda(name string, lambda *ast.Lambda) string {
	plan := g.planTail(name, types.RefLocal, lambda.Params, nil, lambda.Body)
	if plan == nil {
		return g.expr(lambda)
	}
	params := make([]string, len(plan.params))
	for i, param := range plan.params {
		params[i] = param + " any"
	}
	return g.sourceExpr(lambda.GetPos(), "func("+strings.Join(params, ", ")+") any {\n"+g.loopBody(plan, lambda.Params, lambda.Body)+"}")
}

func (g *generator) result(expr ast.Expr, plan *tailPlan) string {
	g.collectAnnotation(expr)
	if plan != nil {
		switch e := expr.(type) {
		case *ast.Call:
			if plan.calls[e] {
				var out strings.Builder
				out.WriteString(g.line(e.GetPos()))
				args := make([]string, len(e.Args))
				for i, arg := range e.Args {
					args[i] = g.fresh()
					fmt.Fprintf(&out, "var %s any = %s\n", args[i], g.expr(arg))
				}
				if len(args) != 0 {
					fmt.Fprintf(&out, "%s = %s\n", strings.Join(plan.params, ", "), strings.Join(args, ", "))
				}
				out.WriteString("continue\n")
				return out.String()
			}
		case *ast.If:
			return g.line(e.GetPos()) + "if any(" + g.expr(e.Cond) + ").(bool) {\n" + g.result(e.Then, plan) + "} else {\n" + g.result(e.Else, plan) + "}\n"
		case *ast.Match:
			return g.matchResult(e, plan)
		case *ast.BlockExpr:
			return g.tailStatements(e.Block.Stmts, e.Block.Result, plan)
		}
	}
	return g.line(expr.GetPos()) + "return " + g.expr(expr) + "\n"
}

func (g *generator) matchResult(match *ast.Match, plan *tailPlan) string {
	var out strings.Builder
	value := g.fresh()
	fmt.Fprintf(&out, "var %s any = %s\n_ = %s\n", value, g.expr(match.Scrut), value)
	for i, arm := range match.Arms {
		body := g.line(arm.Pos) + g.result(arm.Body, plan)
		if arm.Guard != nil {
			body = g.line(arm.Pos) + "if any(" + g.expr(arm.Guard) + ").(bool) {\n" + body + "}\n"
		}
		code, guarded := g.pattern(arm.Pat, value, body, i == len(match.Arms)-1 && arm.Guard == nil)
		out.WriteString(g.line(arm.Pos))
		out.WriteString(code)
		if i == len(match.Arms)-1 && (guarded || arm.Guard != nil) {
			out.WriteString("panic(\"non-exhaustive match\")\n")
		}
	}
	return out.String()
}
