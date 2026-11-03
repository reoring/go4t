package laws

import (
	"strings"
	"testing"
)

func TestInductionAndRing(t *testing.T) {
	p := loadFixture(t, "induction.gt")
	errors := diagnosticsByLaw(t, p)
	for _, name := range []string{"lenApp", "appNil", "appAssoc", "ringComm", "ringWrap", "ringPowers", "ringNeg", "ringCoefficients", "ringAlpha", "ringResidual", "ringAlphaResidual", "ringDivision", "ringRemainder", "ihOrder", "useDouble", "appPrefix", "boolIdentity"} {
		if err := errors[name]; err != nil {
			t.Errorf("%s: %v", name, err)
		}
	}
	for _, name := range []string{"notDivision", "notOrder", "notString", "wrongLenApp", "stdLenApp", "zeroLen", "oneLen", "expandLen"} {
		if errors[name] == nil {
			t.Errorf("%s unexpectedly proved", name)
			continue
		}
		assertNormalForms(t, errors[name])
	}
	if err := errors["useBare"]; err == nil || !strings.Contains(err.Msg, "left side is a free variable") {
		t.Errorf("bare lhs: %v", err)
	}
	cycles := 0
	for _, err := range errors {
		if strings.Contains(err.Msg, "law dependency cycle") {
			cycles++
		}
	}
	if cycles != 1 {
		t.Errorf("cycle reported %d times", cycles)
	}
	if err := errors["stdLenApp"]; err != nil && (!strings.Contains(err.Msg, "fold(append(") || !strings.Contains(err.Msg, ", 1, func(")) {
		t.Errorf("missing accumulator evidence: %v", err)
	}
}

func TestRewriteDependencyOrderAndProofRequirement(t *testing.T) {
	for _, tc := range []struct {
		lemma string
		valid bool
	}{
		{"match xs { Nil => refl; Cons(x, rest) => rewrite zlemma(rest) { refl } }", true},
		{"refl", false},
	} {
		p := loadSource(t, `package main
func app(xs List[Int], ys List[Int]) List[Int] { match xs { Nil => ys; Cons(x, rest) => Cons(x, app(rest, ys)) } }
law ause(xs List[Int]) { (app(xs, []), app(xs, [])) == (xs, xs) }
proof ause(xs) { rewrite zlemma(xs) { refl } }
law zlemma(xs List[Int]) { app(xs, []) == xs }
proof zlemma(xs) { `+tc.lemma+` }
func main() IO[Unit] { ioPure(unit()) }
`)
		errors := CheckProgram(p)
		if tc.valid && errors != nil {
			t.Fatal(errors)
		}
		if !tc.valid && (len(errors) != 1 || !strings.Contains(errors[0].Msg, "law zlemma:")) {
			t.Fatalf("unproved lemma accepted: %v", errors)
		}
	}
}

func TestInductionArgumentOrderAndNestedDescendants(t *testing.T) {
	for _, tc := range []struct {
		args  string
		valid bool
	}{
		{"n, tail, x + acc", true},
		{"n + 1, tail, acc", false},
		{"n, xs, acc", false},
		{"n, Cons(x, tail), acc", false},
	} {
		p := loadSource(t, `package main
func len(xs List[Int]) Int { match xs { Nil => 0; Cons(_, rest) => 1 + len(rest) } }
law identity(n Int, xs List[Int], acc Int) { len(xs) == len(xs) }
proof identity(n, xs, acc) {
 match xs {
  Nil => refl
  Cons(_, Nil) => refl
  Cons(_, Cons(x, tail)) => rewrite identity(`+tc.args+`) { refl }
 }
}
func main() IO[Unit] { ioPure(unit()) }
`)
		errors := CheckProgram(p)
		if tc.valid && errors != nil {
			t.Fatal(errors)
		}
		if !tc.valid && (len(errors) != 1 || !strings.Contains(errors[0].Msg, "induction hypothesis must structurally decrease")) {
			t.Fatalf("args %s: %v", tc.args, errors)
		}
	}
}

func TestRewriteDoesNotRevisitReplacement(t *testing.T) {
	e := &evaluator{fuel: StepLimit}
	a := e.fresh("a")
	from := &term{kind: "primitive", name: "f", args: []*term{a}}
	to := &term{kind: "tuple", args: []*term{from, from}}
	result, err := e.replace(from, from, to)
	if err != nil || result != to {
		t.Fatalf("replacement revisited: %v", err)
	}
}
