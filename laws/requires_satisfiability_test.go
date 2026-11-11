package laws

import (
	"math"
	"reflect"
	"strings"
	"testing"

	"github.com/reoring/go4t/types"
)

func TestRequiresUnsatisfiable(t *testing.T) {
	p := loadFixture(t, "requires_satisfiability/contradictions.gt")
	got := diagnosticsByLaw(t, p)
	want := map[string]string{
		"v1":        "7 < a is false given the conditions before it",
		"v3":        "b < a is false given the conditions before it",
		"never":     "false is false",
		"reflexive": "false is false",
		"above":     "9223372036854775807 < a is false",
		"below":     "a < -9223372036854775808 is false",
		"unfolded":  "now < t is false given the conditions before it",
	}
	if len(got) != len(want) {
		t.Fatalf("diagnostics = %v", got)
	}
	for name, text := range want {
		d := got[name]
		condition := p.Laws[name].Decl.Requires
		if d == nil || d.Msg != "law "+name+": requires can never hold: "+text || d.Pos != condition.GetPos() || d.End != condition.GetEnd() {
			t.Errorf("%s: diagnostic = %+v", name, d)
		}
	}
	if len(p.Warnings) != 0 {
		t.Fatalf("rejected preconditions warned: %v", p.Warnings)
	}
}

func TestRequiresWitnessDiagnostics(t *testing.T) {
	p := loadFixture(t, "requires_satisfiability/witness.gt")
	first := diagnosticsByLaw(t, p)
	if len(first) != 1 || first["failed"] == nil || !strings.HasSuffix(first["failed"].Msg, "no arguments satisfying requires were found in 128 candidates: no counterexample could be tried") {
		t.Fatalf("diagnostics = %v", first)
	}
	if len(p.Warnings) != 1 {
		t.Fatalf("warnings = %v", p.Warnings)
	}
	warning := p.Warnings[0]
	condition := p.Laws["vacuous"].Decl.Requires
	if warning.Msg != "law vacuous: no arguments satisfying requires were found in 128 candidates; the law may say nothing" || warning.File == "" || warning.Pos != condition.GetPos() || warning.End != condition.GetEnd() {
		t.Fatalf("warning = %+v", warning)
	}
	second := diagnosticsByLaw(t, p)
	if !reflect.DeepEqual(first, second) || len(p.Warnings) != 1 || p.Warnings[0] != warning {
		t.Fatal("repeated checking changed diagnostics or duplicated warnings")
	}
}

func TestRequiresWitnessSeededPrefix(t *testing.T) {
	p := loadFixture(t, "requires_satisfiability/witness.gt")
	for _, name := range []string{"literal", "range", "stringLiteral"} {
		e := newEvaluator(p)
		law, err := e.extractLaw(name)
		if err != nil {
			t.Fatal(err)
		}
		result := e.searchCandidates(law, true)
		if !result.witness || result.attempts > counterexampleEdges {
			t.Fatalf("%s: no prefix witness: %+v", name, result)
		}
		if again := e.searchCandidates(law, true); again != result {
			t.Fatalf("%s: nondeterministic search: %+v vs %+v", name, result, again)
		}
	}
}

func TestRequiresWitnessLiteralBounds(t *testing.T) {
	p := loadSource(t, `package main
law endpoints(a Int, s String) requires a == 9223372036854775807 || a == (-9223372036854775807 - 1) || s == "seed" { a == a }
proof endpoints(a, s) { refl }
func main() IO[Unit] { ioPure(unit()) }
`)
	e := newEvaluator(p)
	law, err := e.extractLaw("endpoints")
	if err != nil {
		t.Fatal(err)
	}
	seeds := e.preconditionLiterals(law)
	var ints []int64
	for _, seed := range seeds["int"] {
		ints = append(ints, seed.number)
	}
	if want := []int64{math.MaxInt64, math.MaxInt64 - 1, math.MinInt64, math.MinInt64 + 1}; !reflect.DeepEqual(ints, want) {
		t.Fatalf("literal neighbors = %v, want %v", ints, want)
	}
	g := e.candidateGenerator(types.Int, counterexampleDepth, make(map[candidateKey]*candidateGenerator), seeds)
	seen := make(map[int64]bool)
	for _, atom := range g.atoms {
		if seen[atom.number] {
			t.Fatalf("duplicate edge: %d", atom.number)
		}
		seen[atom.number] = true
	}
}

func TestRequiresSatisfiableBoundaries(t *testing.T) {
	for _, condition := range []string{
		"a >= 9223372036854775807",
		"a <= -9223372036854775807 - 1",
		"0 <= a && a <= 0",
		"a + 1 < a",
	} {
		t.Run(condition, func(t *testing.T) {
			p := loadSource(t, "package main\nlaw boundary(a Int) requires "+condition+" { a == a }\nproof boundary(a) { refl }\nfunc main() IO[Unit] { ioPure(unit()) }\n")
			if errors := CheckProgram(p); len(errors) != 0 || len(p.Warnings) != 0 {
				t.Fatalf("satisfiable boundary: errors=%v warnings=%v", errors, p.Warnings)
			}
		})
	}
}

func TestRequiresWitnessWithWeakProof(t *testing.T) {
	p := loadSource(t, `package main
type Choice = First | Second(Int)
law weak(a Int, c Choice) requires a == 123456 { (match c { First => true; Second(_) => true }) == true }
proof weak(a, c) { refl }
func main() IO[Unit] { ioPure(unit()) }
`)
	errors := CheckProgram(p)
	if len(errors) != 1 || !strings.HasSuffix(errors[0].Msg, "no counterexample in 128 candidates: the law may hold; this proof needs a case split, rewrite or induction") || len(p.Warnings) != 0 {
		t.Fatalf("admitted candidates: errors=%v warnings=%v", errors, p.Warnings)
	}
}

func TestRequiresWitnessSkippedType(t *testing.T) {
	p := loadSource(t, `package main
type Opaque = Opaque(Float)
law opaque(a Opaque) requires true { a == a }
proof opaque(a) { refl }
func main() IO[Unit] { ioPure(unit()) }
`)
	if errors := CheckProgram(p); len(errors) != 0 || len(p.Warnings) != 1 || p.Warnings[0].Msg != "law opaque: no arguments satisfying requires were found in 0 candidates; the law may say nothing" {
		t.Fatalf("unsupported witness type: errors=%v warnings=%v", errors, p.Warnings)
	}
}
