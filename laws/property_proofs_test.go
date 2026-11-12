package laws

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
	"testing"

	"github.com/reoring/go4t/ast"
	"github.com/reoring/go4t/types"
)

// Record each prefix of the real, checked AST tactic tree. Even an invalid
// theorem's residual terms must normalize soundly: those terms, not the
// unproved equality, are compiled as the independent runtime oracle.
func propertyProofVariants(t *testing.T, e *evaluator, law lawGoal, side int) []propertyVariant {
	t.Helper()
	var result []propertyVariant
	env, vars := e.proofEnvironment(law, e.p.Proofs[law.Name])
	parentAssumptions := e.assumptions
	if err := e.assumePrecondition(law, env); err != nil {
		t.Fatal(err)
	}
	defer func() { e.assumptions = parentAssumptions }()
	var visit func(ast.Tactic, map[string]proofVar, string)
	visit = func(proof ast.Tactic, vars map[string]proofVar, path string) {
		e.fuel = StepLimit
		if condition, _, err := e.contradiction(); err != nil {
			t.Fatalf("partial proof %s/%s contradiction: %v", law.Name, path, err)
		} else if condition != nil {
			// A certified impossible case has no runtime instance. Do not
			// pretend to compare one; reachable cases still require samples.
			return
		}
		left, right, err := e.goal(law, env)
		if err != nil {
			t.Fatalf("partial proof %s/%s: %v", law.Name, path, err)
		}
		variant := propertyVariant{name: path, normal: []*term{left, right}[side], env: child(nil)}
		for _, param := range law.Params {
			variant.env.values[param.Name] = e.instantiate(env.lookup(param.Name))
		}
		for a := e.assumptions; a != nil; a = a.parent {
			variant.assumptions = &assumption{condition: e.instantiate(a.condition), value: e.instantiate(a.value), parent: variant.assumptions}
		}
		var names []string
		for name := range vars {
			names = append(names, name)
		}
		sort.Strings(names)
		seen := make(map[int]bool)
		for _, name := range names {
			v := vars[name]
			if e.solutions[v.value.id] != nil || seen[v.value.id] {
				continue
			}
			seen[v.value.id] = true
			variant.params = append(variant.params, parameter{Name: propertyVariable(v.value), Type: v.typ})
			variant.vars = append(variant.vars, v.value)
		}
		variant.source = propertyTermSource(t, e, variant.normal)
		result = append(result, variant)
		switch proof := proof.(type) {
		case *ast.TRefl, *ast.TAbsurd:
		case *ast.TRewrite:
			rule, err := e.rewriteRule(law, proof, env, vars)
			if err != nil {
				if law.Name == "useBare" && strings.Contains(err.Error(), "left side is a free variable") ||
					strings.Contains(err.Error(), "precondition not established:") {
					return
				}
				t.Fatalf("partial rewrite %s/%s: %v", law.Name, path, err)
			}
			left, err = e.replace(left, rule.left, rule.right)
			if err != nil {
				t.Fatal(err)
			}
			right, err = e.replace(right, rule.left, rule.right)
			if err != nil {
				t.Fatal(err)
			}
			parent := e.currentGoal
			e.currentGoal = &rewriteRule{left: left, right: right}
			visit(proof.Body, vars, path+"/rewrite="+proof.Law.Name)
			e.currentGoal = parent
		case *ast.TIf:
			condition, err := e.condition(proof.Cond, vars)
			if err != nil {
				t.Fatal(err)
			}
			parent := e.assumptions
			for i, arm := range []ast.Tactic{proof.Then, proof.Else} {
				e.assumptions = parent
				e.addAssumption(condition, i == 0)
				visit(arm, vars, fmt.Sprintf("%s/if=%t", path, i == 0))
			}
			e.assumptions = parent
		case *ast.TMatch:
			scrutinee, v, variable, err := e.tacticScrutinee(proof, vars)
			if err != nil {
				t.Fatalf("partial match %s/%s: %v", law.Name, path, err)
			}
			arms := proof.Arms
			if proof.Bare {
				arms = e.inductionArms(proof, v.typ)
			}
			for i, arm := range arms {
				for j, pattern := range ast.Alternatives(arm.Pat) {
					branch := copyProofVars(vars)
					if !variable {
						v.value = e.fresh("$match")
					}
					value := e.splitPattern(pattern, v, branch)
					propertySplitVariables(e, value, v.typ, branch)
					parent := e.assumptions
					if variable {
						if value != v.value {
							e.solutions[v.value.id] = value
						}
					} else {
						e.refineMatch(scrutinee, value)
					}
					body := arm.Body
					if proof.Induction {
						caseArm := arm
						caseArm.Pat = pattern
						body, err = e.induction(law, proof, caseArm, value, v, env, branch)
						if err != nil {
							t.Fatal(err)
						}
					}
					visit(body, branch, fmt.Sprintf("%s/match=%d.%d", path, i, j))
					e.assumptions = parent
					if variable {
						delete(e.solutions, v.value.id)
					}
				}
			}
		default:
			t.Fatalf("unsupported property proof %T", proof)
		}
	}
	visit(e.p.Proofs[law.Name].Decl.Body, vars, "partial")
	return result
}

