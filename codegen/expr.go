package codegen

import (
	"fmt"
	"strconv"
	"strings"

	"go4t/ast"
	"go4t/types"
)

func (g *generator) block(block *ast.Block) string {
	if block == nil || block.Result == nil {
		g.fail(ast.Pos{}, "block has no result")
	}
	return g.statements(block.Stmts, block.Result)
}

func (g *generator) statements(stmts []ast.Stmt, result ast.Expr) string {
	return g.tailStatements(stmts, result, nil)
}

func (g *generator) tailStatements(stmts []ast.Stmt, result ast.Expr, plan *tailPlan) string {
	var out strings.Builder
	for i, stmt := range stmts {
		switch stmt := stmt.(type) {
		case *ast.FuncDecl:
			group := []*ast.FuncDecl{stmt}
			next := i + 1
			for next < len(stmts) {
				decl, ok := stmts[next].(*ast.FuncDecl)
				if !ok {
					break
				}
				group = append(group, decl)
				next++
			}
			out.WriteString(g.localFunctions(group, g.tailStatements(stmts[next:], result, plan)))
			return out.String()
		case *ast.LetStmt:
			// A nested scope preserves lexical shadowing, including the case where
			// the initializer refers to the previous binding of the same name.
			restore := g.localScope()
			g.bindLet(stmt.Name, stmt.Pat, stmt.Value)
			body := g.tailStatements(stmts[i+1:], result, plan)
			restore()
			out.WriteString(g.letStatement(stmt.Name, stmt.Pat, stmt.Value, body))
			return out.String()
		case *ast.ExprStmt:
			fmt.Fprintf(&out, "_ = %s\n", g.expr(stmt.X))
		default:
			g.fail(ast.Pos{}, "unsupported statement %T", stmt)
		}
	}
	out.WriteString(g.result(result, plan))
	return out.String()
}

func (g *generator) letStatement(name string, pattern ast.Pattern, value ast.Expr, body string) string {
	if pattern == nil {
		return "{\n" + g.letBinding(name, value) + body + "}\n"
	}
	temp := g.fresh()
	rep := g.exprRep(value)
	binding := "var " + temp + " " + rep + " = " + g.expr(value) + "\n_ = " + temp + "\n"
	body, _ = g.patternAs(pattern, temp, rep, body, true)
	return "{\n" + binding + body + "}\n"
}

func (g *generator) letBinding(name string, value ast.Expr) string {
	sourceName := name
	name = mangle(name)
	ann, _ := (*value.AnnSlot()).(*types.Ann)
	if ann != nil && ann.LetScheme != nil {
		scheme := ann.LetScheme
		previous := g.dictScope
		g.dictScope = g.newDictionaryScope(scheme.Constraints, &previous)
		params := g.dictScope.declarations()
		var body string
		if ann.Recursive {
			body = g.recursiveLambdaBinding(sourceName, value.(*ast.Lambda)) + "return " + name + "\n"
		} else {
			body = "return " + g.expr(value) + "\n"
		}
		g.dictScope = previous
		wrapper := g.sourceExpr(value.GetPos(), "func("+strings.Join(params, ", ")+") any {\n"+body+"}")
		return name + " := " + wrapper + "\n_ = " + name + "\n"
	}
	if ann != nil && ann.Recursive {
		return g.recursiveLambdaBinding(sourceName, value.(*ast.Lambda))
	}
	return "var " + name + " " + g.exprRep(value) + " = " + g.expr(value) + "\n_ = " + name + "\n"
}

