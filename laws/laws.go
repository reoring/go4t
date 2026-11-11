// Package laws checks source-level equational proofs after type checking and
// generates explicit sampled-law tests. Normalization uses Int ring laws,
// constructor induction, rewriting, and branch-local Boolean assumptions.
package laws

import (
	"fmt"
	"sort"
	"strings"

	"github.com/reoring/go4t/ast"
	"github.com/reoring/go4t/types"
)

type parameter struct {
	Name string
	Type types.Type
}

type lawGoal struct {
	Name     string
	Params   []parameter
	LHS, RHS ast.Expr
}

// StepLimit bounds normalization and proof traversal. Exhaustion is an error,
// never evidence of equality, even after the termination check succeeds.
const StepLimit = 10000

// CheckProgram verifies proofs and validates sampled laws, including imports.
// It returns positioned failures; accepted samples are never proof evidence.
// The checked program and its annotations are never modified.
func CheckProgram(p *types.Program) []*types.Error {
	if p == nil {
		return []*types.Error{{Msg: "nil checked program"}}
	}
	files := make(map[ast.Node]string)
	for _, file := range p.Files {
		for _, decl := range file.Decls {
			files[decl] = file.Name
		}
	}
	diagnostic := func(node ast.Node, name, message string) *types.Error {
		separator := ": "
		if strings.HasPrefix(message, "depends on ") {
			separator = " "
		}
		return &types.Error{Pos: node.GetPos(), End: node.GetEnd(), File: files[node], Msg: "law " + sourceName(p, name) + separator + message}
	}
	var names []string
	for name := range p.Laws {
		names = append(names, name)
	}
	sort.Strings(names)
	edges := make(map[string][]string)
	for _, name := range names {
		if proof := p.Proofs[name]; proof != nil {
			set := make(map[string]bool)
			ast.Walk(proof.Decl, func(node ast.Node) {
				if rewrite, ok := node.(*ast.TRewrite); ok && rewrite.Law.Name != name {
					set[rewrite.Law.Name] = true
				}
			})
			for to := range set {
				edges[name] = append(edges[name], to)
			}
			sort.Strings(edges[name])
		}
	}
	var errors []*types.Error
	valid := make(map[string]bool)
	for _, component := range lawComponents(names, edges) {
		if len(component) > 1 {
			members := make([]string, len(component))
			for i, name := range component {
				members[i] = sourceName(p, name)
			}
			name := component[0]
			errors = append(errors, diagnostic(p.Laws[name].Decl, name, "law dependency cycle: "+strings.Join(members, ", ")))
			continue
		}
		name := component[0]
		info := p.Laws[name]
		ok := true
		for _, dep := range edges[name] {
			if !valid[dep] {
				ok = false
			}
		}
		if !ok {
			continue
		}
		proof := p.Proofs[name]
		if proof == nil {
			errors = append(errors, diagnostic(info.Decl, name, "missing proof"))
			continue
		}
		if _, sampled := proof.Decl.Body.(*ast.TSample); sampled {
			if err := checkSample(p, info); err != nil {
				errors = append(errors, diagnostic(info.Decl, name, err.Error()))
			}
			// A sample never becomes a proved rewrite dependency.
			continue
		}
		e := newEvaluator(p)
		law, err := e.extractLaw(name)
		if err == nil {
			err = e.checkDependencies(info, proof)
		}
		if err != nil {
			errors = append(errors, diagnostic(info.Decl, name, err.Error()))
			continue
		}
		env, vars := e.proofEnvironment(law, proof)
		err = e.prove(law, proof.Decl.Body, env, vars)
		if err != nil {
			node := ast.Node(proof.Decl)
			message := err.Error()
			if failure, yes := err.(*proofFailure); yes {
				node, message = failure.node, failure.message
			}
			d := diagnostic(node, name, message)
			d.File = files[proof.Decl]
			errors = append(errors, d)
			continue
		}
		valid[name] = true
	}
	return errors
}

// Components are emitted dependency-first. A recursive component is diagnosed
// once, even when it contains several overlapping cycles. Self edges are
// excluded by the caller: they are checked as structural induction hypotheses.
func lawComponents(names []string, edges map[string][]string) [][]string {
	index, low := make(map[string]int), make(map[string]int)
	active := make(map[string]bool)
	next := 0
	var stack []string
	var components [][]string
	var visit func(string)
	visit = func(name string) {
		next++
		index[name], low[name], active[name] = next, next, true
		stack = append(stack, name)
		for _, to := range edges[name] {
			if index[to] == 0 {
				visit(to)
				if low[to] < low[name] {
					low[name] = low[to]
				}
			} else if active[to] && index[to] < low[name] {
				low[name] = index[to]
			}
		}
		if low[name] != index[name] {
			return
		}
		var component []string
		for {
			last := stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			active[last] = false
			component = append(component, last)
			if last == name {
				break
			}
		}
		sort.Strings(component)
		components = append(components, component)
	}
	for _, name := range names {
		if index[name] == 0 {
			visit(name)
		}
	}
	return components
}

