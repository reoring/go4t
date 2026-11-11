package laws

import (
	"fmt"
	"math"
	"math/rand/v2"
	"strings"

	"github.com/reoring/go4t/types"
)

const (
	counterexampleCandidates = 128
	counterexampleEdges      = 64
	counterexampleDepth      = 3
)

// counterexample checks the original equation, never a rewritten goal or a
// branch-local hypothesis. Save the whole evaluator so even an admissibility
// check consuming fuel or freshening binders cannot affect another diagnostic.
func (e *evaluator) counterexample(law lawGoal) string {
	saved := *e
	defer func() { *e = saved }()
	generators := make([]*candidateGenerator, len(law.Params))
	cache := make(map[candidateKey]*candidateGenerator)
	for i, param := range law.Params {
		if e.candidateType(param.Type, make(map[string]bool)) {
			generators[i] = e.candidateGenerator(param.Type, counterexampleDepth, cache)
		}
		if generators[i] == nil {
			return "counterexample search skipped: no candidates for " + e.sourceName(types.TypeString(param.Type))
		}
	}
	random := rand.New(rand.NewPCG(1, 0))
	bindings := child(nil)
	e.solutions = make(map[int]*term)
	for candidate := range counterexampleCandidates {
		e.fuel, e.nextID = StepLimit, 0
		e.assumptions, e.currentGoal, e.renormalizing = nil, nil, false
		clear(e.solutions)
		index := candidate
		for i, param := range law.Params {
			g := generators[i]
			if candidate < counterexampleEdges {
				bindings.values[param.Name] = g.value(e, index%g.edges, nil)
				index /= g.edges
			} else {
				bindings.values[param.Name] = g.value(e, 0, random)
			}
		}
		// A candidate outside the law's precondition says nothing about the law.
		if ok, err := e.preconditionHolds(law, bindings); err != nil || !ok {
			continue
		}
		if e.admissible != nil {
			if ok, err := e.admissible(bindings); err != nil || !ok {
				continue
			}
		}
		left, right, err := e.equation(law, bindings)
		if err != nil || !closedCandidate(left) || !closedCandidate(right) || alphaEqual(left, right, nil) {
			continue
		}
		shown := make([]string, len(law.Params))
		for i, param := range law.Params {
			shown[i] = param.Name + " = " + e.display(bindings.values[param.Name])
		}
		return fmt.Sprintf("counterexample: %s\n  LHS: %s\n  RHS: %s", strings.Join(shown, ", "), e.display(left), e.display(right))
	}
	return fmt.Sprintf("no counterexample in %d candidates: the law may hold; this proof needs a case split, rewrite or induction", counterexampleCandidates)
}

// A closed neutral is not a value. In particular, unsupported map operations,
// lambdas and Float-bearing opaque data are not evidence of a counterexample.
func closedCandidate(t *term) bool {
	if t == nil {
		return false
	}
	switch t.kind {
	case "int", "bool", "string", "unit", "ctor", "tuple", "map":
		for _, arg := range t.args {
			if !closedCandidate(arg) {
				return false
			}
		}
		return true
	default:
		return false
	}
}

// Reject an unsupported field even if another constructor could avoid it. The
// diagnostic must say that this type is not enumerated, not imply coverage of
// its Float/handle-bearing alternatives. Recursive data is inspected once.
func (e *evaluator) candidateType(typ types.Type, seen map[string]bool) bool {
	con, args := typeHead(typ)
	if con == nil {
		return false
	}
	switch con.Name {
	case "Int", "Bool", "String", "Unit":
		return true
	case "Map":
		return len(args) == 2 && candidateMapKey(args[0]) && e.candidateType(args[1], seen)
	}
	for _, arg := range args {
		if !e.candidateType(arg, seen) {
			return false
		}
	}
	if _, tuple := types.IsTuple(con); tuple {
		return true
	}
	data := e.p.DataTypes[con.Name]
	if data == nil {
		return false
	}
	if seen[con.Name] {
		return true
	}
	seen[con.Name] = true
	for _, ctor := range data.Ctors {
		for _, field := range sampleFields(data, args, ctor) {
			if !e.candidateType(field, seen) {
				return false
			}
		}
	}
	return true
}

func candidateMapKey(typ types.Type) bool {
	con, args := typeHead(typ)
	if con == nil {
		return false
	}
	switch con.Name {
	case "Int", "Bool", "String":
		return true
	}
	if _, tuple := types.IsTuple(con); !tuple {
		return false
	}
	for _, arg := range args {
		if !candidateMapKey(arg) {
			return false
		}
	}
	return true
}

