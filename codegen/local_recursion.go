package codegen

import (
	"strings"

	"gofx/ast"
)

func erasedFuncType(arity int) string {
	params := make([]string, arity)
	for i := range params {
		params[i] = "any"
	}
	return "func(" + strings.Join(params, ", ") + ") any"
}

// The Go declaration must precede the closure so that self references capture
// the new function, rather than an outer binding with the same spelling.
func (g *generator) recursiveLambdaBinding(sourceName string, value *ast.Lambda) string {
	name := mangle(sourceName)
	return "var " + name + " " + erasedFuncType(len(value.Params)) + "\n" +
		name + " = " + g.namedLambda(sourceName, value) + "\n_ = " + name + "\n"
}

func (g *generator) localFunctions(group []*ast.FuncDecl, body string) string {
	var out strings.Builder
	out.WriteString("{\n")
	for _, decl := range group {
		out.WriteString("var " + mangle(decl.Name) + " " + erasedFuncType(len(decl.Params)) + "\n")
	}
	for _, decl := range group {
		lambda := &ast.Lambda{
			ExprBase: ast.ExprBase{Pos: decl.Pos, End: decl.End},
			Params:   decl.Params,
			Body:     &ast.BlockExpr{ExprBase: ast.ExprBase{Pos: decl.Body.Pos, End: decl.Body.End}, Block: decl.Body},
		}
		name := mangle(decl.Name)
		out.WriteString(name + " = " + g.namedLambda(decl.Name, lambda) + "\n_ = " + name + "\n")
	}
	out.WriteString(body)
	out.WriteString("}\n")
	return out.String()
}