// Wildcards and omitted record fields still represent universally quantified
// values in a residual term, even though source tactics cannot refer to them.
func propertySplitVariables(e *evaluator, value *term, typ types.Type, vars map[string]proofVar) {
	if value.kind == "var" {
		for _, v := range vars {
			if v.value.id == value.id {
				return
			}
		}
		vars[fmt.Sprintf("$%d", value.id)] = proofVar{value: value, typ: typ}
		return
	}
	var fields []types.Type
	if value.kind == "tuple" {
		_, fields = typeHead(typ)
	} else if value.kind == "ctor" {
		data, args := propertyData(e, typ)
		fields = propertyFields(data, args, value.ctor)
	}
	for i, field := range fields {
		propertySplitVariables(e, value.args[i], field, vars)
	}
}

func propertyVariable(value *term) string {
	return "propertyVar" + strconv.Itoa(value.id)
}

// The residual oracle is source code, not another evaluator invocation. Use
// IDs for both free variables and binders so serialization cannot capture a
// variable. Runtime type checking resolves the same monomorphic dictionaries.
func propertyTermSource(t *testing.T, e *evaluator, value *term) string {
	t.Helper()
	if value.kind == "match" {
		return propertyMatchSource(t, e, value)
	}
	args := make([]string, len(value.args))
	for i, arg := range value.args {
		args[i] = propertyTermSource(t, e, arg)
	}
	switch value.kind {
	case "var", "bind":
		return propertyVariable(value)
	case "wild":
		return "_"
	case "int", "float", "bool", "string", "unit":
		return propertyLiteral(t, e, value)
	case "tuple":
		return "(" + strings.Join(args, ", ") + ")"
	case "map":
		return propertyMapSource(t, e, value, propertyTermSource)
	case "insert":
		return propertyMapReference(t, e, "MapInsert") + "(" + strings.Join(args, ", ") + ", func(a, b) => compare(a, b))"
	case "mapop":
		if len(value.dicts) != 0 {
			args = append(args, "func(a, b) => compare(a, b)")
		}
		return propertyMapReference(t, e, value.name) + "(" + strings.Join(args, ", ") + ")"
	case "mapfrom":
		// The accumulator is the left-fold prefix, not a right-fold tail.
		return "fold(" + args[1] + ", " + args[0] + ", func(acc, pair) => " + propertyMapReference(t, e, "MapInsert") + "(acc, pair.0, pair.1, func(a, b) => compare(a, b)))"
	case "ctor":
		name := propertyReference(e, value.ctor.Name)
		if len(args) == 0 {
			return name
		}
		if value.ctor.FieldNames != nil {
			for i, field := range value.ctor.FieldNames {
				args[i] = field + ": " + args[i]
			}
			return name + "{" + strings.Join(args, ", ") + "}"
		}
		return name + "(" + strings.Join(args, ", ") + ")"
	case "sum":
		return "(" + strings.Join(args, " + ") + ")"
	case "product":
		factors := []string{propertyLiteral(t, e, integer(value.number))}
		for i, arg := range args {
			for range value.indexes[i] {
				factors = append(factors, arg)
			}
		}
		return "(" + strings.Join(factors, " * ") + ")"
	case "binary", "ring":
		return "(" + args[0] + " " + value.name + " " + args[1] + ")"
	case "unary":
		return "(" + value.name + args[0] + ")"
	case "if":
		return "(if " + args[0] + " { " + args[1] + " } else { " + args[2] + " })"
	case "lambda":
		return "(func(" + strings.Join(args[:len(args)-1], ", ") + ") => " + args[len(args)-1] + ")"
	case "call":
		return "(" + args[0] + ")(" + strings.Join(args[1:], ", ") + ")"
	case "global", "method":
		return propertyReference(e, value.name)
	case "primitive":
		switch strings.TrimPrefix(value.name, "rt.") {
		case "EqInt", "EqString", "EqBool":
			return "(" + args[0] + " == " + args[1] + ")"
		case "LtInt", "LtString":
			return "(" + args[0] + " < " + args[1] + ")"
		case "ShowInt", "ShowBool", "ShowString":
			return "show(" + args[0] + ")"
		case "DebugString":
			return "debug(" + args[0] + ")"
		}
		for _, f := range e.p.Funcs {
			if f.Decl.Extern != nil && "rt."+f.Decl.Extern.Name == value.name && e.trustedExtern(f) {
				return propertyReference(e, f.Name) + "(" + strings.Join(args, ", ") + ")"
			}
		}
	case "order":
		switch value.name {
		case "<":
			return "(" + args[0] + " == LT)"
		case "<=":
			return "(" + args[0] + " != GT)"
		case ">":
			return "(" + args[0] + " == GT)"
		case ">=":
			return "(" + args[0] + " != LT)"
		}
	case "as":
		return args[0] + " @ " + args[1]
	case "field":
		field := value.name
		if field == "" {
			field = strconv.Itoa(value.index)
		}
		return "(" + args[0] + ")." + field
	case "update":
		if len(value.cases) != 0 || value.ctor == nil || value.ctor.FieldNames == nil {
			break
		}
		fields := make([]string, len(value.indexes))
		for i, index := range value.indexes {
			fields[i] = value.ctor.FieldNames[index] + ": " + args[i+1]
		}
		return "(" + args[0] + "){" + strings.Join(fields, ", ") + "}"
	}
	t.Fatalf("no residual source serializer for %s: %s", value.kind, e.display(value))
	return ""
}