func (g *generator) expr(expr ast.Expr) (code string) {
	rep := "any"
	defer func() {
		if strings.HasPrefix(code, "func(") {
			code = g.sourceExpr(expr.GetPos(), code)
		}
		code = convert(code, rep, g.exprRep(expr))
	}()
	g.collectAnnotation(expr)
	switch e := expr.(type) {
	case *ast.IntLit:
		rep = "int64"
		return "int64(" + strconv.FormatInt(e.Value, 10) + ")"
	case *ast.FloatLit:
		rep = "float64"
		return "float64(" + strconv.FormatFloat(e.Value, 'g', -1, 64) + ")"
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
			tuple := types.Unalias(g.ann(e.X).Type).(*types.TApp)
			return fmt.Sprintf("any(%s).(*%s).F%d", g.expr(e.X), g.tupleName(len(tuple.Args)), e.Index)
		}
		ann := g.ann(e)
		if len(ann.RecordCases) != 0 {
			return g.commonField(e)
		}
		if scalarRecord(ann.Ctor) {
			rep = scalar(ann.Ctor.Fields[ann.FieldIndex])
		}
		return fmt.Sprintf("any(%s).(*%s).F%d", g.expr(e.X), ctorName(ann.Ctor), ann.FieldIndex)
	case *ast.RecordLit:
		ann := g.ann(e)
		fields := make([]string, len(e.Values))
		for i, value := range e.Values {
			fields[i] = fmt.Sprintf("F%d: %s", ann.FieldIndexes[i], g.recordValue(ann.Ctor, ann.FieldIndexes[i], value))
		}
		return "&" + ctorName(ann.Ctor) + "{" + strings.Join(fields, ", ") + "}"
	case *ast.RecordUpdate:
		ann := g.ann(e)
		if len(ann.RecordCases) != 0 {
			return g.commonRecordUpdate(e)
		}
		base := g.fresh()
		var out strings.Builder
		fmt.Fprintf(&out, "func() any {\n%s := any(%s).(*%s)\n_ = %s\n", base, g.expr(e.X), ctorName(ann.Ctor), base)
		values := make([]string, len(e.Values))
		for i, value := range e.Values {
			values[i] = g.recordValue(ann.Ctor, ann.FieldIndexes[i], value)
		}
		fmt.Fprintf(&out, "return %s\n}()", updatedRecord(ann.Ctor, ann.FieldIndexes, base, values))
		return out.String()
	case *ast.BoolLit:
		rep = "bool"
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
				if r := g.locals[e.Name]; r != "" {
					rep = r
				}
				return mangle(e.Name)
			}
			dicts := make([]string, len(ann.Dicts))
			for i := range ann.Dicts {
				dicts[i] = g.dict(&ann.Dicts[i])
			}
			return mangle(e.Name) + "(" + strings.Join(dicts, ", ") + ")"
		case types.RefFunc:
			return g.functionValue(e)
		case types.RefMethod:
			if len(ann.Dicts) == 1 {
				return "(" + g.dict(&ann.Dicts[0]) + ")." + exported(e.Name)
			}
		case types.RefCtor:
			ctor := g.ctor(e.Name, e.GetPos())
			if len(ctor.Fields) == 0 {
				return construct(ctorName(ctor), nil)
			}
			args := anonymousArgs(len(ctor.Fields))
			if scalarRecord(ctor) {
				for i, field := range ctor.Fields {
					args[i] = convert(args[i], "any", scalar(field))
				}
			}
			return "func(" + strings.Join(anonymousParams(len(ctor.Fields)), ", ") + ") any { return " + construct(ctorName(ctor), args) + " }"
		default:
			g.fail(e.GetPos(), "invalid reference annotation")
		}
		// Taking a constrained function or method as a value binds the hidden
		// dictionaries now, retaining only its source-language arity.
		ft := g.funcType(e)
		return "func(" + strings.Join(anonymousParams(len(ft.Params)), ", ") + ") any { return " + g.identCall(e, anonymousArgs(len(ft.Params))) + " }"
	case *ast.Call:
		if id, ok := e.Fn.(*ast.Ident); ok && g.ann(id).Ref == types.RefFunc {
			if code := g.collectionCall(e, id); code != "" {
				return code
			}
		}
		args := make([]string, len(e.Args))
		for i, arg := range e.Args {
			args[i] = g.expr(arg)
		}
		if ident, ok := e.Fn.(*ast.Ident); ok && g.ann(ident).Ref != types.RefLocal {
			if spec := g.specialized(ident); spec != nil {
				rep = scalar(substitute(spec.function.Scheme.Type.Ret, spec.types))
			} else if f := g.funcs[ident.Name]; f != nil && f.Decl.Extern == nil {
				rep = scalar(f.Scheme.Type.Ret)
			}
			return g.identCall(ident, args)
		}
		ft := g.funcType(e.Fn)
		params := make([]string, len(ft.Params))
		for i := range params {
			params[i] = "any"
		}
		return "any(" + g.expr(e.Fn) + ").(func(" + strings.Join(params, ", ") + ") any)(" + strings.Join(args, ", ") + ")"
	case *ast.Lambda:
		return g.lambda(e)
	case *ast.BlockExpr:
		rep = g.exprRep(e)
		body := g.returning(rep, func() string { return g.block(e.Block) })
		return "func() " + rep + " {\n" + body + "}()"
	case *ast.If:
		rep = g.exprRep(e)
		body := g.returning(rep, func() string {
			return "if " + g.condition(e.Cond) + " {\n" + g.result(e.Then, nil) + "}\n" + g.result(e.Else, nil)
		})
		return "func() " + rep + " {\n" + body + "}()"
	case *ast.Match:
		rep = g.exprRep(e)
		body := g.returning(rep, func() string { return g.matchResult(e, nil) })
		return "func() " + rep + " {\n" + body + "}()"
	case *ast.Do:
		return "func() any {\n" + g.returning("any", func() string { return g.doStatements(e, 0) }) + "}()"
	case *ast.Binary:
		rep = g.exprRep(e)
		return g.binary(e)
	case *ast.Unary:
		rep = g.exprRep(e.X)
		return g.nativeUnary(e)
	}
	g.fail(expr.GetPos(), "unsupported expression %T", expr)
	return ""
}

