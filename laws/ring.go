package laws

import (
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"
)

// Polynomials are sparse sums of products. Product.number is an int64
// coefficient (Go arithmetic wraps modulo 2^64); args are ordered atoms and
// indexes are their positive powers. Constants and unit products are unboxed.
// Only checked Int arithmetic enters ring, never String concatenation.
type factor struct {
	atom *term
	key string
	power int
}

type monomial struct {
	coefficient int64
	factors []factor
}

func polynomial(t *term) []monomial {
	switch t.kind {
	case "int":
		if t.number == 0 {
			return nil
		}
		return []monomial{{coefficient: t.number}}
	case "sum":
		result := make([]monomial, 0, len(t.args))
		for _, arg := range t.args {
			result = append(result, polynomial(arg)...)
		}
		return result
	case "product":
		m := monomial{coefficient: t.number, factors: make([]factor, len(t.args))}
		for i, atom := range t.args {
			m.factors[i] = factor{atom: atom, key: termKey(atom), power: t.indexes[i]}
		}
		return []monomial{m}
	default:
		return []monomial{{coefficient: 1, factors: []factor{{atom: t, key: termKey(t), power: 1}}}}
	}
}

func multiply(a, b monomial) monomial {
	m := monomial{coefficient: a.coefficient*b.coefficient, factors: make([]factor, 0, len(a.factors)+len(b.factors))}
	for i, j := 0, 0; i < len(a.factors) || j < len(b.factors); {
		switch {
		case j == len(b.factors) || i < len(a.factors) && a.factors[i].key < b.factors[j].key:
			m.factors = append(m.factors, a.factors[i])
			i++
		case i == len(a.factors) || b.factors[j].key < a.factors[i].key:
			m.factors = append(m.factors, b.factors[j])
			j++
		default:
			f := a.factors[i]
			f.power += b.factors[j].power
			m.factors = append(m.factors, f)
			i++
			j++
		}
	}
	return m
}

func monomialKey(m monomial) string {
	var key strings.Builder
	for _, f := range m.factors {
		fmt.Fprintf(&key, "%d:%s^%d;", len(f.key), f.key, f.power)
	}
	return key.String()
}

func (e *evaluator) ring(op string, a, b *term) (*term, error) {
	left, right := polynomial(a), polynomial(b)
	terms := make(map[string]monomial)
	add := func(m monomial) error {
		if err := e.step(); err != nil {
			return err
		}
		key := monomialKey(m)
		m.coefficient += terms[key].coefficient
		if m.coefficient == 0 {
			delete(terms, key)
		} else {
			terms[key] = m
		}
		return nil
	}
	if op == "*" {
		for _, a := range left {
			for _, b := range right {
				if err := add(multiply(a, b)); err != nil {
					return nil, err
				}
			}
		}
	} else {
		for _, m := range left {
			if err := add(m); err != nil {
				return nil, err
			}
		}
		for _, m := range right {
			if op == "-" {
				m.coefficient = -m.coefficient
			}
			if err := add(m); err != nil {
				return nil, err
			}
		}
	}
	keys := make([]string, 0, len(terms))
	for key := range terms {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	args := make([]*term, 0, len(keys))
	for _, key := range keys {
		m := terms[key]
		var t *term
		switch {
		case len(m.factors) == 0:
			t = integer(m.coefficient)
		case len(m.factors) == 1 && m.factors[0].power == 1 && m.coefficient == 1:
			t = m.factors[0].atom
		default:
			t = &term{kind: "product", number: m.coefficient, args: make([]*term, len(m.factors)), indexes: make([]int, len(m.factors))}
			for i, f := range m.factors {
				t.args[i], t.indexes[i] = f.atom, f.power
			}
		}
		args = append(args, t)
	}
	if len(args) == 0 {
		return integer(0), nil
	}
	if len(args) == 1 {
		return args[0], nil
	}
	return &term{kind: "sum", args: args}, nil
}

// The key is structural, not display text: source names can collide and bound
// variable IDs must not influence atom identity. Free variables retain IDs.
func termKey(t *term) string {
	var out strings.Builder
	writeTermKey(&out, t, make(map[int]int))
	return out.String()
}

func writeTermKey(out *strings.Builder, t *term, bound map[int]int) {
	out.WriteString(t.kind)
	out.WriteByte('(')
	if t.kind == "lambda" {
		bound = copyBindings(bound)
	}
	if t.kind == "sum" || t.kind == "product" {
		fmt.Fprintf(out, "%d:", t.number)
		keys := make([]string, len(t.args))
		for i, arg := range t.args {
			var key strings.Builder
			writeTermKey(&key, arg, bound)
			if t.kind == "product" {
				fmt.Fprintf(&key, "^%d", t.indexes[i])
			}
			keys[i] = key.String()
		}
		sort.Strings(keys)
		for _, key := range keys {
			fmt.Fprintf(out, "%d:%s", len(key), key)
		}
		out.WriteByte(')')
		return
	}
	switch t.kind {
	case "var":
		if index, ok := bound[t.id]; ok {
			fmt.Fprintf(out, "b%020d", index)
		} else {
			fmt.Fprintf(out, "f%020d", t.id)
		}
	case "bind":
		bound[t.id] = len(bound)
	case "float":
		fmt.Fprintf(out, "%016x", math.Float64bits(t.float))
	default:
		if t.kind != "field" {
			out.WriteString(strconv.Quote(t.name))
		}
		fmt.Fprintf(out, ":%d:%t:%d:", t.number, t.flag, t.index)
		if t.ctor != nil {
			out.WriteString(strconv.Quote(t.ctor.Name))
		}
		fmt.Fprint(out, t.indexes)
		for _, c := range t.cases {
			out.WriteString(strconv.Quote(c.Ctor.Name))
			fmt.Fprint(out, c.Indexes)
		}
		for _, d := range t.dicts {
			writeDictionaryKey(out, d)
		}
	}
	for _, arg := range t.args {
		writeTermKey(out, arg, bound)
	}
	for _, arm := range t.arms {
		scope := copyBindings(bound)
		writeTermKey(out, arm.pattern, scope)
		writeTermKey(out, arm.guard, scope)
		writeTermKey(out, arm.body, scope)
	}
	out.WriteByte(')')
}

func writeDictionaryKey(out *strings.Builder, d *dictionary) {
	out.WriteString(strconv.Quote(d.instance.Class.Name))
	out.WriteString(strconv.Quote(d.instance.TypeCon.Name))
	out.WriteByte('[')
	for _, arg := range d.args {
		writeDictionaryKey(out, arg)
	}
	out.WriteByte(']')
}
