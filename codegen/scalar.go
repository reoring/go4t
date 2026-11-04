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
	if strings.HasPrefix(to, "value:") {
		return "*any(" + code + ").(*" + strings.TrimPrefix(to, "value:") + ")"
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
		return functionName(id.Name)
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
	record := g.scalarRecordType(f.Scheme.Type.Ret)
	returnRep := ret
	if record != nil && f.Decl.Extern == nil {
		ret = ctorName(record)
		returnRep = "record:" + ret
	}
	code := g.returning(returnRep, func() string {
		if f.Decl.Extern != nil {
			return g.emitExtern(f)
		}
		if plan != nil {
			return g.loopBody(plan, f.Decl.Params, body)
		}
		return g.block(f.Decl.Body)
	})
	if record != nil && f.Decl.Extern == nil {
		args := make([]string, len(params))
		for i, param := range params {
			args[i] = strings.Fields(param)[0]
		}
		nativeParams := append(params, "_go4tResult *"+ret)
		native := g.line(f.Decl.Pos) + fmt.Sprintf("func %s_scalar(%s) {\n%s}\n", name, strings.Join(nativeParams, ", "), code)
		args = append(args, "&result")
		return native + g.line(f.Decl.Pos) + fmt.Sprintf("func %s(%s) any { var result %s; %s_scalar(%s); return &result }\n", name, strings.Join(params, ", "), ret, name, strings.Join(args, ", "))
	}
	return g.line(f.Decl.Pos) + fmt.Sprintf("func %s(%s) %s {\n%s}\n", name, strings.Join(params, ", "), ret, code)
}

func (g *generator) returnValue(e ast.Expr) string {
	if record, ok := strings.CutPrefix(g.returnRep, "record:"); ok {
		if update, ok := e.(*ast.RecordUpdate); ok && len(g.ann(update).RecordCases) == 0 {
			ann := g.ann(update)
			base := g.fresh()
			var out strings.Builder
			fmt.Fprintf(&out, "%s := any(%s).(*%s)\n", base, g.expr(update.X), record)
			values := make([]string, len(update.Values))
			for i, value := range update.Values {
				values[i] = g.fresh()
				fmt.Fprintf(&out, "%s := %s\n", values[i], g.recordValue(ann.Ctor, ann.FieldIndexes[i], value))
			}
			// Only the private accumulator can alias the destination. Replacements
			// are fully evaluated before any write, including field swaps.
			fmt.Fprintf(&out, "if _go4tResult != %s { *_go4tResult = *%s }\n", base, base)
			for i, value := range values {
				fmt.Fprintf(&out, "_go4tResult.F%d = %s\n", ann.FieldIndexes[i], value)
			}
			out.WriteString("return\n")
			return out.String()
		}
		return "*_go4tResult = " + convert(g.expr(e), "any", "value:"+record) + "\nreturn\n"
	}
	return "return " + convert(g.expr(e), g.exprRep(e), g.returnRep) + "\n"
}

// Numeric records retain the pointer-shaped erased ABI, but their payload does
// not need an interface allocation for every field of every immutable update.
func scalarRecord(c *types.Ctor) bool {
	if c == nil || len(c.Data.TParams) != 0 || len(c.Data.Ctors) != 1 || len(c.FieldNames) == 0 {
		return false
	}
	for _, field := range c.Fields {
		if scalar(field) == "any" {
			return false
		}
	}
	return true
}

func (g *generator) recordValue(ctor *types.Ctor, index int, value ast.Expr) string {
	code := g.expr(value)
	if scalarRecord(ctor) {
		return convert(code, g.exprRep(value), scalar(ctor.Fields[index]))
	}
	return code
}

func (g *generator) scalarRecordType(t types.Type) *types.Ctor {
	con, ok := types.Unalias(t).(*types.TCon)
	if !ok {
		return nil
	}
	return g.scalarRecords[con.Name]
}

// Only an unmodified compare reference, or its exact eta expansion, can carry
// ordering provenance. Arbitrary closures keep their ordinary function ABI.
func (g *generator) nativeCompare(e ast.Expr) string {
	if lambda, ok := e.(*ast.Lambda); ok {
		if len(lambda.Params) != 2 {
			return ""
		}
		body := lambda.Body
		if block, ok := body.(*ast.BlockExpr); ok && len(block.Block.Stmts) == 0 {
			body = block.Block.Result
		}
		call, ok := body.(*ast.Call)
		if !ok || len(call.Args) != 2 {
			return ""
		}
		for i, arg := range call.Args {
			id, ok := arg.(*ast.Ident)
			if !ok || lambda.Params[i].Pat != nil || id.Name != lambda.Params[i].Name || g.ann(id).Ref != types.RefLocal {
				return ""
			}
		}
		e = call.Fn
	}
	id, ok := e.(*ast.Ident)
	if !ok || g.standard[id.Name] != "std/prelude.compare" {
		return ""
	}
	ann := g.ann(id)
	if ann.Ref != types.RefMethod || len(ann.Dicts) != 1 {
		return ""
	}
	return "rt.NativeCompare(" + g.dict(&ann.Dicts[0]) + ")"
}

