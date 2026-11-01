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
			fmt.Fprintf(&out, "{\n%s%s}\n", g.letBinding(stmt.Name, stmt.Value), g.statements(stmts[i+1:], result))
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

func (g *generator) letBinding(name string, value ast.Expr) string {
	name = mangle(name)
	if ann, ok := (*value.AnnSlot()).(*types.Ann); ok && ann != nil && ann.LetScheme != nil {
		scheme := ann.LetScheme
		previous := g.dictScope
		g.dictScope.params = append(g.dictScope.params, scheme.Constraints...)
		body := g.expr(value)
		g.dictScope = previous
		return name + " := func(" + strings.Join(g.dictParams(scheme.Constraints), ", ") + ") any {\nreturn " + body + "\n}\n_ = " + name + "\n"
	}
	return "var " + name + " any = " + g.expr(value) + "\n_ = " + name + "\n"
}

func (g *generator) expr(expr ast.Expr) string {
	g.collectAnnotation(expr)
	switch e := expr.(type) {
	case *ast.IntLit:
		return "int64(" + strconv.FormatInt(e.Value, 10) + ")"
	case *ast.StringLit:
		return strconv.Quote(e.Value)
	case *ast.Interp:
		ann := g.ann(e)
		parts := make([]string, 0, len(e.Texts)+len(e.Exprs))
		for i, text := range e.Texts {
			if text != "" {
				parts = append(parts, strconv.Quote(text))
			}
			if i < len(e.Exprs) {
				parts = append(parts, "("+g.dict(&ann.ShowDicts[i])+").Show("+g.expr(e.Exprs[i])+")")
			}
		}
		return "rt.Concat(" + strings.Join(parts, ", ") + ")"
	case *ast.Field:
		if e.Index >= 0 {
			tuple := g.ann(e.X).Type.(*types.TApp)
			return fmt.Sprintf("any(%s).(*%s).F%d", g.expr(e.X), g.tupleName(len(tuple.Args)), e.Index)
		}
		ann := g.ann(e)
		return fmt.Sprintf("any(%s).(*%s).F%d", g.expr(e.X), ctorName(ann.Ctor), ann.FieldIndex)
	case *ast.RecordLit:
		ann := g.ann(e)
		fields := make([]string, len(e.Values))
		for i, value := range e.Values {
			fields[i] = fmt.Sprintf("F%d: %s", ann.FieldIndexes[i], g.expr(value))
		}
		return "&" + ctorName(ann.Ctor) + "{" + strings.Join(fields, ", ") + "}"
	case *ast.RecordUpdate:
		ann := g.ann(e)
		base := g.fresh()
		var out strings.Builder
		fmt.Fprintf(&out, "func() any {\n%s := any(%s).(*%s)\n_ = %s\n", base, g.expr(e.X), ctorName(ann.Ctor), base)
		replaced := make([]bool, len(ann.Ctor.Fields))
		for _, index := range ann.FieldIndexes {
			replaced[index] = true
		}
		fields := make([]string, 0, len(ann.Ctor.Fields))
		for index, changed := range replaced {
			if !changed {
				fields = append(fields, fmt.Sprintf("F%d: %s.F%d", index, base, index))
			}
		}
		// Emit replacements in source order, not declaration order: their
		// expressions may call functions or fail.
		for i, value := range e.Values {
			fields = append(fields, fmt.Sprintf("F%d: %s", ann.FieldIndexes[i], g.expr(value)))
		}
		fmt.Fprintf(&out, "return &%s{%s}\n}()", ctorName(ann.Ctor), strings.Join(fields, ", "))
		return out.String()
	case *ast.BoolLit:
		return strconv.FormatBool(e.Value)
	case *ast.TupleLit:
		args := make([]string, len(e.Elems))
		for i, elem := range e.Elems {
			args[i] = g.expr(elem)
		}
		return construct(g.tupleName(len(e.Elems)), args)
	case *ast.ListLit:
		rest := construct(ctorName(g.ctor("Nil", e.GetPos())), nil)
		if len(e.Elems) == 0 {
			return rest
		}
		cons := g.ctor("Cons", e.GetPos())
		for i := len(e.Elems) - 1; i >= 0; i-- {
			rest = construct(ctorName(cons), []string{g.expr(e.Elems[i]), rest})
		}
		return rest
	case *ast.Ident:
		ann := g.ann(e)
		switch ann.Ref {
		case types.RefLocal:
			if len(ann.Dicts) == 0 {
				return mangle(e.Name)
			}
			dicts := make([]string, len(ann.Dicts))
			for i := range ann.Dicts {
				dicts[i] = g.dict(&ann.Dicts[i])
			}
			return mangle(e.Name) + "(" + strings.Join(dicts, ", ") + ")"
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
				return construct(ctorName(ctor), nil)
			}
			return "func(" + strings.Join(anonymousParams(len(ctor.Fields)), ", ") + ") any { return " + construct(ctorName(ctor), anonymousArgs(len(ctor.Fields))) + " }"
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
		for i, arm := range e.Arms {
			code, guarded := g.pattern(arm.Pat, value, "return "+g.expr(arm.Body)+"\n", i == len(e.Arms)-1)
			out.WriteString(code)
			if i == len(e.Arms)-1 && guarded {
				out.WriteString("panic(\"non-exhaustive match\")\n")
			}
		}
		out.WriteString("}()")
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
				body = "{\n" + g.letBinding(stmt.Name, stmt.Value) + body + "}\n"
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
	g.collectAnnotation(ident)
	ann := g.ann(ident)
	callee := mangle(ident.Name)
	dicts := ann.Dicts
	switch ann.Ref {
	case types.RefCtor:
		return construct(ctorName(g.ctor(ident.Name, ident.GetPos())), args)
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

func construct(name string, args []string) string {
	fields := make([]string, len(args))
	for i, arg := range args {
		fields[i] = fmt.Sprintf("F%d: %s", i, arg)
	}
	return "&" + name + "{" + strings.Join(fields, ", ") + "}"
}

// Each arm has its own failure continuation: a failed nested pattern falls
// through to the next complete arm, including arms with the same outer ctor.
// The final arm's head is guaranteed by exhaustiveness; nested heads are only
// unguarded when they are irrefutable on their own. The result reports whether
// a guard can still fall through and needs a continuation.
func (g *generator) pattern(pattern ast.Pattern, value, success string, unguarded bool) (string, bool) {
	switch p := pattern.(type) {
	case *ast.PWild:
		return "{\n" + success + "}\n", false
	case *ast.PVar:
		return "{\nvar " + mangle(p.Name) + " any = " + value + "\n_ = " + mangle(p.Name) + "\n" + success + "}\n", false
	case *ast.PAs:
		code, guarded := g.pattern(p.Pat, value, success, unguarded)
		return "{\nvar " + mangle(p.Name) + " any = " + value + "\n_ = " + mangle(p.Name) + "\n" + code + "}\n", guarded
	case *ast.PTuple:
		return g.structPattern(g.tupleName(len(p.Elems)), p.Elems, value, success, unguarded)
	case *ast.PCtor:
		ctor := g.ctor(p.Name, p.Pos)
		return g.structPattern(ctorName(ctor), p.Args, value, success, unguarded)
	case *ast.PRecord:
		record := g.recordPats[p]
		fields := make([]ast.Pattern, len(record.Ctor.Fields))
		for i := range fields {
			fields[i] = &ast.PWild{Pos: p.Pos}
		}
		for i, index := range record.Indexes {
			fields[index] = p.Pats[i]
		}
		return g.structPattern(ctorName(record.Ctor), fields, value, success, unguarded)
	case *ast.PLit:
		if unguarded {
			return "{\n" + success + "}\n", false
		}
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
		return "if " + value + ".(" + rep + ") == " + g.expr(p.X) + " {\n" + success + "}\n", true
	}
	g.fail(ast.Pos{}, "unsupported pattern %T", pattern)
	return "", false
}

func (g *generator) irrefutableHead(pattern ast.Pattern) bool {
	switch p := pattern.(type) {
	case *ast.PWild, *ast.PVar, *ast.PTuple:
		return true
	case *ast.PAs:
		return g.irrefutableHead(p.Pat)
	case *ast.PCtor:
		return len(g.ctor(p.Name, p.Pos).Data.Ctors) == 1
	case *ast.PRecord:
		return len(g.recordPats[p].Ctor.Data.Ctors) == 1
	}
	return false
}

func (g *generator) structPattern(name string, fields []ast.Pattern, value, success string, unguarded bool) (string, bool) {
	matched := g.fresh()
	guarded := !unguarded
	for i := len(fields) - 1; i >= 0; i-- {
		var fieldGuarded bool
		success, fieldGuarded = g.pattern(fields[i], fmt.Sprintf("%s.F%d", matched, i), success, unguarded && g.irrefutableHead(fields[i]))
		guarded = guarded || fieldGuarded
	}
	if unguarded {
		return "{\n" + matched + " := " + value + ".(*" + name + ")\n_ = " + matched + "\n" + success + "}\n", guarded
	}
	return "switch " + matched + " := " + value + ".(type) {\ncase *" + name + ":\n_ = " + matched + "\n" + success + "}\n", true
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
