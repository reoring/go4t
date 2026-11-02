package laws

import (
	"sort"
	"strconv"
	"strings"

	"go4t/types"
)

// A term is private to one proof. Checked source nodes and annotations stay
// shared and immutable; only symbolic terms contain fresh variable identities.
type term struct {
	kind, name string
	id, arity, index int
	number int64
	flag bool
	ctor *types.Ctor
	args []*term
	arms []normalArm
	indexes []int
	cases []types.RecordCase
	closure *closure
	dicts []*dictionary
}

type normalArm struct {
	pattern, guard, body *term
}

func integer(n int64) *term { return &term{kind: "int", number: n} }
func text(s string) *term { return &term{kind: "string", name: s} }
func boolean(b bool) *term { return &term{kind: "bool", flag: b} }

// assume receives a normalized Bool term. Free IDs must still agree; only
// binders may be alpha-renamed. In particular this is not an arithmetic solver.
func (e *evaluator) assume(value *term) *term {
	for a := e.assumptions; a != nil; a = a.parent {
		if alphaEqual(value, a.condition, nil) {
			return a.value
		}
	}
	return value
}

// normal reads back closures under fresh binders. Delaying this until readback
// is essential: beta reduction of a recursive function on concrete arguments
// must not first try to unfold it on free arguments.
func (e *evaluator) normal(value *term) (*term, error) {
	if err := e.step(); err != nil {
		return nil, err
	}
	refinedCall := value.kind == "call" && len(e.solutions) != 0 && e.hasSolution(value)
	switch value.kind {
	case "var":
		if solution := e.solutions[value.id]; solution != nil {
			return e.normal(solution)
		}
	case "closure", "global", "method", "ctorfn":
		args := make([]*term, value.arity)
		binders := make([]*term, value.arity+1)
		for i := range args {
			name := "a"+strconv.Itoa(i)
			if value.closure != nil && value.closure.params[i].Name != "" {
				name = value.closure.params[i].Name
			}
			args[i] = e.fresh(name)
			binders[i] = &term{kind: "bind", id: args[i].id, name: name}
		}
		body, err := e.apply(value, args)
		if err != nil {
			return nil, err
		}
		body, err = e.normal(body)
		if err != nil {
			return nil, err
		}
		binders[value.arity] = body
		return &term{kind: "lambda", args: binders}, nil
	}
	var args []*term
	for i, arg := range value.args {
		normal, err := e.normal(arg)
		if err != nil {
			return nil, err
		}
		if normal != arg && args == nil {
			args = append([]*term(nil), value.args...)
		}
		if args != nil {
			args[i] = normal
		}
	}
	var arms []normalArm
	for i, arm := range value.arms {
		guard, err := e.normal(arm.guard)
		if err != nil {
			return nil, err
		}
		body, err := e.normal(arm.body)
		if err != nil {
			return nil, err
		}
		if (guard != arm.guard || body != arm.body) && arms == nil {
			arms = append([]normalArm(nil), value.arms...)
		}
		if arms != nil {
			arms[i] = normalArm{pattern: arm.pattern, guard: guard, body: body}
		}
	}
	changed := args != nil || arms != nil
	if changed {
		result := *value
		if args != nil {
			result.args = args
		}
		if arms != nil {
			result.arms = arms
		}
		value = &result
	}
	switch value.kind {
	case "ring":
		return e.ring(value.name, value.args[0], value.args[1])
	case "unary":
		if value.name == "-" {
			return e.ring("-", integer(0), value.args[0])
		}
	case "sum", "product":
		if changed {
			return e.rebuildPolynomial(value)
		}
	case "call":
		if changed && refinedCall && value.args[0].kind == "opaque" {
			head := *value.args[0]
			head.kind = "global"
			result, err := e.apply(&head, value.args[1:])
			if err != nil {
				return nil, err
			}
			return e.normal(result)
		}
	}
	return e.resume(value)
}

func copyBindings(bound map[int]int) map[int]int {
	copy := make(map[int]int, len(bound))
	for left, right := range bound {
		copy[left] = right
	}
	return copy
}

