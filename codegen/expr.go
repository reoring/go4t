package codegen

import (
	"fmt"
	"strconv"
	"strings"

	"gofx/ast"
	"gofx/types"
)

func (g *generator) block(block *ast.Block) string {
	if block == nil || block.Result == nil {
		g.fail(ast.Pos{}, "block has no result")
	}
	return g.statements(block.Stmts, block.Result)
}

func (g *generator) statements(stmts []ast.Stmt, result ast.Expr) string {
	var out strings.Builder
	for i, stmt := range stmts {
		switch stmt := stmt.(type) {
		case *ast.LetStmt:
			// A nested scope preserves lexical shadowing, including the case where
			// the initializer refers to the previous binding of the same name.
			fmt.Fprintf(&out, "{\nvar %s any = %s\n_ = %s\n%s}\n", mangle(stmt.Name), g.expr(stmt.Value), mangle(stmt.Name), g.statements(stmts[i+1:], result))
			return out.String()
		case *ast.ExprStmt:
			fmt.Fprintf(&out, "_ = %s\n", g.expr(stmt.X))
		default:
			g.fail(ast.Pos{}, "unsupported statement %T", stmt)
		}
	}
	fmt.Fprintf(&out, "return %s\n", g.expr(result))
	return out.String()
}

func (g *generator) expr(expr ast.Expr) string {
	switch e := expr.(type) {
	case *ast.IntLit:
		return "int64(" + strconv.FormatInt(e.Value, 10) + ")"
	case *ast.StringLit:
		return strconv.Quote(e.Value)
	case *ast.BoolLit:
		return strconv.FormatBool(e.Value)
	case *ast.Ident:
		ann := g.ann(e)
		switch ann.Ref {
		case types.RefLocal:
			return mangle(e.Name)
		case types.RefFunc:
			if len(ann.Dicts) == 0 {
				return mangle(e.Name)
			}
		case types.RefMethod:
			if len(ann.Dicts) == 1 {
				return "(" + g.dict(&ann.Dicts[0]) + ")." + exported(e.Name)
			}
		case types.RefCtor:
			ctor := g.ctor(e.Name, e.GetPos())
			if len(ctor.Fields) == 0 {
				return g.construct(ctor, nil)
			}
			return "func(" + strings.Join(anonymousParams(len(ctor.Fields)), ", ") + ") any { return " + g.construct(ctor, anonymousArgs(len(ctor.Fields))) + " }"
		default:
			g.fail(e.GetPos(), "invalid reference annotation")
		}
		// Taking a constrained function or method as a value binds the hidden
		// dictionaries now, retaining only its source-language arity.
		ft := g.funcType(e)
		return "func(" + strings.Join(anonymousParams(len(ft.Params)), ", ") + ") any { return " + g.identCall(e, anonymousArgs(len(ft.Params))) + " }"
	case *ast.Call:
		args := make([]string, len(e.Args))
		for i, arg := range e.Args {
			args[i] = g.expr(arg)
		}
		if ident, ok := e.Fn.(*ast.Ident); ok && g.ann(ident).Ref != types.RefLocal {
			return g.identCall(ident, args)
		}
		ft := g.funcType(e.Fn)
		params := make([]string, len(ft.Params))
		for i := range params {
			params[i] = "any"
		}
		return "any(" + g.expr(e.Fn) + ").(func(" + strings.Join(params, ", ") + ") any)(" + strings.Join(args, ", ") + ")"
	case *ast.Lambda:
		params := make([]string, len(e.Params))
		for i, param := range e.Params {
			params[i] = mangle(param.Name) + " any"
		}
		return "func(" + strings.Join(params, ", ") + ") any {\nreturn " + g.expr(e.Body) + "\n}"
	case *ast.BlockExpr:
		return "func() any {\n" + g.block(e.Block) + "}()"
	case *ast.If:
		return "func() any {\nif any(" + g.expr(e.Cond) + ").(bool) {\nreturn " + g.expr(e.Then) + "\n}\nreturn " + g.expr(e.Else) + "\n}()"
	case *ast.Match:
		var out strings.Builder
		value := g.fresh()
		fmt.Fprintf(&out, "func() any {\nvar %s any = %s\n_ = %s\n", value, g.expr(e.Scrut), value)
		for _, arm := range e.Arms {
			out.WriteString(g.pattern(arm.Pat, value, "return "+g.expr(arm.Body)+"\n"))
		}
		out.WriteString("panic(\"non-exhaustive match\")\n}()")
		return out.String()
	case *ast.Do:
		dict := g.dict(g.ann(e).Monad)
		body := "return " + g.expr(e.Result) + "\n"
		for i := len(e.Stmts) - 1; i >= 0; i-- {
			var value ast.Expr
			var name string
			switch stmt := e.Stmts[i].(type) {
			case *ast.DoBind:
				value, name = stmt.X, mangle(stmt.Name)
			case *ast.DoExpr:
				value, name = stmt.X, "_"
			case *ast.DoLet:
				body = "{\nvar " + mangle(stmt.Name) + " any = " + g.expr(stmt.Value) + "\n_ = " + mangle(stmt.Name) + "\n" + body + "}\n"
				continue
			default:
				g.fail(e.GetPos(), "unsupported do statement %T", stmt)
			}
			body = "return (" + dict + ").Bind(" + g.expr(value) + ", func(" + name + " any) any {\n" + body + "})\n"
		}
		return "func() any {\n" + body + "}()"
	case *ast.Binary:
		return "rt." + g.binaryOp(e) + "(" + g.expr(e.L) + ", " + g.expr(e.R) + ")"
	case *ast.Unary:
		var op string
		typ := g.ann(e.X).Type
		if e.Op == "-" && isCon(typ, "Int") {
			op = "NegInt"
		} else if e.Op == "!" && isCon(typ, "Bool") {
			op = "Not"
		} else {
			g.fail(e.GetPos(), "unsupported unary operator %s on %s", e.Op, types.TypeString(typ))
		}
		return "rt." + op + "(" + g.expr(e.X) + ")"
	}
	g.fail(expr.GetPos(), "unsupported expression %T", expr)
	return ""
}