func sourceName(p *types.Program, name string) string {
	if original := p.ModuleNames[name]; original != "" {
		return original
	}
	return name
}

type proofVar struct {
	value     *term
	typ       types.Type
	ancestors []int
}

type proofFailure struct {
	node    ast.Tactic
	message string
}

func (f *proofFailure) Error() string { return f.message }

func (e *evaluator) proofEnvironment(law lawGoal, proof *types.ProofInfo) (*environment, map[string]proofVar) {
	env := child(nil)
	vars := make(map[string]proofVar)
	for i, param := range law.Params {
		name := proof.Decl.Params[i].Name
		v := e.fresh(name)
		env.values[param.Name] = v
		vars[name] = proofVar{value: v, typ: param.Type}
	}
	return env, vars
}

func proofScope(vars map[string]proofVar) *environment {
	env := child(nil)
	for name, v := range vars {
		env.values[name] = v.value
	}
	return env
}

func (e *evaluator) condition(expr ast.Expr, vars map[string]proofVar) (*term, error) {
	value, err := e.eval(expr, proofScope(vars), nil)
	if err != nil {
		return nil, err
	}
	return e.normal(value)
}

func (e *evaluator) prove(law lawGoal, proof ast.Tactic, env *environment, vars map[string]proofVar) (err error) {
	defer func() {
		if err == nil {
			return
		}
		if _, ok := err.(*proofFailure); ok {
			return
		}
		message := err.Error()
		// Preserve the actual failure when the diagnostic's readback also runs
		// out of fuel or encounters a forbidden dependency.
		if left, right, readErr := e.goal(law, env); readErr == nil && !strings.Contains(message, "\nLHS: ") {
			d := e.displayTerms(left, right)
			message += fmt.Sprintf("\nLHS: %s\nRHS: %s", d.display(left), d.display(right))
		}
		err = &proofFailure{node: proof, message: message}
	}()
	if err = e.step(); err != nil {
		return err
	}
	switch tactic := proof.(type) {
	case *ast.TRefl:
		left, right, err := e.goal(law, env)
		if err != nil {
			return err
		}
		if !alphaEqual(left, right, nil) {
			d := e.displayTerms(left, right)
			stuck, differs := d.diagnose(left, right)
			message := "stuck normal forms differ"
			if stuck != "" {
				message += "\n" + stuck
			}
			message += "\n" + differs
			return fmt.Errorf("%s\nLHS: %s\nRHS: %s\n%s", message, d.display(left), d.display(right), e.counterexample(law))
		}
	case *ast.TRewrite:
		rule, err := e.rewriteRule(law, tactic, env, vars)
		if err != nil {
			return err
		}
		left, right, err := e.goal(law, env)
		if err != nil {
			return err
		}
		left, err = e.replace(left, rule.left, rule.right)
		if err != nil {
			return err
		}
		right, err = e.replace(right, rule.left, rule.right)
		if err != nil {
			return err
		}
		parent := e.currentGoal
		e.currentGoal = &rewriteRule{left: left, right: right}
		err = e.prove(law, tactic.Body, env, vars)
		e.currentGoal = parent
		return err
	case *ast.TIf:
		condition, err := e.condition(tactic.Cond, vars)
		if err != nil {
			return err
		}
		if condition.kind == "bool" {
			return fmt.Errorf("condition already decided")
		}
		for i, arm := range []ast.Tactic{tactic.Then, tactic.Else} {
			branch := assumption{condition: condition, value: boolean(i == 0), parent: e.assumptions}
			e.assumptions = &branch
			err := e.prove(law, arm, env, vars)
			e.assumptions = branch.parent
			if err != nil {
				return err
			}
		}
	case *ast.TMatch:
		v := vars[tactic.Var.Name]
		if e.solutions[v.value.id] != nil {
			return fmt.Errorf("proof variable %s is already split", tactic.Var.Name)
		}
		arms := tactic.Arms
		if tactic.Bare {
			arms = e.inductionArms(tactic, v.typ)
		}
		for _, arm := range arms {
			branch := copyProofVars(vars)
			value := e.splitPattern(arm.Pat, v, branch)
			if value != v.value {
				e.solutions[v.value.id] = value
			}
			body := arm.Body
			if tactic.Induction {
				body, err = e.induction(law, tactic, arm, value, v, env, branch)
				if err != nil {
					delete(e.solutions, v.value.id)
					return err
				}
			}
			err := e.prove(law, body, env, branch)
			delete(e.solutions, v.value.id)
			if err != nil {
				return err
			}
		}
	default:
		return fmt.Errorf("unsupported proof %T", proof)
	}
	return nil
}

func copyProofVars(vars map[string]proofVar) map[string]proofVar {
	branch := make(map[string]proofVar, len(vars))
	for name, value := range vars {
		branch[name] = value
	}
	return branch
}

