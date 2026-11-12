package laws

import (
	"math"
	"math/big"
	"sort"
	"strings"
)

// Scalar comparison terms use only the existing trusted runtime primitives.
// In particular <= is !(b < a), not a subtraction that could overflow.
func scalarComparison(domain, op string, a, b *term) *term {
	switch op {
	case ">":
		a, b, op = b, a, "<"
	case ">=":
		a, b, op = b, a, "<="
	}
	name := "rt.Lt" + domain
	if op == "==" || op == "!=" {
		name = "rt.Eq" + domain
		if termKey(a) > termKey(b) {
			a, b = b, a
		}
	}
	if op == "<=" {
		a, b = b, a
	}
	value := &term{kind: "primitive", name: name, args: []*term{a, b}}
	if op == "<=" || op == "!=" {
		return unary("!", value)
	}
	return value
}

type orderFact struct {
	domain, op string
	a, b       *term
}

func comparisonFact(t *term) (orderFact, bool) {
	negated := false
	for t.kind == "unary" && t.name == "!" {
		negated = !negated
		t = t.args[0]
	}
	if t.kind != "primitive" || len(t.args) != 2 {
		return orderFact{}, false
	}
	f := orderFact{a: t.args[0], b: t.args[1]}
	switch t.name {
	case "rt.LtInt", "rt.LtString":
		f.op = "<"
	case "rt.EqInt", "rt.EqString":
		f.op = "=="
	default:
		return orderFact{}, false
	}
	f.domain = strings.TrimPrefix(strings.TrimPrefix(t.name, "rt.Lt"), "rt.Eq")
	if negated {
		f = f.negate()
	}
	return f, true
}

func (f orderFact) negate() orderFact {
	switch f.op {
	case "<":
		f.a, f.b, f.op = f.b, f.a, "<="
	case "<=":
		f.a, f.b, f.op = f.b, f.a, "<"
	case "==":
		f.op = "!="
	case "!=":
		f.op = "=="
	}
	return f
}

// Layer A never opens arithmetic: every normalized operand is an opaque atom.
// reach[i][j] is 0 (unknown), 1 (<=), or 2 (<). A strict edge in a path makes
// the whole path strict. Disequalities strengthen known non-strict edges.
type orderGraph struct {
	atoms []*term
	ids   map[string]int
	reach [][]uint8
	neq   [][2]int
}

func (g *orderGraph) add(t *term) int {
	key := termKey(t)
	if i, ok := g.ids[key]; ok {
		return i
	}
	i := len(g.atoms)
	g.ids[key] = i
	g.atoms = append(g.atoms, t)
	return i
}

func (e *evaluator) orderFacts(domain string) ([]orderFact, error) {
	var facts []orderFact
	// Re-read assumptions through constructor refinements, but never use an
	// assumption (or order closure) to establish that same assumption.
	e.renormalizing = true
	defer func() { e.renormalizing = false }()
	for a := e.assumptions; a != nil; a = a.parent {
		if a.value.kind != "bool" {
			continue
		}
		condition, err := e.normal(e.instantiate(a.condition))
		if err != nil {
			return nil, err
		}
		if f, ok := comparisonFact(condition); ok && f.domain == domain {
			if !a.value.flag {
				f = f.negate()
			}
			facts = append(facts, f)
		}
	}
	return facts, nil
}