func (g *generator) collectionCall(call *ast.Call, id *ast.Ident) string {
	f := g.funcs[id.Name]
	if f == nil {
		return ""
	}
	switch g.standard[id.Name] {
	case "std/list.fold":
		return g.nativeFold(call, "ListFoldScalar")
	case "std/list.sort":
		if len(g.ann(id).Dicts) == 1 {
			return "rt.ListSortBy(" + g.expr(call.Args[0]) + ", rt.NativeCompare(" + g.dict(&g.ann(id).Dicts[0]) + "))"
		}
	case "std/list.sortBy":
		if cmp := g.nativeCompare(call.Args[1]); cmp != "" {
			return "rt.ListSortBy(" + g.expr(call.Args[0]) + ", " + cmp + ")"
		}
	case "std/list.sortOn":
		if len(g.ann(id).Dicts) == 1 {
			return "rt.ListSortOn(" + g.expr(call.Args[0]) + ", " + g.expr(call.Args[1]) + ", rt.NativeCompare(" + g.dict(&g.ann(id).Dicts[0]) + "))"
		}
	}
	if f.Decl.Extern == nil || f.Decl.Extern.ImportPath != "go4t/rt" {
		return ""
	}
	index := -1
	switch f.Decl.Extern.Name {
	case "ArrayFold":
		return g.nativeFold(call, "ArrayFoldScalar")
	case "MutMapNew":
		index = 0
	case "MapInsert":
		index = 3
	case "MapLookup", "MapDelete", "MapMember", "MapUnion",
		"SetInsert", "SetDelete", "SetMember", "SetUnion", "SetIntersection", "SetDifference":
		index = 2
	case "MapFromList", "SetFromList", "ArraySortBy", "MutMapThaw":
		index = 1
	}
	if index < 0 || index >= len(call.Args) {
		return ""
	}
	cmp := g.nativeCompare(call.Args[index])
	if cmp == "" {
		return ""
	}
	args := make([]string, len(call.Args))
	for i, arg := range call.Args {
		if i == index {
			args[i] = cmp
		} else {
			args[i] = g.expr(arg)
		}
	}
	return g.identCall(id, args)
}

func (g *generator) nativeFold(call *ast.Call, helper string) string {
	ft := g.funcType(call.Args[2])
	ret := g.rep(ft.Ret)
	record := g.scalarRecordType(ft.Ret)
	if ret == "any" && record == nil {
		return ""
	}
	if record != nil {
		ret = ctorName(record)
	}
	callback := g.nativeFoldCallback(call.Args[2], ret, record)
	if callback == "" {
		return ""
	}
	init := g.expr(call.Args[1])
	if record != nil {
		init = convert(init, "any", "value:"+ret)
		helper = strings.TrimSuffix(helper, "Scalar") + "Record"
	}
	code := "rt." + helper + "(" + g.expr(call.Args[0]) + ", " + init + ", " + callback + ")"
	if record != nil {
		code = "rt.BoxRecord(" + code + ")"
	}
	return code
}

func (g *generator) nativeFoldCallback(e ast.Expr, ret string, record *types.Ctor) string {
	if lambda, ok := e.(*ast.Lambda); ok && record == nil {
		element := convert("p1", "any", g.rep(g.funcType(e).Params[1]))
		body := g.sourceExpr(e.GetPos(), g.lambdaCode(lambda, nil, false))
		return g.sourceExpr(e.GetPos(), "func(p0 "+ret+", p1 any) "+ret+" { return "+body+"(p0, "+element+") }")
	}
	id, ok := e.(*ast.Ident)
	if !ok || g.ann(id).Ref != types.RefFunc {
		return ""
	}
	f := g.funcs[id.Name]
	if f == nil || f.Decl.Extern != nil {
		return ""
	}
	if record != nil && g.scalarRecordType(f.Scheme.Type.Ret) != record {
		return ""
	}
	params := []string{"p0 " + ret, "p1 any"}
	elementRep := scalar(f.Scheme.Type.Params[1])
	if spec := g.specialized(id); spec != nil {
		elementRep = scalar(substitute(f.Scheme.Type.Params[1], spec.types))
	}
	args := []string{"p0", convert("p1", "any", elementRep)}
	if record != nil {
		params[0] = "p0 *" + ret
	}
	target := functionName(id.Name)
	if spec := g.specialized(id); spec != nil {
		target = spec.name
	}
	code := g.identCall(id, args)
	if record != nil {
		code = strings.Replace(code, target+"(", target+"_scalar(", 1)
		code = strings.TrimSuffix(code, ")") + ", p0)"
		return g.sourceExpr(e.GetPos(), "func("+strings.Join(params, ", ")+") { "+code+" }")
	} else {
		resultRep := scalar(f.Scheme.Type.Ret)
		if spec := g.specialized(id); spec != nil {
			resultRep = scalar(substitute(f.Scheme.Type.Ret, spec.types))
		}
		code = convert(code, resultRep, ret)
	}
	return g.sourceExpr(e.GetPos(), "func("+strings.Join(params, ", ")+") "+ret+" { return "+code+" }")
}
