package laws

import (
	"errors"
	"math/rand/v2"
	"strings"
	"testing"

	"github.com/reoring/go4t/types"
)

func TestCounterexampleDiagnostics(t *testing.T) {
	p := loadFixture(t, "counterexample.gt")
	first, second := diagnosticsByLaw(t, p), diagnosticsByLaw(t, p)
	for _, tc := range []struct{ name, want string }{
		{"overflow", "counterexample: a = 9223372036854775807\n  LHS: false\n  RHS: true"},
		{"adt", "counterexample: c = Second(1)\n  LHS: false\n  RHS: true"},
		{"list", "counterexample: xs = Cons(0, Nil)\n  LHS: Cons(0, Nil)\n  RHS: Nil"},
		{"mapping", "counterexample: m = map{0: 0}\n  LHS: 1\n  RHS: 0"},
		{"weak", "no counterexample in 128 candidates: the law may hold; this proof needs a case split, rewrite or induction"},
		{"skipped", "counterexample search skipped: no candidates for Opaque"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			a, b := first[tc.name], second[tc.name]
			if a == nil || b == nil {
				t.Fatal("failed proof was accepted")
			}
			if !strings.Contains(a.Msg, tc.want) {
				t.Fatalf("missing %q:\n%s", tc.want, a.Msg)
			}
			if a.Msg != b.Msg {
				t.Fatalf("nondeterministic diagnostics:\n%s\n%s", a.Msg, b.Msg)
			}
		})
	}
	if err := first["passing"]; err != nil {
		t.Fatal(err)
	}
}

func TestCounterexampleIsolationAndAdmissibility(t *testing.T) {
	p := loadFixture(t, "counterexample.gt")
	for _, mode := range []string{"allowed", "rejected", "error", "fuel"} {
		t.Run(mode, func(t *testing.T) {
			e := newEvaluator(p)
			law, err := e.extractLaw("overflow")
			if err != nil {
				t.Fatal(err)
			}
			e.fuel, e.nextID, e.renormalizing = 17, 23, true
			assumptions := &assumption{condition: boolean(true), value: boolean(false)}
			solution := integer(99)
			goal := &rewriteRule{left: integer(0), right: integer(0)}
			e.assumptions, e.currentGoal = assumptions, goal
			e.solutions[7] = solution
			calls := 0
			e.admissible = func(env *environment) (bool, error) {
				calls++
				if env.lookup("a") == nil || env.lookup("renamed") != nil {
					t.Fatal("admissibility did not receive root law bindings")
				}
				if e.fuel != StepLimit || e.assumptions != nil || e.currentGoal != nil || len(e.solutions) != 0 || e.nextID != 0 || e.renormalizing {
					t.Fatal("proof state leaked into a candidate")
				}
				switch mode {
				case "rejected":
					return false, nil
				case "error":
					return false, errors.New("undecided precondition")
				case "fuel":
					e.fuel = 0
				}
				return true, nil
			}
			got := e.counterexample(law)
			if mode == "allowed" {
				if !strings.HasPrefix(got, "counterexample: a = 9223372036854775807\n") || calls != 5 {
					t.Fatalf("edge prefix: %d calls, %s", calls, got)
				}
			} else if !strings.HasPrefix(got, "no counterexample in 128 candidates:") || calls != counterexampleCandidates {
				t.Fatalf("discarded candidates: %d calls, %s", calls, got)
			}
			if e.fuel != 17 || e.nextID != 23 || !e.renormalizing || e.assumptions != assumptions || e.currentGoal != goal || len(e.solutions) != 1 || e.solutions[7] != solution {
				t.Fatal("counterexample search changed evaluator state")
			}
		})
	}
	e := newEvaluator(p)
	e.admissible = func(*environment) (bool, error) {
		t.Fatal("passing proof searched for counterexamples")
		return false, nil
	}
	law, err := e.extractLaw("passing")
	if err != nil {
		t.Fatal(err)
	}
	env, vars := e.proofEnvironment(law, p.Proofs[law.Name])
	if err := e.prove(law, p.Proofs[law.Name].Decl.Body, env, vars); err != nil {
		t.Fatal(err)
	}
}

