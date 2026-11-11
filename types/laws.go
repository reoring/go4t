package types

import (
	"strings"

	"github.com/reoring/go4t/ast"
)

func (c *checker) pairLaws(files []*ast.File) {
	for _, file := range files {
		c.file = file.Name
		for _, decl := range file.Decls {
			c.decl = decl
			switch decl := decl.(type) {
			case *ast.LawDecl:
				law := c.program.Laws[decl.Name]
				law.Proof = c.program.Proofs[decl.Name]
				if law.Proof == nil {
					fail(decl.Pos, "law %s has no proof", diagnosticName(decl.Name))
				}
			case *ast.ProofDecl:
				proof := c.program.Proofs[decl.Name]
				proof.Law = c.program.Laws[decl.Name]
				if proof.Law == nil {
					fail(decl.Pos, "proof %s has no law %s", diagnosticName(decl.Name), diagnosticName(decl.Name))
				}
				if len(decl.Params) != len(proof.Law.ParamTypes) {
					fail(decl.Pos, "proof %s: want %d parameters, got %d", diagnosticName(decl.Name), len(proof.Law.ParamTypes), len(decl.Params))
				}
			}
		}
	}
}

// All law sides are checked before tactics, so forward and self rewrites have
// both their parameter types and their annotated result type available.
func (c *checker) checkLaws(files []*ast.File) {
	for _, file := range files {
		c.file = file.Name
		for _, decl := range file.Decls {
			law, ok := decl.(*ast.LawDecl)
			if !ok {
				continue
			}
			c.decl = decl
			info := c.program.Laws[law.Name]
			b := c.body(nil, nil)
			env := lawParams(law.Params, info.ParamTypes)
			_, sampled := info.Proof.Decl.Body.(*ast.TSample)
			if sampled && law.Requires != nil {
				fail(info.Proof.Decl.Body.GetPos(), "sample does not support requires")
			}
			for i, typ := range info.ParamTypes {
				if !sampled {
					// Proved laws keep the existing Float boundary.
					c.rejectLawFloat(law.Params[i].Pos, typ)
				} else {
					b.sampleParameter(law.Params[i].Pos, typ, make(map[string]bool))
				}
			}
			if law.Requires != nil {
				unify(law.Requires.GetPos(), Bool, b.inferExpected(law.Requires, env, Bool))
			}
			left := b.infer(law.LHS, env)
			right := b.inferExpected(law.RHS, env, left)
			unify(law.RHS.GetPos(), left, right)
			b.finish()
			if sampled {
				b.dictionary(law.Pos, Constraint{Class: "Eq", Arg: AnnOf(law.LHS).Type})
			} else {
				b.rejectLawFloats()
			}
		}
	}
	for _, file := range files {
		c.file = file.Name
		for _, decl := range file.Decls {
			proof, ok := decl.(*ast.ProofDecl)
			if !ok {
				continue
			}
			c.decl = decl
			info := c.program.Proofs[proof.Name]
			b := c.body(nil, nil)
			b.tactic(proof.Body, lawParams(proof.Params, info.Law.ParamTypes), info)
			b.finish()
			for pat, typ := range info.PatVars {
				info.PatVars[pat] = zonk(pat.Pos, typ, b.defaultMeta)
			}
			if _, sampled := proof.Body.(*ast.TSample); !sampled {
				b.rejectLawFloats()
			}
		}
	}
}

func lawParams(params []ast.Param, types []Type) *locals {
	env := newLocals(nil)
	for i, param := range params {
		checkValueName(param.Pos, param.Name)
		if env.values[param.Name] != nil {
			fail(param.Pos, "duplicate parameter %s", param.DisplayName())
		}
		env.values[param.Name] = types[i]
	}
	return env
}