func (g *generator) funcType(expr ast.Expr) *types.TFunc {
	ft, ok := g.ann(expr).Type.(*types.TFunc)
	if !ok {
		g.fail(expr.GetPos(), "function reference has no function type")
	}
	return ft
}

func (g *generator) identCall(ident *ast.Ident, args []string) string {
	ann := g.ann(ident)
	callee := mangle(ident.Name)
	dicts := ann.Dicts
	switch ann.Ref {
	case types.RefCtor:
		return g.construct(g.ctor(ident.Name, ident.GetPos()), args)
	case types.RefMethod:
		if len(dicts) == 0 {
			g.fail(ident.GetPos(), "method reference has no dictionary")
		}
		callee = "(" + g.dict(&dicts[0]) + ")." + exported(ident.Name)
		dicts = dicts[1:]
	case types.RefFunc:
	default:
		g.fail(ident.GetPos(), "invalid direct call annotation")
	}
	allArgs := make([]string, 0, len(dicts)+len(args))
	for i := range dicts {
		allArgs = append(allArgs, g.dict(&dicts[i]))
	}
	allArgs = append(allArgs, args...)
	return callee + "(" + strings.Join(allArgs, ", ") + ")"
}

func (g *generator) ctor(name string, pos ast.Pos) *types.Ctor {
	ctor := g.ctors[name]
	if ctor == nil {
		g.fail(pos, "unknown constructor %s", name)
	}
	return ctor
}

func (g *generator) construct(ctor *types.Ctor, args []string) string {
	fields := make([]string, len(args))
	for i, arg := range args {
		fields[i] = fmt.Sprintf("F%d: %s", i, arg)
	}
	return "&" + ctorName(ctor) + "{" + strings.Join(fields, ", ") + "}"
}

// Each arm has its own failure continuation: a failed nested pattern falls
// through to the next complete arm, including arms with the same outer ctor.
func (g *generator) pattern(pattern ast.Pattern, value, success string) string {
	switch p := pattern.(type) {
	case *ast.PWild:
		return "{\n" + success + "}\n"
	case *ast.PVar:
		return "{\nvar " + mangle(p.Name) + " any = " + value + "\n_ = " + mangle(p.Name) + "\n" + success + "}\n"
	case *ast.PCtor:
		ctor := g.ctor(p.Name, p.Pos)
		matched := g.fresh()
		for i := len(p.Args) - 1; i >= 0; i-- {
			success = g.pattern(p.Args[i], fmt.Sprintf("%s.F%d", matched, i), success)
		}
		return "switch " + matched + " := " + value + ".(type) {\ncase *" + ctorName(ctor) + ":\n_ = " + matched + "\n" + success + "}\n"
	case *ast.PLit:
		var rep string
		switch p.X.(type) {
		case *ast.IntLit:
			rep = "int64"
		case *ast.StringLit:
			rep = "string"
		case *ast.BoolLit:
			rep = "bool"
		default:
			g.fail(p.Pos, "unsupported literal pattern %T", p.X)
		}
		return "if " + value + ".(" + rep + ") == " + g.expr(p.X) + " {\n" + success + "}\n"
	}
	g.fail(ast.Pos{}, "unsupported pattern %T", pattern)
	return ""
}

func isCon(typ types.Type, name string) bool {
	con, ok := typ.(*types.TCon)
	return ok && con.Name == name
}

func (g *generator) binaryOp(expr *ast.Binary) string {
	var typ types.Type
	if expr.Op == "+" {
		typ = g.ann(expr).Type
	} else {
		typ = g.ann(expr.L).Type
	}
	var prefix string
	switch expr.Op {
	case "+":
		if isCon(typ, "Int") {
			return "AddInt"
		}
		if isCon(typ, "String") {
			return "AddString"
		}
	case "-", "*", "/":
		if isCon(typ, "Int") {
			switch expr.Op {
			case "-":
				return "SubInt"
			case "*":
				return "MulInt"
			case "/":
				return "DivInt"
			}
		}
	case "&&":
		if isCon(typ, "Bool") {
			return "And"
		}
	case "||":
		if isCon(typ, "Bool") {
			return "Or"
		}
	case "==":
		prefix = "Eq"
	case "!=":
		prefix = "Ne"
	case "<":
		prefix = "Lt"
	case "<=":
		prefix = "Le"
	case ">":
		prefix = "Gt"
	case ">=":
		prefix = "Ge"
	}
	if prefix != "" {
		if isCon(typ, "Int") {
			return prefix + "Int"
		}
		if isCon(typ, "String") {
			return prefix + "String"
		}
		if isCon(typ, "Bool") && (expr.Op == "==" || expr.Op == "!=") {
			return prefix + "Bool"
		}
	}
	g.fail(expr.GetPos(), "unsupported binary operator %s on %s", expr.Op, types.TypeString(typ))
	return ""
}
