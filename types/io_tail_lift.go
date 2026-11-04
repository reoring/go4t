package types

import "github.com/reoring/go4t/ast"

// ioItem requires the resolved builtin constructor, not an unconstrained monad
// head or a user declaration that happens to be spelled IO.
func ioItem(typ Type) Type {
	app, ok := normalize(typ).(*TApp)
	if ok && len(app.Args) == 1 && prune(app.Fun) == IO {
		return app.Args[0]
	}
	return nil
}

func unresolvedHead(typ Type) bool {
	_, ok := prune(typ).(*TMeta)
	return ok
}

// unresolvedAppHead reports an application whose container is still a meta,
// such as map's F[B] before its arguments have bound F.
func unresolvedAppHead(typ Type) bool {
	app, ok := prune(typ).(*TApp)
	return ok && unresolvedHead(app.Fun)
}

// A variable is not evidence of a pure result. An application with an unknown
// unary head can still unify with IO[_], so it must not acquire another layer.
func ioLiftable(typ Type) bool {
	switch typ := normalize(typ).(type) {
	case *TMeta, *TVar:
		return false
	case *TApp:
		if len(typ.Args) == 1 {
			head := prune(typ.Fun)
			if head == IO || unresolvedHead(head) {
				return false
			}
		}
	}
	return true
}

// Contextual constructor/call checking still reaches projections inside a
// lifted value. Failed payload checks report the original expected/actual pair.
func checkContext(pos ast.Pos, expected, actual Type, lift bool) {
	if item := ioItem(expected); lift && item != nil && ioLiftable(actual) {
		if !unifyTypes(pos, item, actual) {
			fail(pos, "type mismatch: expected %s, got %s", expected, actual)
		}
		return
	}
	unify(pos, expected, actual)
}

// Only checking sites call inferTail. Arguments and record fields retain
// inferExpected, and operands and literal elements retain ordinary inference.
func (b *bodyChecker) inferTail(slot *ast.Expr, env *locals, expected Type) Type {
	e := *slot
	item := ioItem(expected)
	actual := b.inferContext(e, env, expected, item != nil)
	if item == nil || !ioLiftable(actual) {
		return actual
	}
	checkContext(e.GetPos(), expected, actual, true)
	// Module qualification reserves this canonical identity for std/io. Never
	// resolve the synthetic identifier through the user's lexical environment.
	pure := b.c.program.Funcs["ioPure"]
	if pure == nil {
		return actual
	}
	fn := &ast.Ident{
		Name: pure.Decl.Name,
		ExprBase: ast.ExprBase{Ann: &Ann{
			Type: &TFunc{Params: []Type{actual}, Ret: expected},
			Ref:  RefFunc, Inst: []Type{item},
		}},
	}
	call := &ast.Call{
		ExprBase: ast.ExprBase{Pos: e.GetPos(), End: e.GetEnd(), Ann: &Ann{Type: expected, ImplicitIO: true}},
		Fn:       fn, Args: []ast.Expr{e},
	}
	b.exprs = append(b.exprs, fn, call)
	*slot = call
	return expected
}

func implicitIO(e ast.Expr) *ast.Call {
	if block, ok := e.(*ast.BlockExpr); ok {
		return implicitIO(block.Block.Result)
	}
	call, ok := e.(*ast.Call)
	if ok && AnnOf(call).ImplicitIO {
		return call
	}
	return nil
}

func removeIOLift(e ast.Expr) ast.Expr {
	if block, ok := e.(*ast.BlockExpr); ok {
		block.Block.Result = removeIOLift(block.Block.Result)
		AnnOf(block).Type = AnnOf(block.Block.Result).Type
		return block
	}
	return implicitIO(e).Args[0]
}

// A wholly pure branch expression is ioPure(if/match ...), not a collection of
// per-arm closures. Mixed branches keep their individual lifts. Besides avoiding
// extra closures, this preserves the ordinary explicit-ioPure codegen exactly.
func (b *bodyChecker) hoistIOLift(e ast.Expr, actual Type) Type {
	switch e := e.(type) {
	case *ast.If:
		then, otherwise := implicitIO(e.Then), implicitIO(e.Else)
		if then != nil && otherwise != nil {
			e.Then, e.Else = removeIOLift(e.Then), removeIOLift(e.Else)
			return AnnOf(e.Then).Type
		}
	case *ast.Match:
		if len(e.Arms) == 0 {
			return actual
		}
		for _, arm := range e.Arms {
			if implicitIO(arm.Body) == nil {
				return actual
			}
		}
		for i := range e.Arms {
			e.Arms[i].Body = removeIOLift(e.Arms[i].Body)
		}
		return AnnOf(e.Arms[0].Body).Type
	}
	return actual
}
