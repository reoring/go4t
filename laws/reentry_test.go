package laws

import (
	"fmt"
	"strings"
	"testing"
)

func TestReentry(t *testing.T) {
	p := loadFixture(t, "reentry.gt")
	errors := diagnosticsByLaw(t, p)
	for name := range p.Laws {
		if (errors[name] != nil) != (name == "symbolicTail") {
			t.Errorf("%s: unexpected result %v", name, errors[name])
		}
	}
	err := errors["symbolicTail"]
	assertNormalForms(t, err)
	for _, want := range []string{"stuck normal forms differ", "_firstEvictable(xs#"} {
		if !strings.Contains(err.Msg, want) {
			t.Errorf("missing %q: %v", want, err)
		}
	}
	if strings.Contains(err.Msg, "step limit") {
		t.Fatalf("symbolic recursion exhausted fuel: %v", err)
	}
}

func TestReentryCertifiedPositions(t *testing.T) {
	p := loadSource(t, `package main
 type Box[A] = Box(A) deriving Eq
 func left(n Int, xs List[Int]) Int { match xs { Nil => n; Cons(_, rest) => right(n, rest) } }
 func right(n Int, xs List[Int]) Int { match xs { Nil => n; Cons(_, rest) => left(n, rest) } }
 func count[A Eq](seed A, xs List[Int]) Int {
   match xs { Nil => if eq(seed, seed) { 1 } else { 0 }; Cons(_, rest) => count(seed, rest) }
 }
 func identity(n Int) Int { n }
 law positions(n Int, xs List[Int]) {
   (left(n, xs), count(Box(1), xs), count(Box(true), xs), identity(n)) ==
   (left(n, xs), count(Box(1), xs), count(Box(true), xs), identity(n))
 }
 proof positions(n, xs) { refl }
 func main() IO[Unit] { ioPure(unit()) }
 `)
	e := newEvaluator(p)
	if err := e.checkDependencies(p.Laws["positions"], p.Proofs["positions"]); err != nil {
		t.Fatal(err)
	}
	if len(e.decreasing) != 3 {
		t.Fatalf("certified globals = %v, want only left, right, count", e.decreasing)
	}
	for name, want := range map[string]int{"left": 1, "right": 1, "count": 2} {
		positions := e.decreasing[name]
		if len(positions) != want {
			t.Fatalf("%s: got %d certificates, want %d", name, len(positions), want)
		}
		for _, position := range positions {
			if position.position != 1 {
				t.Errorf("%s: decreasing position = %d, want 1", name, position.position)
			}
		}
	}
	positions := e.decreasing["count"]
	if sameDictionaries(positions[0].dicts, positions[1].dicts) {
		t.Fatal("distinct nested dictionary environments were merged")
	}
	for _, position := range positions {
		// Re-resolution allocates dictionary wrappers; identity is structural,
		// including nested arguments, rather than pointer equality.
		dict := position.dicts[0]
		copy := &dictionary{instance: dict.instance, args: append([]*dictionary(nil), dict.args...)}
		fn, err := e.function("count", []*dictionary{copy})
		if err != nil { t.Fatal(err) }
		args := []*term{{kind: "ctor", ctor: e.ctors["Box"], args: []*term{integer(1)}}, e.fresh("xs")}
		if e.unfoldGlobal(fn, args) {
			t.Fatal("a constructor at the wrong position allowed symbolic recursion")
		}
		args[1] = &term{kind: "ctor", ctor: e.ctors["Nil"]}
		if !e.unfoldGlobal(fn, args) {
			t.Fatal("a constructor at the certified position did not unfold")
		}
	}
	// Removing one certificate must not transfer the remaining instance's
	// recursive classification to the other dictionary environment.
	e.decreasing["count"] = positions[:1]
	other, err := e.function("count", positions[1].dicts)
	if err != nil { t.Fatal(err) }
	if !e.unfoldGlobal(other, []*term{e.fresh("seed"), e.fresh("xs")}) {
		t.Fatal("a certificate leaked into another dictionary environment")
	}
	identity, err := e.function("identity", nil)
	if err != nil { t.Fatal(err) }
	n := e.fresh("n")
	value, err := e.apply(identity, []*term{n})
	if err != nil || value != n {
		t.Fatalf("acyclic symbolic call did not unfold: %v, %v", value, err)
	}
}

func TestReentryFuelBound(t *testing.T) {
	const depth = 128
	list := strings.Repeat("Cons(0, ", depth) + "Nil" + strings.Repeat(")", depth)
	p := loadSource(t, fmt.Sprintf(`package main
 func count(xs List[Int]) Int { match xs { Nil => 0; Cons(_, rest) => 1 + count(rest) } }
 law deep(gate Option[Int]) {
   (match gate { None => count(%s); Some(_) => 0 }) ==
   (match gate { None => %d; Some(_) => 0 })
 }
 proof deep(gate) { refl }
 func main() IO[Unit] { ioPure(unit()) }
 `, list, depth))
	e := newEvaluator(p)
	law, err := e.extractLaw("deep")
	if err != nil { t.Fatal(err) }
	proof := p.Proofs[law.Name]
	if err := e.checkDependencies(p.Laws[law.Name], proof); err != nil { t.Fatal(err) }
	env, vars := e.proofEnvironment(law, proof)
	if err := e.prove(law, proof.Decl.Body, env, vars); err != nil { t.Fatal(err) }
	if e.fuel <= 0 {
		t.Fatalf("depth %d exhausted StepLimit %d", depth, StepLimit)
	}
	t.Logf("depth %d residual recursion used %d/%d steps including certification", depth, StepLimit-e.fuel, StepLimit)
}

func TestReentryTupleAndRewrite(t *testing.T) {
	p := loadSource(t, `package main
 type Chain = End | Next((Chain, Int))
 func tupleLength(pair (Chain, Int)) Int {
   match pair { (End, _) => 0; (Next(rest), _) => 1 + tupleLength(rest) }
 }
 law tuples(gate Option[Int]) {
   (match gate { None => tupleLength((Next((Next((End, 0)), 0)), 0)); Some(_) => 0 }) ==
   (match gate { None => 2; Some(_) => 0 })
 }
 proof tuples(gate) { refl }
 law identity(pair (Chain, Int)) { tupleLength(pair) == tupleLength(pair) }
 proof identity(pair) { refl }
 law unrelated(n Int, pair (Chain, Int)) { n == n }
 proof unrelated(n, pair) { rewrite identity(pair) { refl } }
 func main() IO[Unit] { ioPure(unit()) }
 `)
	if errors := CheckProgram(p); errors != nil { t.Fatal(errors) }
	e := newEvaluator(p)
	if err := e.checkDependencies(p.Laws["unrelated"], p.Proofs["unrelated"]); err != nil { t.Fatal(err) }
	if positions := e.decreasing["tupleLength"]; len(positions) != 1 || positions[0].position != 0 {
		t.Fatalf("rewrite-only dependency was not certified: %v", positions)
	}
}
