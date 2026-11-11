package types

import "github.com/reoring/go4t/ast"

// TailCalls finds direct self calls in result positions that the backend can
// lower to loops. The backend supplies dictionary equivalence and direct IO
// sequencing predicates, including any specialization it has performed.
func TailCalls(name string, ref RefKind, params []ast.Param, body ast.Expr, dictionaries func([]DictRef) bool, directIO func(*ast.Do) bool) map[*ast.Call]bool {
	var calls map[*ast.Call]bool
	var visit func(ast.Expr)
	visit = func(expr ast.Expr) {
		switch e := expr.(type) {
		case *ast.Call:
			id, ok := e.Fn.(*ast.Ident)
			if !ok || id.Name != name || len(e.Args) != len(params) {
				return
			}
			ann := AnnOf(id)
			if ann.Ref != ref || !dictionaries(ann.Dicts) {
				return
			}
			if calls == nil {
				calls = make(map[*ast.Call]bool)
			}
			calls[e] = true
		case *ast.If:
			visit(e.Then)
			visit(e.Else)
		case *ast.Match:
			for _, arm := range e.Arms {
				if ref != RefLocal || !patternBinds(arm.Pat, name) {
					visit(arm.Body)
				}
			}
		case *ast.BlockExpr:
			if ref == RefLocal {
				for _, stmt := range e.Block.Stmts {
					switch stmt := stmt.(type) {
					case *ast.LetStmt:
						if stmt.Name == name || patternBinds(stmt.Pat, name) {
							return
						}
					case *ast.FuncDecl:
						if stmt.Name == name {
							return
						}
					}
				}
			}
			visit(e.Block.Result)
		case *ast.Do:
			if directIO(e) {
				visit(e.Result)
			}
		}
	}
	for _, param := range params {
		if ref == RefLocal && (param.Name == name || patternBinds(param.Pat, name)) {
			return nil
		}
	}
	visit(body)
	return calls
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