func TestCounterexampleResiduals(t *testing.T) {
	p := loadFixture(t, "map_neutral.gt")
	e := newEvaluator(p)
	law, err := e.extractLaw("customComparator")
	if err != nil {
		t.Fatal(err)
	}
	if err := e.checkDependencies(p.Laws[law.Name], p.Proofs[law.Name]); err != nil {
		t.Fatal(err)
	}
	if got := e.counterexample(law); !strings.HasPrefix(got, "no counterexample in 128 candidates:") {
		t.Fatalf("a closed but neutral map operation was reported: %s", got)
	}
	for _, kind := range []string{"var", "primitive", "lambda", "call", "mapop", "float"} {
		if closedCandidate(&term{kind: "tuple", args: []*term{{kind: kind}}}) {
			t.Errorf("residual %s was considered closed data", kind)
		}
	}
}

func TestCounterexampleSkippedTypes(t *testing.T) {
	p := loadSource(t, `package main
type Mixed = Empty | Floating(Float)
type Loop = Loop(Loop)
law mixed(x Mixed, y Mixed) { x == y }
proof mixed(x, y) { refl }
law loop(x Loop, y Loop) { x == y }
proof loop(x, y) { refl }
func main() IO[Unit] { ioPure(unit()) }
`)
	errors := diagnosticsByLaw(t, p)
	for name, typ := range map[string]string{"mixed": "Mixed", "loop": "Loop"} {
		if err := errors[name]; err == nil || !strings.Contains(err.Msg, "counterexample search skipped: no candidates for "+typ) {
			t.Fatalf("%s: %v", name, err)
		}
	}
	e := newEvaluator(&types.Program{})
	for _, typ := range []types.Type{types.Float, &types.TCon{Name: "Handle"}, &types.TFunc{Params: []types.Type{types.Int}, Ret: types.Int}} {
		if e.candidateType(typ, make(map[string]bool)) {
			t.Fatalf("unsupported candidate type %s", typ)
		}
	}
}

func TestCounterexampleRandomDeterminism(t *testing.T) {
	p := loadSource(t, `package main
law falseInt(a Int) { a == a + 1 }
proof falseInt(a) { refl }
func main() IO[Unit] { ioPure(unit()) }
`)
	e := newEvaluator(p)
	law, err := e.extractLaw("falseInt")
	if err != nil {
		t.Fatal(err)
	}
	calls := 0
	e.admissible = func(env *environment) (bool, error) {
		calls++
		n := env.lookup("a").number
		return n < -3 && n != -9223372036854775808 || n > 3 && n != 9223372036854775807, nil
	}
	first := e.counterexample(law)
	if calls <= counterexampleEdges || !strings.HasPrefix(first, "counterexample: a = ") {
		t.Fatalf("random phase was not exercised: %d calls, %s", calls, first)
	}
	if second := e.counterexample(law); first != second {
		t.Fatalf("random search changed: %s vs %s", first, second)
	}
}

func TestCounterexampleCompositeGenerators(t *testing.T) {
	p := loadSource(t, `package main
type Bundle = Bundle{flag: Bool, text: String, value: Unit, pair: (Int, String), table: Map[(Int, Bool), String]}
law identity(x Bundle) { x == x }
proof identity(x) { refl }
func main() IO[Unit] { ioPure(unit()) }
`)
	e := newEvaluator(p)
	law, err := e.extractLaw("identity")
	if err != nil {
		t.Fatal(err)
	}
	typ := law.Params[0].Type
	if !e.candidateType(typ, make(map[string]bool)) {
		t.Fatal("supported composite type was skipped")
	}
	g := e.candidateGenerator(typ, counterexampleDepth, make(map[candidateKey]*candidateGenerator))
	if g == nil {
		t.Fatal("supported composite has no generator")
	}
	random := rand.New(rand.NewPCG(1, 0))
	for i := range counterexampleCandidates {
		var value *term
		if i < counterexampleEdges {
			value = g.value(e, i%g.edges, nil)
		} else {
			value = g.value(e, 0, random)
		}
		if !closedCandidate(value) {
			t.Fatalf("generator returned a neutral: %s", e.display(value))
		}
		table := value.args[4]
		if table.kind != "map" || len(table.args) > 6 {
			t.Fatalf("unbounded map candidate: %s", e.display(table))
		}
		for i := 2; i < len(table.args); i += 2 {
			if compareMapKeys(table.args[i-2], table.args[i]) >= 0 {
				t.Fatalf("noncanonical map candidate: %s", e.display(table))
			}
		}
	}
}