// A residual match may have pruned constructors that are impossible for its
// known scrutinee. Factor those known constructors out rather than inventing an
// unreachable fallback to appease the source coverage checker.
func propertyMatchSource(t *testing.T, e *evaluator, value *term) string {
	arms := make([]string, len(value.arms))
	var scrutinee *term
	for i, arm := range value.arms {
		var values, patterns []*term
		bindings := make(map[int]*term)
		var flatten func(*term, *term)
		flatten = func(v, p *term) {
			known := v.kind == "ctor" || v.kind == "tuple" || v.kind == "int" || v.kind == "bool" || v.kind == "string" || v.kind == "unit"
			if !known {
				values = append(values, v)
				patterns = append(patterns, p)
				return
			}
			if p.kind == "as" {
				bindings[p.args[0].id] = v
				p = p.args[1]
			}
			if p.kind == "bind" {
				bindings[p.id] = v
				p = &term{kind: "wild"}
			}
			for j, field := range v.args {
				nested := p
				if p.kind != "wild" {
					nested = p.args[j]
				}
				flatten(field, nested)
			}
		}
		flatten(value.args[0], arm.pattern)
		pattern := &term{kind: "wild"}
		switch len(values) {
		case 0:
			scrutinee = boolean(true)
		case 1:
			scrutinee, pattern = values[0], patterns[0]
		default:
			scrutinee, pattern = &term{kind: "tuple", args: values}, &term{kind: "tuple", args: patterns}
		}
		guard, body := propertySubstitute(arm.guard, bindings), propertySubstitute(arm.body, bindings)
		condition := ""
		if guard.kind != "bool" || !guard.flag {
			condition = " if " + propertyTermSource(t, e, guard)
		}
		arms[i] = propertyTermSource(t, e, pattern) + condition + " => " + propertyTermSource(t, e, body)
	}
	return "(match " + propertyTermSource(t, e, scrutinee) + " { " + strings.Join(arms, "; ") + " })"
}