func (e *evaluator) orderClosure(goal orderFact, facts []orderFact) (*orderGraph, error) {
	g := &orderGraph{ids: make(map[string]int)}
	g.add(goal.a)
	g.add(goal.b)
	for _, f := range facts {
		g.add(f.a)
		g.add(f.b)
	}
	// Ring atoms need literal bounds even when absent as whole operands.
	if goal.domain == "Int" {
		for _, t := range []*term{goal.a, goal.b} {
			for _, m := range polynomial(t) {
				for _, f := range m.factors {
					g.add(f.atom)
				}
			}
		}
	}
	n := len(g.atoms)
	g.reach = make([][]uint8, n)
	for i, a := range g.atoms {
		g.reach[i] = make([]uint8, n)
		g.reach[i][i] = 1
		for j, b := range g.atoms {
			if a.kind == "int" && b.kind == "int" && a.number < b.number ||
				a.kind == "string" && b.kind == "string" && a.name < b.name {
				g.reach[i][j] = 2
			}
		}
	}
	for _, f := range facts {
		i, j := g.ids[termKey(f.a)], g.ids[termKey(f.b)]
		switch f.op {
		case "<":
			g.reach[i][j] = 2
		case "<=":
			if g.reach[i][j] == 0 {
				g.reach[i][j] = 1
			}
		case "==":
			if g.reach[i][j] == 0 {
				g.reach[i][j] = 1
			}
			if g.reach[j][i] == 0 {
				g.reach[j][i] = 1
			}
		case "!=":
			g.neq = append(g.neq, [2]int{i, j})
		}
	}
	// A second closure is needed only when a disequality strengthens an edge.
	for {
		for k := range g.atoms {
			for i := range g.atoms {
				if g.reach[i][k] == 0 {
					continue
				}
				if err := e.step(); err != nil {
					return nil, err
				}
				for j := range g.atoms {
					if g.reach[k][j] == 0 {
						continue
					}
					strength := g.reach[i][k]
					if g.reach[k][j] > strength {
						strength = g.reach[k][j]
					}
					if strength > g.reach[i][j] {
						g.reach[i][j] = strength
					}
				}
			}
		}
		changed := false
		for _, pair := range g.neq {
			for _, edge := range [][2]int{pair, {pair[1], pair[0]}} {
				if g.reach[edge[0]][edge[1]] == 1 {
					g.reach[edge[0]][edge[1]] = 2
					changed = true
				}
			}
		}
		if !changed {
			break
		}
	}
	return g, nil
}

func (g *orderGraph) equal(i, j int) bool {
	return g.reach[i][j] != 0 && g.reach[j][i] != 0
}

func (g *orderGraph) entails(f orderFact) bool {
	i, j := g.ids[termKey(f.a)], g.ids[termKey(f.b)]
	switch f.op {
	case "<":
		return g.reach[i][j] == 2
	case "<=":
		return g.reach[i][j] != 0
	case "==":
		return g.equal(i, j)
	case "!=":
		if g.reach[i][j] == 2 || g.reach[j][i] == 2 {
			return true
		}
		for _, pair := range g.neq {
			if g.equal(i, pair[0]) && g.equal(j, pair[1]) ||
				g.equal(i, pair[1]) && g.equal(j, pair[0]) {
				return true
			}
		}
	}
	return false
}

type integerInterval struct{ lo, hi *big.Int }

func intInterval(lo, hi int64) integerInterval {
	return integerInterval{big.NewInt(lo), big.NewInt(hi)}
}
func (r integerInterval) fits() bool { return r.lo.IsInt64() && r.hi.IsInt64() && r.lo.Cmp(r.hi) <= 0 }
func (g *orderGraph) bounds(t *term) integerInterval {
	if t.kind == "int" {
		return intInterval(t.number, t.number)
	}
	r := intInterval(math.MinInt64, math.MaxInt64)
	i, ok := g.ids[termKey(t)]
	if !ok {
		return r
	}
	for j, literal := range g.atoms {
		if literal.kind != "int" {
			continue
		}
		if edge := g.reach[j][i]; edge != 0 {
			lo := big.NewInt(literal.number)
			if edge == 2 {
				lo.Add(lo, big.NewInt(1))
			}
			if lo.Cmp(r.lo) > 0 {
				r.lo = lo
			}
		}
		if edge := g.reach[i][j]; edge != 0 {
			hi := big.NewInt(literal.number)
			if edge == 2 {
				hi.Sub(hi, big.NewInt(1))
			}
			if hi.Cmp(r.hi) < 0 {
				r.hi = hi
			}
		}
	}
	return r
}