// splitPattern constructs precisely the pattern's refinement. A variable-only
// pattern aliases the scrutinee (not a strict subterm); each constructor/tuple
// descent extends ancestry, including omitted record fields and wildcards.
func (e *evaluator) splitPattern(pattern ast.Pattern, v proofVar, vars map[string]proofVar) *term {
	var ctor *types.Ctor
	var patterns []ast.Pattern
	var fields []types.Type
	switch p := pattern.(type) {
	case *ast.PWild:
		return v.value
	case *ast.PVar:
		vars[p.Name] = v
		return v.value
	case *ast.PTuple:
		_, fields = typeHead(v.typ)
		patterns = p.Elems
	case *ast.PCtor:
		ctor = e.ctors[p.Name]
		patterns = p.Args
	case *ast.PRecord:
		record := e.p.RecordPats[p]
		ctor = record.Ctor
		patterns = make([]ast.Pattern, len(ctor.Fields))
		for i, pattern := range p.Pats {
			patterns[record.Indexes[i]] = pattern
		}
	}
	kind := "tuple"
	if ctor != nil {
		kind = "ctor"
		con, args := typeHead(v.typ)
		data := e.p.DataTypes[con.Name]
		sub := make(map[*types.TVar]types.Type, len(args))
		for i, arg := range args {
			sub[data.TParams[i]] = arg
		}
		fields = make([]types.Type, len(ctor.Fields))
		for i, field := range ctor.Fields {
			fields[i] = substituteType(field, sub)
		}
	}
	values := make([]*term, len(fields))
	for i, field := range fields {
		name := "_"
		if p, ok := patterns[i].(*ast.PVar); ok {
			name = p.Name
		}
		value := e.fresh(name)
		ancestors := append(append([]int(nil), v.ancestors...), v.value.id)
		nested := proofVar{value: value, typ: field, ancestors: ancestors}
		if patterns[i] != nil {
			values[i] = e.splitPattern(patterns[i], nested, vars)
		} else {
			values[i] = value
		}
	}
	return &term{kind: kind, ctor: ctor, args: values}
}

func typeHead(t types.Type) (*types.TCon, []types.Type) {
	t = types.Unalias(t)
	if app, ok := t.(*types.TApp); ok {
		con, _ := types.Unalias(app.Fun).(*types.TCon)
		return con, app.Args
	}
	con, _ := t.(*types.TCon)
	return con, nil
}

func substituteType(t types.Type, sub map[*types.TVar]types.Type) types.Type {
	switch t := types.Unalias(t).(type) {
	case *types.TVar:
		if value := sub[t]; value != nil {
			return value
		}
		return t
	case *types.TApp:
		args := make([]types.Type, len(t.Args))
		for i, arg := range t.Args {
			args[i] = substituteType(arg, sub)
		}
		return types.App(substituteType(t.Fun, sub), args...)
	case *types.TFunc:
		params := make([]types.Type, len(t.Params))
		for i, param := range t.Params {
			params[i] = substituteType(param, sub)
		}
		return &types.TFunc{Params: params, Ret: substituteType(t.Ret, sub)}
	default:
		return t
	}
}

func (e *evaluator) unsupportedType(t types.Type, firstOrder bool, seen map[string]bool) string {
	return e.unsupportedTypeAt(t, firstOrder, true, seen)
}

// Constructor fields may contain opaque Float values, but IO and higher-order
// quantified fields retain their existing recursive exclusions.
func (e *evaluator) unsupportedTypeAt(t types.Type, firstOrder, checkFloat bool, seen map[string]bool) string {
	switch t := types.Unalias(t).(type) {
	case *types.TCon:
		switch t.Name {
		case "Float":
			if checkFloat {
				return "Float is not supported in laws"
			}
		case "IO":
			return "law depends on IO"
		}
		if data := e.p.DataTypes[t.Name]; data != nil && !seen[t.Name] {
			seen[t.Name] = true
			for _, ctor := range data.Ctors {
				for _, field := range ctor.Fields {
					if reason := e.unsupportedTypeAt(field, firstOrder, false, seen); reason != "" {
						return reason
					}
				}
			}
		}
	case *types.TApp:
		if reason := e.unsupportedTypeAt(t.Fun, firstOrder, checkFloat, seen); reason != "" {
			return reason
		}
		for _, arg := range t.Args {
			if reason := e.unsupportedTypeAt(arg, firstOrder, checkFloat, seen); reason != "" {
				return reason
			}
		}
	case *types.TFunc:
		if firstOrder {
			return "higher-order quantified parameters are not supported"
		}
		for _, param := range t.Params {
			if reason := e.unsupportedTypeAt(param, false, checkFloat, seen); reason != "" {
				return reason
			}
		}
		return e.unsupportedTypeAt(t.Ret, false, checkFloat, seen)
	}
	return ""
}