func (b *bodyChecker) tactic(t ast.Tactic, env *locals, proof *ProofInfo) {
	switch t := t.(type) {
	case *ast.TRefl, *ast.TAbsurd:
	case *ast.TSample:
		if proof.Decl.Body != t {
			fail(t.Pos, "sample must be the whole proof body")
		}
		if t.Count < 1 || t.Count > 100000 {
			fail(t.Pos, "sample count must be an Int literal in 1..100000")
		}
	case *ast.TMatch:
		var scrutinee ast.Expr = t.Var
		if t.Scrut != nil {
			scrutinee = t.Scrut
		} else if typ, _ := env.lookup(t.Var.Name); typ == nil {
			fail(t.Var.Pos, "match variable %s is not a proof variable", t.Var.DisplayName())
		}
		typ := b.infer(scrutinee, env)
		head := prune(typ)
		if app, ok := head.(*TApp); ok {
			head = prune(app.Fun)
		}
		con, ok := head.(*TCon)
		if !ok {
			fail(scrutinee.GetPos(), "cannot split %s", typ)
		}
		_, tuple := IsTuple(con)
		if !tuple && con != Bool && b.c.program.DataTypes[con.Name] == nil {
			fail(scrutinee.GetPos(), "cannot split %s", typ)
		}
		if t.Induction {
			data := b.c.program.DataTypes[con.Name]
			if data == nil || t.Var == nil {
				fail(scrutinee.GetPos(), "cannot split %s", typ)
			}
			quantified := false
			for _, param := range proof.Decl.Params {
				quantified = quantified || param.Name == t.Var.Name
			}
			if !quantified {
				fail(t.Var.Pos, "induction variable %s is not a quantified variable", t.Var.DisplayName())
			}
			if t.Bare {
				return
			}
			for _, arm := range t.Arms {
				for _, alternative := range ast.Alternatives(arm.Pat) {
					pat, ok := alternative.(*ast.PCtor)
					if !ok {
						fail(arm.Pos, "induction arms require constructor patterns with field binders")
					}
					for _, field := range pat.Args {
						switch field.(type) {
						case *ast.PVar, *ast.PWild:
						default:
							fail(field.GetPos(), "induction arms require constructor patterns with field binders")
						}
					}
				}
			}
		}
		if t.Bare {
			return
		}
		var rows [][]ast.Pattern
		for _, arm := range t.Arms {
			checkTacticPattern(arm.Pat)
			alternatives := []ast.Pattern{arm.Pat}
			if or, ok := arm.Pat.(*ast.POr); ok {
				alternatives = or.Alts
			}
			var bindings *locals
			for _, alternative := range alternatives {
				local := newLocals(env)
				b.patVars = proof.PatVars
				b.pattern(alternative, typ, local)
				b.patVars = nil
				if bindings != nil {
					if len(bindings.values) != len(local.values) {
						fail(arm.Pos, "or-pattern alternatives must bind the same variables")
					}
					for name, expected := range bindings.values {
						actual := local.values[name]
						if actual == nil {
							fail(arm.Pos, "or-pattern alternatives must bind the same variables")
						}
						unify(arm.Pos, expected, actual)
					}
				}
				bindings = local
				rows = append(rows, []ast.Pattern{alternative})
			}
			b.tactic(arm.Body, bindings, proof)
		}
		missing := b.c.uncovered(rows, []Type{typ}, 5)
		if len(missing) != 0 {
			patterns := make([]string, len(missing))
			for i, row := range missing {
				patterns[i] = row[0]
			}
			fail(t.Pos, "non-exhaustive match; missing: %s", strings.Join(patterns, ", "))
		}
	case *ast.TIf:
		unify(t.Cond.GetPos(), Bool, b.infer(t.Cond, env))
		b.tactic(t.Then, env, proof)
		b.tactic(t.Else, env, proof)
	case *ast.TRewrite:
		law := b.c.program.Laws[t.Law.Name]
		if law == nil {
			fail(t.Law.Pos, "unknown law %s", t.Law.DisplayName())
		}
		if _, sampled := law.Proof.Decl.Body.(*ast.TSample); sampled {
			fail(t.Law.Pos, "cannot rewrite sampled law %s: tested, not proved", t.Law.DisplayName())
		}
		if len(t.Args) != len(law.ParamTypes) {
			fail(t.Pos, "arity mismatch: want %d arguments, got %d", len(law.ParamTypes), len(t.Args))
		}
		*t.Law.AnnSlot() = &Ann{Ref: RefLaw, Type: &TFunc{Params: law.ParamTypes, Ret: AnnOf(law.Decl.LHS).Type}}
		b.exprs = append(b.exprs, t.Law)
		for i, arg := range t.Args {
			unify(arg.GetPos(), law.ParamTypes[i], b.inferExpected(arg, env, law.ParamTypes[i]))
		}
		b.tactic(t.Body, env, proof)
	}
}