// Pairing fresh binder IDs is equivalent to comparing de Bruijn indices. Free
// variables must retain their original identity, including beneath lambdas.
func alphaEqual(a, b *term, bound map[int]int) bool {
	if a.kind != b.kind {
		return false
	}
	if a.kind == "var" {
		if right, ok := bound[a.id]; ok {
			return right == b.id
		}
		return a.id == b.id
	}
	if a.kind == "bind" {
		bound[a.id] = b.id
		return true
	}
	if a.kind == "lambda" {
		bound = copyBindings(bound)
	}
	if a.kind != "field" && a.name != b.name {
		return false
	}
	if a.ctor != b.ctor || a.number != b.number || a.flag != b.flag || a.index != b.index || len(a.args) != len(b.args) || len(a.arms) != len(b.arms) || len(a.indexes) != len(b.indexes) || len(a.cases) != len(b.cases) {
		return false
	}
	if !sameDictionaries(a.dicts, b.dicts) {
		return false
	}
	if a.kind == "sum" || a.kind == "product" {
		// Fresh binder IDs can change the local sort order. Compare the
		// canonical monomials as a multiset under the current alpha pairing.
		used := make([]bool, len(b.args))
		for i, arg := range a.args {
			found := false
			for j, other := range b.args {
				if used[j] || a.kind == "product" && a.indexes[i] != b.indexes[j] {
					continue
				}
				if alphaEqual(arg, other, bound) {
					used[j], found = true, true
					break
				}
			}
			if !found {
				return false
			}
		}
		return true
	}
	for i, index := range a.indexes {
		if index != b.indexes[i] {
			return false
		}
	}
	for i, c := range a.cases {
		other := b.cases[i]
		if c.Ctor != other.Ctor || len(c.Indexes) != len(other.Indexes) {
			return false
		}
		for j, index := range c.Indexes {
			if index != other.Indexes[j] {
				return false
			}
		}
	}
	for i, arg := range a.args {
		if !alphaEqual(arg, b.args[i], bound) {
			return false
		}
	}
	for i, arm := range a.arms {
		other := b.arms[i]
		scope := copyBindings(bound)
		if !alphaEqual(arm.pattern, other.pattern, scope) || !alphaEqual(arm.guard, other.guard, scope) || !alphaEqual(arm.body, other.body, scope) {
			return false
		}
	}
	return true
}

func (e *evaluator) sourceName(name string) string {
	if source := e.p.ModuleNames[name]; source != "" {
		return source
	}
	return name
}

// displayComparison recognizes only the residuals emitted by the prelude
// primitive Eq/Ord instances. This is diagnostic sugar, not normalization:
// neither the terms nor the equality/assumption machinery are changed.
func (e *evaluator) displayComparison(t *term) (op string, left, right *term) {
	if t.kind == "primitive" && len(t.args) == 2 {
		switch t.name {
		case "rt.EqInt", "rt.EqString", "rt.EqBool":
			return "==", t.args[0], t.args[1]
		case "rt.LtInt", "rt.LtString":
			return "<", t.args[0], t.args[1]
		}
	}
	if t.kind == "unary" && t.name == "!" && len(t.args) == 1 {
		if op, a, b := e.displayComparison(t.args[0]); op == "==" {
			return "!=", a, b
		}
	}
	if t.kind != "order" || len(t.args) != 1 {
		return "", nil, nil
	}
	switch t.name {
	case "<", "<=", ">", ">=":
	default:
		return "", nil, nil
	}
	c := t.args[0]
	if c.kind != "if" || len(c.args) != 3 {
		return "", nil, nil
	}
	test, yes, no := c.args[0], c.args[1], c.args[2]
	if test.kind != "primitive" || len(test.args) != 2 || no.kind != "if" || len(no.args) != 3 {
		return "", nil, nil
	}
	a, b := test.args[0], test.args[1]
	switch test.name {
	case "rt.LtInt", "rt.LtString":
		eq := no.args[0]
		eqName := "rt.EqInt"
		if test.name == "rt.LtString" {
			eqName = "rt.EqString"
		}
		if eq.kind == "primitive" && eq.name == eqName && len(eq.args) == 2 &&
			alphaEqual(a, eq.args[0], nil) && alphaEqual(b, eq.args[1], nil) &&
			e.displayOrdering(yes, "LT") && e.displayOrdering(no.args[1], "EQ") && e.displayOrdering(no.args[2], "GT") {
			return t.name, a, b
		}
	case "rt.EqBool":
		if alphaEqual(a, no.args[0], nil) &&
			e.displayOrdering(yes, "EQ") && e.displayOrdering(no.args[1], "GT") && e.displayOrdering(no.args[2], "LT") {
			return t.name, a, b
		}
	}
	return "", nil, nil
}