type candidateGenerator struct {
	kind    string
	edges   int
	ctor    *types.Ctor
	atoms   []*term
	fields  []*candidateGenerator
	choices []*candidateGenerator
}

type candidateKey struct {
	typ   types.Type
	depth int
}

func (e *evaluator) candidateGenerator(typ types.Type, depth int, cache map[candidateKey]*candidateGenerator) *candidateGenerator {
	key := candidateKey{typ: types.Unalias(typ), depth: depth}
	if g, ok := cache[key]; ok {
		return g
	}
	g := &candidateGenerator{edges: 1}
	con, args := typeHead(typ)
	switch con.Name {
	case "Int":
		g.kind = "int"
		for _, n := range []int64{0, 1, -1, math.MinInt64, math.MaxInt64, 2, -2, 3, -3} {
			g.atoms = append(g.atoms, integer(n))
		}
	case "Bool":
		g.atoms = []*term{boolean(false), boolean(true)}
	case "String":
		g.kind = "string"
		for _, s := range []string{"", "a", "b", " ", "\n", "λ"} {
			g.atoms = append(g.atoms, text(s))
		}
	case "Unit":
		g.atoms = []*term{{kind: "unit"}}
	default:
		if depth < 0 {
			return nil
		}
		_, tuple := types.IsTuple(con)
		switch {
		case con.Name == "Map":
			g.kind, g.edges = "map", counterexampleEdges
			g.fields = e.candidateFields(args, depth-1, cache)
			if g.fields == nil {
				g = nil
			}
		case tuple:
			g.kind = "tuple"
			g.fields = e.candidateFields(args, depth-1, cache)
			if g.fields == nil {
				g = nil
			} else {
				g.edges = candidateProduct(g.fields)
			}
		default:
			g.kind = "choice"
			data := e.p.DataTypes[con.Name]
			for _, ctor := range data.Ctors {
				fields := e.candidateFields(sampleFields(data, args, ctor), depth-1, cache)
				if fields != nil {
					choice := &candidateGenerator{kind: "ctor", ctor: ctor, fields: fields, edges: candidateProduct(fields)}
					g.choices = append(g.choices, choice)
					g.edges = max(g.edges, choice.edges)
				}
			}
			if len(g.choices) == 0 {
				g = nil
			} else {
				g.edges = min(counterexampleEdges, g.edges*len(g.choices))
			}
		}
	}
	if g != nil && len(g.atoms) != 0 {
		g.edges = len(g.atoms)
	}
	cache[key] = g
	return g
}

func (e *evaluator) candidateFields(fields []types.Type, depth int, cache map[candidateKey]*candidateGenerator) []*candidateGenerator {
	result := make([]*candidateGenerator, len(fields))
	for i, field := range fields {
		result[i] = e.candidateGenerator(field, depth, cache)
		if result[i] == nil {
			return nil
		}
	}
	return result
}

func candidateProduct(fields []*candidateGenerator) int {
	n := 1
	for _, field := range fields {
		n = min(counterexampleEdges, n*field.edges)
	}
	return n
}

func (g *candidateGenerator) value(e *evaluator, index int, random *rand.Rand) *term {
	if len(g.atoms) != 0 {
		if random != nil {
			if random.IntN(2) == 0 {
				switch g.kind {
				case "int":
					return integer(int64(random.Uint64()))
				case "string":
					alphabet := []rune("abλ\n")
					s := make([]rune, random.IntN(7))
					for i := range s {
						s[i] = alphabet[random.IntN(len(alphabet))]
					}
					return text(string(s))
				}
			}
			index = random.IntN(len(g.atoms))
		}
		return g.atoms[index%len(g.atoms)]
	}
	if g.kind == "choice" {
		choice := index % len(g.choices)
		if random != nil {
			choice = random.IntN(len(g.choices))
		}
		return g.choices[choice].value(e, index/len(g.choices), random)
	}
	if g.kind == "map" {
		n := index % 4
		index /= 4
		if random != nil {
			n = random.IntN(4)
		}
		value := &term{kind: "map"}
		for i := range n {
			key := g.fields[0].value(e, (index+i)%g.fields[0].edges, random)
			part := g.fields[1].value(e, index%g.fields[1].edges, random)
			value = e.mapInsert(value, key, part, nil)
		}
		return value
	}
	args := make([]*term, len(g.fields))
	for i, field := range g.fields {
		args[i] = field.value(e, index%field.edges, random)
		index /= field.edges
	}
	return &term{kind: g.kind, ctor: g.ctor, args: args}
}
