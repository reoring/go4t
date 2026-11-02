package laws

import (
	"fmt"

	"go4t/ast"
)

type rewriteRule struct {
	left, right *term
}

func (e *evaluator) extractLaw(name string) (lawGoal, error) {
	info := e.p.Laws[name]
	if info == nil {
		return lawGoal{}, fmt.Errorf("unknown law %s", name)
	}
	law := lawGoal{Name: name, LHS: info.Decl.LHS, RHS: info.Decl.RHS}
	for i, param := range info.Decl.Params {
		typ := info.ParamTypes[i]
		if reason := e.unsupportedType(typ, true, make(map[string]bool)); reason != "" {
			return lawGoal{}, fmt.Errorf("parameter %s: %s", param.Name, reason)
		}
		law.Params = append(law.Params, parameter{Name: param.Name, Type: typ})
	}
	return law, nil
}

func (e *evaluator) equation(law lawGoal, env *environment) (*term, *term, error) {
	left, err := e.eval(law.LHS, env, nil)
	if err != nil {
		return nil, nil, err
	}
	left, err = e.normal(left)
	if err != nil {
		return nil, nil, err
	}
	right, err := e.eval(law.RHS, env, nil)
	if err != nil {
		return nil, nil, err
	}
	right, err = e.normal(right)
	return left, right, err
}

func (e *evaluator) rewriteRule(current lawGoal, proof *ast.TRewrite, env *environment, vars map[string]proofVar) (rewriteRule, error) {
	law, err := e.extractLaw(proof.Law.Name)
	if err != nil {
		return rewriteRule{}, err
	}
	bound := child(nil)
	decreased := false
	for i, expr := range proof.Args {
		value, err := e.condition(expr, vars)
		if err != nil {
			return rewriteRule{}, err
		}
		bound.values[law.Params[i].Name] = value
		if law.Name != current.Name || decreased {
			continue
		}
		original := env.lookup(current.Params[i].Name)
		normal, err := e.normal(original)
		if err != nil {
			return rewriteRule{}, err
		}
		if alphaEqual(value, normal, nil) {
			continue
		}
		// The decreasing argument must denote a pattern-bound descendant,
		// not a freshly constructed value or an arithmetically smaller Int.
		for _, v := range vars {
			for _, ancestor := range v.ancestors {
				if ancestor == original.id && alphaEqual(value, e.instantiate(v.value), nil) {
					decreased = true
					break
				}
			}
		}
		if !decreased {
			return rewriteRule{}, fmt.Errorf("rewrite %s: induction hypothesis must structurally decrease at the first changed argument", proof.Law.DisplayName())
		}
	}
	if law.Name == current.Name && !decreased {
		return rewriteRule{}, fmt.Errorf("rewrite %s: induction hypothesis must structurally decrease (nondecreasing self-reference is a cycle)", proof.Law.DisplayName())
	}
	left, right, err := e.equation(law, bound)
	if err != nil {
		return rewriteRule{}, err
	}
	if left.kind == "var" {
		return rewriteRule{}, fmt.Errorf("rewrite %s: normalized left side is a free variable", proof.Law.DisplayName())
	}
	return rewriteRule{left: left, right: right}, nil
}

func (e *evaluator) goal(law lawGoal, env *environment) (*term, *term, error) {
	if e.currentGoal == nil {
		return e.equation(law, env)
	}
	left, err := e.normal(e.currentGoal.left)
	if err != nil {
		return nil, nil, err
	}
	right, err := e.normal(e.currentGoal.right)
	return left, right, err
}

func (e *evaluator) replace(t, from, to *term) (*term, error) {
	if err := e.step(); err != nil {
		return nil, err
	}
	if alphaEqual(t, from, nil) {
		return to, nil
	}
	var args []*term
	for i, arg := range t.args {
		v, err := e.replace(arg, from, to)
		if err != nil {
			return nil, err
		}
		if v != arg && args == nil {
			args = append([]*term(nil), t.args...)
		}
		if args != nil {
			args[i] = v
		}
	}
	var arms []normalArm
	for i, arm := range t.arms {
		guard, err := e.replace(arm.guard, from, to)
		if err != nil {
			return nil, err
		}
		body, err := e.replace(arm.body, from, to)
		if err != nil {
			return nil, err
		}
		if (guard != arm.guard || body != arm.body) && arms == nil {
			arms = append([]normalArm(nil), t.arms...)
		}
		if arms != nil {
			arms[i] = normalArm{pattern: arm.pattern, guard: guard, body: body}
		}
	}
	if args == nil && arms == nil {
		return t, nil
	}
	result := *t
	if args != nil {
		result.args = args
	}
	if arms != nil {
		result.arms = arms
	}
	if result.kind == "sum" || result.kind == "product" {
		return e.rebuildPolynomial(&result)
	}
	return e.normal(&result)
}

func (e *evaluator) rebuildPolynomial(t *term) (*term, error) {
	result := integer(0)
	if t.kind == "product" {
		result = integer(t.number)
	}
	for i, arg := range t.args {
		count, op := 1, "+"
		if t.kind == "product" {
			count, op = t.indexes[i], "*"
		}
		for range count {
			var err error
			result, err = e.ring(op, result, arg)
			if err != nil {
				return nil, err
			}
		}
	}
	return result, nil
}

func sameDictionaries(a, b []*dictionary) bool {
	if len(a) != len(b) {
		return false
	}
	for i, d := range a {
		if d.instance != b[i].instance || !sameDictionaries(d.args, b[i].args) {
			return false
		}
	}
	return true
}