func checkTacticPattern(p ast.Pattern) {
	switch p := p.(type) {
	case *ast.PVar, *ast.PWild:
	case *ast.POr:
		for _, alternative := range p.Alts {
			checkTacticPattern(alternative)
		}
	case *ast.PLit:
		if _, ok := p.X.(*ast.BoolLit); !ok {
			fail(p.Pos, "tactic match supports only the literal patterns true and false")
		}
	case *ast.PCtor:
		if p.List {
			fail(p.Pos, "tactic match does not support list patterns")
		}
		for _, arg := range p.Args {
			checkTacticPattern(arg)
		}
	case *ast.PRecord:
		for _, pat := range p.Pats {
			checkTacticPattern(pat)
		}
	case *ast.PTuple:
		for _, elem := range p.Elems {
			checkTacticPattern(elem)
		}
	default:
		fail(p.GetPos(), "tactic match does not support as-patterns")
	}
}

func (b *bodyChecker) rejectLawFloats() {
	for _, expr := range b.exprs {
		b.c.rejectLawFloat(expr.GetPos(), AnnOf(expr).Type)
	}
}

func (c *checker) rejectLawFloat(pos ast.Pos, typ Type) {
	if c.lawHasFloat(typ) {
		fail(pos, "laws do not support Float")
	}
}

// Float is excluded from expressions and exposed type arguments, but fields
// of a data type remain opaque until an expression actually reads them.
func (c *checker) lawHasFloat(typ Type) bool {
	if alias, ok := typ.(*TAlias); ok {
		for _, arg := range alias.Args {
			if c.lawHasFloat(arg) {
				return true
			}
		}
		return c.lawHasFloat(alias.Body)
	}
	switch typ := prune(typ).(type) {
	case *TCon:
		return typ == Float
	case *TApp:
		for _, arg := range typ.Args {
			if c.lawHasFloat(arg) {
				return true
			}
		}
		return c.lawHasFloat(typ.Fun)
	case *TFunc:
		for _, param := range typ.Params {
			if c.lawHasFloat(param) {
				return true
			}
		}
		return c.lawHasFloat(typ.Ret)
	}
	return false
}

// Sampling constructs data, never functions, IO descriptions or opaque handles.
// Check every field, even when another constructor could avoid that field.
func (b *bodyChecker) sampleParameter(pos ast.Pos, typ Type, seen map[string]bool) {
	head := Unalias(typ)
	var args []Type
	if app, ok := head.(*TApp); ok {
		head, args = Unalias(app.Fun), app.Args
	}
	con, ok := head.(*TCon)
	if !ok {
		fail(pos, "sample: no usable generator for %s", typ)
	}
	switch con.Name {
	case "Int", "Float", "Bool", "String", "Unit":
		return
	case "Map":
		b.dictionary(pos, Constraint{Class: "Ord", Arg: args[0]})
		for _, arg := range args {
			b.sampleParameter(pos, arg, seen)
		}
		return
	}
	if _, tuple := IsTuple(con); tuple {
		for _, arg := range args {
			b.sampleParameter(pos, arg, seen)
		}
		return
	}
	data := b.c.program.DataTypes[con.Name]
	if data == nil || len(data.Ctors) == 0 {
		fail(pos, "sample: no usable generator for %s", typ)
	}
	// Inspect type arguments before cutting recursive constructor cycles.
	for _, arg := range args {
		b.sampleParameter(pos, arg, seen)
	}
	key := con.Name
	if seen[key] {
		return
	}
	seen[key] = true
	sub := make(map[*TVar]Type, len(args))
	for i, arg := range args {
		sub[data.TParams[i]] = arg
	}
	for _, ctor := range data.Ctors {
		for _, field := range ctor.Fields {
			b.sampleParameter(pos, substitute(field, sub), seen)
		}
	}
}

// SampleHasDebug asks the ordinary instance matcher about a concrete generated
// input. Missing Debug is not a type error: sampling has a structural observer.
func SampleHasDebug(p *Program, typ Type) bool {
	return sampleHasInstance(p, "Debug", typ)
}

func sampleHasInstance(p *Program, class string, typ Type) bool {
	head := Unalias(typ)
	var args []Type
	if app, ok := head.(*TApp); ok {
		head, args = Unalias(app.Fun), app.Args
	}
	con, ok := head.(*TCon)
	if !ok {
		return false
	}
	for _, inst := range p.Instances {
		if inst.Class.Name != class || inst.TypeCon != con {
			continue
		}
		sub, matches := inst.matchHead(args)
		if !matches {
			return false
		}
		for _, context := range inst.Context {
			if !sampleHasInstance(p, context.Class, substitute(context.Arg, sub)) {
				return false
			}
		}
		return true
	}
	return false
}
