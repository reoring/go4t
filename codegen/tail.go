package codegen

import (
	"fmt"
	"strings"

	"github.com/reoring/go4t/ast"
	"github.com/reoring/go4t/types"
)

// A plan contains only direct self calls in result position. Calls through
// aliases, inside arguments, or behind a newly shadowing binding stay calls.
// Dictionary-changing polymorphic recursion also stays a call: loop iterations
// share the original dictionaries.
type tailPlan struct {
	params []string
	calls  map[*ast.Call]bool
	reps   []string
	io     bool // Results execute in an IO closure rather than returning an action.
}

func (g *generator) planTail(name string, ref types.RefKind, params []ast.Param, constraints []types.Constraint, body ast.Expr) *tailPlan {
	plan := &tailPlan{}
	if ref == types.RefFunc {
		if f := g.funcs[name]; f != nil {
			if app, ok := types.Unalias(g.substitute(f.Scheme.Type.Ret)).(*types.TApp); ok {
				plan.io = types.Unalias(app.Fun) == types.IO
			}
		}
	}
	plan.calls = types.TailCalls(name, ref, params, body, func(dicts []types.DictRef) bool {
		if len(dicts) != len(constraints) {
			return false
		}
		for i, constraint := range constraints {
			param, ok := constraint.Arg.(*types.TVar)
			if !ok {
				return false
			}
			expected := g.dict(&types.DictRef{Kind: types.DictParam, Param: param, ParamClass: constraint.Class})
			if g.dict(&dicts[i]) != expected {
				return false
			}
		}
		return true
	}, func(e *ast.Do) bool { return plan.io && g.directIO(e) })
	if len(plan.calls) == 0 {
		return nil
	}
	plan.params = make([]string, len(params))
	for i := range plan.params {
		plan.params[i] = g.fresh()
	}
	return plan
}

func (g *generator) loopBody(plan *tailPlan, params []ast.Param, result ast.Expr) string {
	restore := g.localScope()
	defer restore()
	patterns := make([]ast.Pattern, len(params))
	for i, param := range params {
		patterns[i] = param.Pat
		if patterns[i] == nil {
			patterns[i] = &ast.PVar{Name: param.Name}
		}
		t := g.parameterType(param)
		if len(plan.reps) > i && plan.reps[i] != "any" {
			switch plan.reps[i] {
			case "int64":
				t = types.Int
			case "bool":
				t = types.Bool
			case "float64":
				t = types.Float
			}
		}
		g.bindPattern(patterns[i], t)
	}
	body := g.result(result, plan)
	for i := len(params) - 1; i >= 0; i-- {
		// Fresh lexical bindings keep captured values stable across iterations.
		rep := "any"
		if len(plan.reps) > i {
			rep = plan.reps[i]
		}
		body, _ = g.patternAs(patterns[i], plan.params[i], rep, body, true)
	}
	if plan.io {
		body = "if err := ctx.Err(); err != nil { return nil, rt.ClassifyError(err) }\n" + body
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
	code := g.lambdaBody(lambda, plan)
	return g.sourceExpr(lambda.GetPos(), code)
}

func (g *generator) result(expr ast.Expr, plan *tailPlan) string {
	g.collectAnnotation(expr)
	switch e := expr.(type) {
	case *ast.Match:
		return g.matchResult(e, plan)
	case *ast.If:
		return g.line(e.GetPos()) + "if " + g.condition(e.Cond) + " {\n" + g.result(e.Then, plan) + "} else {\n" + g.result(e.Else, plan) + "}\n"
	case *ast.BlockExpr:
		return g.tailStatements(e.Block.Stmts, e.Block.Result, plan)
	case *ast.Do:
		if plan != nil && plan.io && g.directIO(e) {
			return g.doResult(e, 0, plan)
		}
	}
	if plan != nil {
		switch e := expr.(type) {
		case *ast.Call:
			if plan.calls[e] {
				var out strings.Builder
				out.WriteString(g.line(e.GetPos()))
				args := make([]string, len(e.Args))
				for i, arg := range e.Args {
					args[i] = g.fresh()
					rep := "any"
					if len(plan.reps) > i {
						rep = plan.reps[i]
					}
					fmt.Fprintf(&out, "var %s %s = %s\n", args[i], rep, convert(g.expr(arg), g.exprRep(arg), rep))
				}
				if len(args) != 0 {
					fmt.Fprintf(&out, "%s = %s\n", strings.Join(plan.params, ", "), strings.Join(args, ", "))
				}
				out.WriteString("continue\n")
				return out.String()
			}
		}
	}
	if plan != nil && plan.io {
		return g.line(expr.GetPos()) + "return rt.Run(ctx, " + g.expr(expr) + ")\n"
	}
	return g.line(expr.GetPos()) + g.returnValue(expr)
}

func (g *generator) matchResult(match *ast.Match, plan *tailPlan) string {
	var out strings.Builder
	value := g.fresh()
	rep := g.exprRep(match.Scrut)
	fmt.Fprintf(&out, "var %s %s = %s\n_ = %s\n", value, rep, g.expr(match.Scrut), value)
	for i, arm := range match.Arms {
		restore := g.localScope()
		g.bindPattern(arm.Pat, g.ann(match.Scrut).Type)
		body := g.line(arm.Pos) + g.result(arm.Body, plan)
		if arm.Guard != nil {
			body = g.line(arm.Pos) + "if " + g.condition(arm.Guard) + " {\n" + body + "}\n"
		}
		code, guarded := g.patternAs(arm.Pat, value, rep, body, i == len(match.Arms)-1 && arm.Guard == nil)
		restore()
		out.WriteString(g.line(arm.Pos))
		out.WriteString(code)
		if i == len(match.Arms)-1 && (guarded || arm.Guard != nil) {
			out.WriteString("panic(\"non-exhaustive match\")\n")
		}
	}
	return out.String()
}