// impossibleOrder uses the existing int64 intervals only to reject a requires
// conjunct. It does not extend proof normalization: even endpoint decisions
// here must never turn a formerly rejected proof into an accepted one.
func (e *evaluator) impossibleOrder(t *term) (bool, error) {
	f, ok := comparisonFact(t)
	if !ok || f.domain != "Int" {
		return false, nil
	}
	facts, err := e.orderFacts(f.domain)
	if err != nil {
		return false, err
	}
	g, err := e.orderClosure(f, facts)
	if err != nil {
		return false, err
	}
	a, b := g.bounds(f.a), g.bounds(f.b)
	switch f.op {
	case "<":
		return a.lo.Cmp(b.hi) >= 0, nil
	case "<=":
		return a.lo.Cmp(b.hi) > 0, nil
	case "==":
		return a.lo.Cmp(b.hi) > 0 || b.lo.Cmp(a.hi) > 0, nil
	case "!=":
		return a.lo.Cmp(a.hi) == 0 && b.lo.Cmp(b.hi) == 0 && a.lo.Cmp(b.lo) == 0, nil
	}
	return false, nil
}

func intervalProduct(a, b integerInterval) integerInterval {
	values := []*big.Int{
		new(big.Int).Mul(a.lo, b.lo), new(big.Int).Mul(a.lo, b.hi),
		new(big.Int).Mul(a.hi, b.lo), new(big.Int).Mul(a.hi, b.hi),
	}
	r := integerInterval{values[0], values[0]}
	for _, v := range values[1:] {
		if v.Cmp(r.lo) < 0 {
			r.lo = v
		}
		if v.Cmp(r.hi) > 0 {
			r.hi = v
		}
	}
	return r
}

// zPolynomial keeps exact coefficients. Subtracting modulo 2^64 before the
// range check would be unsound (e.g. maxInt - minInt would become -1).
type zMonomial struct {
	coefficient *big.Int
	factors     []factor
}
type zPolynomial map[string]zMonomial

func zPoly(t *term) zPolynomial {
	p := make(zPolynomial)
	for _, m := range polynomial(t) {
		p[monomialKey(m)] = zMonomial{big.NewInt(m.coefficient), m.factors}
	}
	return p
}

func zSubtract(a, b zPolynomial) zPolynomial {
	p := make(zPolynomial, len(a)+len(b))
	for k, m := range a {
		p[k] = zMonomial{new(big.Int).Set(m.coefficient), m.factors}
	}
	for k, m := range b {
		c := new(big.Int).Neg(m.coefficient)
		if old, ok := p[k]; ok {
			c.Add(c, old.coefficient)
		}
		if c.Sign() == 0 {
			delete(p, k)
		} else {
			p[k] = zMonomial{c, m.factors}
		}
	}
	return p
}

func (g *orderGraph) wrapFree(p zPolynomial) bool {
	total := intInterval(0, 0)
	for _, m := range p {
		if !m.coefficient.IsInt64() {
			return false
		}
		r := integerInterval{new(big.Int).Set(m.coefficient), new(big.Int).Set(m.coefficient)}
		for _, f := range m.factors {
			bound := g.bounds(f.atom)
			if !bound.fits() {
				return false
			}
			for range f.power {
				r = intervalProduct(r, bound)
				if !r.fits() {
					return false
				}
			}
		}
		total.lo.Add(total.lo, r.lo)
		total.hi.Add(total.hi, r.hi)
	}
	return total.fits()
}

func (g *orderGraph) difference(f orderFact) (zPolynomial, bool) {
	p, q := zPoly(f.a), zPoly(f.b)
	d := zSubtract(q, p)
	return d, g.wrapFree(p) && g.wrapFree(q) && g.wrapFree(d)
}