func propertySubstitute(value *term, bindings map[int]*term) *term {
	if value.kind == "var" && bindings[value.id] != nil {
		return bindings[value.id]
	}
	result := *value
	result.args = make([]*term, len(value.args))
	for i, arg := range value.args {
		result.args[i] = propertySubstitute(arg, bindings)
	}
	result.arms = make([]normalArm, len(value.arms))
	for i, arm := range value.arms {
		result.arms[i] = normalArm{pattern: arm.pattern, guard: propertySubstitute(arm.guard, bindings), body: propertySubstitute(arm.body, bindings)}
	}
	return &result
}

func propertyReference(e *evaluator, name string) string {
	entry := e.p.Modules[len(e.p.Modules)-1]
	owner := e.p.Owners[name]
	for _, file := range entry.Files {
		for _, imp := range file.Imports {
			if imp.Resolved == owner {
				alias := imp.Alias
				if alias == "" {
					parts := strings.Split(imp.Path, "/")
					alias = parts[len(parts)-1]
				}
				return alias + "." + e.sourceName(name)
			}
		}
	}
	return e.sourceName(name)
}

func TestTacticProofAssumptions(t *testing.T) {
	e := newEvaluator(loadFixture(t, "tactics_forms.gt"))
	law, err := e.extractLaw("expression")
	if err != nil {
		t.Fatal(err)
	}
	var checked bool
	for _, variant := range propertyProofVariants(t, e, law, 0) {
		if !strings.Contains(variant.name, "/match=0.0") {
			continue
		}
		checked = true
		e.assumptions = nil
		for _, test := range []struct {
			flag  bool
			field int64
			want  bool
		}{{true, 1, true}, {true, 2, false}, {false, 1, false}} {
			clear(e.solutions)
			for i, param := range variant.params {
				con, _ := typeHead(param.Type)
				if con == types.Bool {
					e.solutions[variant.vars[i].id] = boolean(test.flag)
				} else {
					e.solutions[variant.vars[i].id] = integer(test.field)
				}
			}
			holds, err := propertyAssumptionsHold(e, variant.assumptions)
			if err != nil || holds != test.want {
				t.Fatalf("flag=%t field=%d: holds=%v err=%v", test.flag, test.field, holds, err)
			}
		}
	}
	if !checked {
		t.Fatal("expression-match partial goal was not visited")
	}
}

func TestTacticProofRequiresSatisfyingSubstitution(t *testing.T) {
	e := newEvaluator(loadFixture(t, "tactics/tactics_no_samples.gt"))
	law, err := e.extractLaw("noSamples")
	if err != nil {
		t.Fatal(err)
	}
	var checked bool
	for _, variant := range propertyProofVariants(t, e, law, 0) {
		if !strings.Contains(variant.name, "/match=1.0") {
			continue
		}
		checked = true
		e.assumptions = nil
		satisfied := 0
		for _, flag := range []bool{false, true} {
			clear(e.solutions)
			for _, variable := range variant.vars {
				e.solutions[variable.id] = boolean(flag)
			}
			holds, err := propertyAssumptionsHold(e, variant.assumptions)
			if err != nil {
				t.Fatal(err)
			}
			if holds {
				satisfied++
			}
		}
		if err := propertyRequireSamples(variant.name, satisfied); err == nil {
			t.Fatal("an expression arm with no satisfying substitution must fail the oracle")
		}
	}
	if !checked {
		t.Fatal("unreachable but uncertified expression arm was not visited")
	}
}