func (e *evaluator) displayOrdering(t *term, name string) bool {
	return t.kind == "ctor" && t.ctor != nil && len(t.args) == 0 &&
		t.ctor == e.ctors[name]
}

// A comparison already delimits its operands. Render canonical sums without
// redundant parentheses, in source-name order rather than fresh-ID order.
func (e *evaluator) displayComparisonOperand(t *term) string {
	if t.kind != "sum" {
		return e.display(t)
	}
	args := make([]string, len(t.args))
	for i, arg := range t.args {
		args[i] = e.display(arg)
	}
	sort.Strings(args)
	return strings.Join(args, " + ")
}

func (e *evaluator) display(t *term) string {
	if op, left, right := e.displayComparison(t); op != "" {
		return "("+e.displayComparisonOperand(left)+" "+op+" "+e.displayComparisonOperand(right)+")"
	}
	args := make([]string, len(t.args))
	for i, arg := range t.args {
		args[i] = e.display(arg)
	}
	switch t.kind {
	case "var", "bind":
		return t.name+"#"+strconv.Itoa(t.id)
	case "wild":
		return "_"
	case "int":
		return strconv.FormatInt(t.number, 10)
	case "bool":
		return strconv.FormatBool(t.flag)
	case "string":
		return strconv.Quote(t.name)
	case "unit":
		return "unit()"
	case "ctor":
		name := e.sourceName(t.ctor.Name)
		if len(args) == 0 {
			return name
		}
		return name+"("+strings.Join(args, ", ")+")"
	case "tuple":
		return "("+strings.Join(args, ", ")+")"
	case "lambda":
		return "func("+strings.Join(args[:len(args)-1], ", ")+") => "+args[len(args)-1]
	case "sum":
		return "("+strings.Join(args, " + ")+")"
	case "product":
		for i, power := range t.indexes {
			if power != 1 {
				args[i] += "^"+strconv.Itoa(power)
			}
		}
		if t.number != 1 {
			args = append([]string{strconv.FormatInt(t.number, 10)}, args...)
		}
		return "("+strings.Join(args, " * ")+")"
	case "opaque":
		return e.sourceName(t.name)
	case "binary":
		return "("+args[0]+" "+t.name+" "+args[1]+")"
	case "unary":
		return "("+t.name+args[0]+")"
	case "if":
		return "if "+args[0]+" { "+args[1]+" } else { "+args[2]+" }"
	case "as":
		return args[0]+" @ "+args[1]
	case "call":
		return args[0]+"("+strings.Join(args[1:], ", ")+")"
	case "field":
		field := t.name
		if field == "" {
			field = strconv.Itoa(t.index)
		}
		return args[0]+"."+field
	case "update":
		fields := make([]string, len(args)-1)
		for i, arg := range args[1:] {
			fields[i] = strconv.Itoa(t.indexes[i])+": "+arg
		}
		return args[0]+"{"+strings.Join(fields, ", ")+"}"
	case "match":
		arms := make([]string, len(t.arms))
		for i, arm := range t.arms {
			guard := ""
			if arm.guard.kind != "bool" || !arm.guard.flag {
				guard = " if "+e.display(arm.guard)
			}
			arms[i] = e.display(arm.pattern)+guard+" => "+e.display(arm.body)
		}
		return "match "+args[0]+" { "+strings.Join(arms, "; ")+" }"
	default:
		return t.name+"("+strings.Join(args, ", ")+")"
	}
}