func (e *evaluator) zTerm(p zPolynomial) (*term, error) {
	keys := make([]string, 0, len(p))
	for k := range p {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	result := integer(0)
	for _, k := range keys {
		m := p[k]
		value := integer(m.coefficient.Int64())
		for _, f := range m.factors {
			for range f.power {
				var err error
				value, err = e.ring("*", value, f.atom)
				if err != nil {
					return nil, err
				}
			}
		}
		var err error
		result, err = e.ring("+", result, value)
		if err != nil {
			return nil, err
		}
	}
	return result, nil
}

type nonnegative struct {
	p      zPolynomial
	strict bool
}

// At most two exact assumed summands, with coefficient one, plus a constant.
// Coefficients are compared over Z, never in the wrapping ring. Nonlinear
// polynomials may be range-checked/canonicalized, but are not decision inputs.
func linear(p zPolynomial) bool {
	for _, m := range p {
		if len(m.factors) > 1 || len(m.factors) == 1 && m.factors[0].power != 1 {
			return false
		}
	}
	return true
}

func nonnegativeConstant(p zPolynomial, strict bool, inheritedStrict bool) bool {
	for k := range p {
		if k != "" {
			return false
		}
	}
	c := new(big.Int)
	if m, ok := p[""]; ok {
		c = m.coefficient
	}
	return c.Sign() >= 0 && (!strict || inheritedStrict || c.Sign() > 0)
}

func (e *evaluator) combination(goal zPolynomial, strict bool, facts []nonnegative) (bool, error) {
	if nonnegativeConstant(goal, strict, false) {
		return true, nil
	}
	if !linear(goal) {
		return false, nil
	}
	for i, a := range facts {
		rest := zSubtract(goal, a.p)
		if nonnegativeConstant(rest, strict, a.strict) {
			return true, nil
		}
		for _, b := range facts[i:] {
			if err := e.step(); err != nil {
				return false, err
			}
			if nonnegativeConstant(zSubtract(rest, b.p), strict, a.strict || b.strict) {
				return true, nil
			}
		}
	}
	return false, nil
}

func (e *evaluator) order(t *term) (*term, error) {
	goal, ok := comparisonFact(t)
	if !ok || e.renormalizing {
		return t, nil
	}
	facts, err := e.orderFacts(goal.domain)
	if err != nil {
		return nil, err
	}
	g, err := e.orderClosure(goal, facts)
	if err != nil {
		return nil, err
	}
	if g.entails(goal) {
		return boolean(true), nil
	}
	if g.entails(goal.negate()) {
		return boolean(false), nil
	}
	if goal.domain != "Int" || goal.op == "!=" {
		return t, nil
	}
	d, ok := g.difference(goal)
	if !ok {
		return t, nil
	}
	var nonnegatives []nonnegative
	for _, f := range facts {
		if f.op != "<" && f.op != "<=" && f.op != "==" {
			continue
		}
		p, safe := g.difference(f)
		if !safe || !linear(p) {
			continue
		}
		nonnegatives = append(nonnegatives, nonnegative{p, f.op == "<"})
		if f.op == "==" {
			nonnegatives = append(nonnegatives, nonnegative{zSubtract(nil, p), false})
		}
	}
	if goal.op != "==" {
		if yes, err := e.combination(d, goal.op == "<", nonnegatives); err != nil || yes {
			return boolean(yes), err
		}
		if no, err := e.combination(zSubtract(nil, d), goal.op == "<=", nonnegatives); err != nil || no {
			return boolean(false), err
		}
	}
	value, err := e.zTerm(d)
	if err != nil {
		return nil, err
	}
	if value.kind == "int" {
		switch goal.op {
		case "<":
			return boolean(value.number > 0), nil
		case "<=":
			return boolean(value.number >= 0), nil
		case "==":
			return boolean(value.number == 0), nil
		}
	}
	return scalarComparison("Int", goal.op, integer(0), value), nil
}
