package types

import "gofx/ast"

// A maximal consecutive group sees all of its signatures but none of the
// subsequent statements. Check every body before publishing generalized schemes.
func (b *bodyChecker) localFunctions(group []*ast.FuncDecl, env *locals) {
	local := newLocals(env)
	signatures := make([]*TFunc, len(group))
	for i, decl := range group {
		checkValueName(decl.Pos, decl.Name)
		if local.values[decl.Name] != nil {
			fail(decl.Pos, "duplicate local function %s", decl.Name)
		}
		if len(decl.TParams) != 0 {
			fail(decl.Pos, "local functions cannot have type parameters")
		}
		signature := &TFunc{Params: make([]Type, len(decl.Params)), Ret: b.c.valueType(decl.Ret, b.types)}
		for j, param := range decl.Params {
			signature.Params[j] = b.c.valueType(param.Type, b.types)
		}
		signatures[i] = signature
		local.values[decl.Name] = signature
	}
	start := len(b.obligations)
	for i, decl := range group {
		b.functionBody(decl.Params, signatures[i], decl.Body, local)
	}
	// Explicit local signatures contain only concrete types and enclosing rigid
	// parameters today; still generalize against the same pre-group environment.
	schemes := make([]*localScheme, len(group))
	for i, decl := range group {
		schemes[i] = b.generalize(signatures[i], env, start, decl.Pos)
	}
	for i, decl := range group {
		b.bindLocal(decl.Name, signatures[i], schemes[i], env)
	}
}