func (g *generator) funcType(expr ast.Expr) *types.TFunc {
	ft, ok := types.Unalias(g.ann(expr).Type).(*types.TFunc)
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
		callee = functionName(ident.Name)
		if spec := g.specialized(ident); spec != nil {
			for i, t := range spec.function.Scheme.Type.Params {
				args[i] = convert(args[i], g.rep(g.funcType(ident).Params[i]), scalar(substitute(t, spec.types)))
			}
			return spec.name + "(" + strings.Join(args, ", ") + ")"
		}
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
// The final unguarded arm's head is guaranteed by exhaustiveness; nested heads
// are only unguarded when they are irrefutable on their own. Lambda parameters,
// lets and do binds reuse this path. The result reports whether a pattern can still fall through.
func (g *generator) pattern(pattern ast.Pattern, value, success string, unguarded bool) (string, bool) {
	return g.patternAs(pattern, value, "any", success, unguarded)
}

func (g *generator) patternAs(pattern ast.Pattern, value, valueRep, success string, unguarded bool) (string, bool) {
	switch p := pattern.(type) {
	case *ast.PWild:
		return "{\n" + success + "}\n", false
	case *ast.PVar:
		rep := g.rep(g.patternTypes[p])
		return "{\nvar " + mangle(p.Name) + " " + rep + " = " + convert(value, valueRep, rep) + "\n_ = " + mangle(p.Name) + "\n" + success + "}\n", false
	case *ast.PAs:
		code, guarded := g.patternAs(p.Pat, value, valueRep, success, unguarded)
		rep := g.rep(g.patternTypes[p])
		return "{\nvar " + mangle(p.Name) + " " + rep + " = " + convert(value, valueRep, rep) + "\n_ = " + mangle(p.Name) + "\n" + code + "}\n", guarded
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
		case *ast.FloatLit:
			rep = "float64"
		case *ast.StringLit:
			rep = "string"
		case *ast.BoolLit:
			rep = "bool"
		default:
			g.fail(p.Pos, "unsupported literal pattern %T", p.X)
		}
		return "if " + convert(value, valueRep, rep) + " == " + g.expr(p.X) + " {\n" + success + "}\n", true
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
	con, ok := types.Unalias(typ).(*types.TCon)
	return ok && con.Name == name
}

func (g *generator) binary(expr *ast.Binary) string {
	left, right := g.expr(expr.L), g.expr(expr.R)
	switch expr.Op {
	case "&&", "||":
		return "(" + convert(left, g.exprRep(expr.L), "bool") + " " + expr.Op + " " + convert(right, g.exprRep(expr.R), "bool") + ")"
	case "==", "!=", "<", "<=", ">", ">=":
		ann := g.ann(expr)
		if len(ann.Dicts) != 1 {
			g.fail(expr.GetPos(), "comparison requires one dictionary annotation")
		}
		operand := g.exprRep(expr.L)
		if operand == "int64" || (operand == "bool" || operand == "float64") && (expr.Op == "==" || expr.Op == "!=") {
			return g.nativeBinary(expr, left, right)
		}
		if operand == "float64" || operand == "bool" {
			return g.nativeOrdering(expr, operand, left, right)
		}
		dict := "(" + g.dict(&ann.Dicts[0]) + ")"
		if expr.Op == "==" || expr.Op == "!=" {
			equal := dict + ".Eq(" + left + ", " + right + ")"
			if expr.Op == "!=" {
				return "(!any(" + equal + ").(bool))"
			}
			return "any(" + equal + ").(bool)"
		}
		g.ordering = true
		var helper string
		switch expr.Op {
		case "<":
			helper = "ord_lt"
		case "<=":
			helper = "ord_le"
		case ">":
			helper = "ord_gt"
		case ">=":
			helper = "ord_ge"
		}
		return helper + "(" + dict + ".Compare(" + left + ", " + right + "))"
	}
	if rep := g.exprRep(expr); rep == "int64" || rep == "float64" {
		return g.nativeBinary(expr, convert(left, g.exprRep(expr.L), rep), convert(right, g.exprRep(expr.R), rep))
	}
	if expr.Op == "+" && isCon(g.ann(expr).Type, "String") {
		return "rt.AddString(" + left + ", " + right + ")"
	}
	g.fail(expr.GetPos(), "unsupported binary operator %s on %s", expr.Op, g.ann(expr.L).Type)
	return ""
}
