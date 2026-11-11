package laws

import (
	"os"
	"strings"
	"testing"

	"github.com/reoring/go4t/ast"
)

func TestTacticForms(t *testing.T) {
	for _, fixture := range []string{"tactics_forms.gt", "tactics/tactics_accepted.gt"} {
		if errors := CheckProgram(loadFixture(t, fixture)); len(errors) != 0 {
			t.Fatalf("%s: %v", fixture, errors)
		}
	}
}

func TestTacticFalseLaws(t *testing.T) {
	p := loadFixture(t, "tactics/tactics_rejected.gt")
	errors := diagnosticsByLaw(t, p)
	for name := range p.Laws {
		if errors[name] == nil {
			t.Errorf("false or unproved law %s was accepted", name)
		}
	}
	for _, name := range []string{"falseAbsurd", "satisfiable", "merelyUnproved", "falseAssumption"} {
		if err := errors[name]; err == nil || !strings.Contains(err.Msg, "absurd: the assumptions in this case are not contradictory") {
			t.Errorf("%s: %v", name, err)
		}
	}
	if err := errors["satisfiable"]; err == nil || !strings.Contains(err.Msg, "\nassumed: 0 < x is true") {
		t.Errorf("missing assumption: %v", err)
	}
	if err := errors["falseExpressionDescent"]; err == nil || !strings.Contains(err.Msg, "induction hypothesis must structurally decrease") {
		t.Errorf("expression manufactured structural descent: %v", err)
	}
}

func TestAbsurdReflDiagnostic(t *testing.T) {
	source, err := os.ReadFile("testdata/tactics/tactics_accepted.gt")
	if err != nil {
		t.Fatal(err)
	}
	bad := strings.Replace(string(source), "Done => absurd", "Done => refl", 1)
	diagnostic := diagnosticsByLaw(t, loadSource(t, bad))["bumpAdds"]
	if diagnostic == nil || !strings.HasSuffix(diagnostic.Msg, " is false here; close it with absurd") || strings.Contains(diagnostic.Msg, "counterexample") {
		t.Fatalf("refl contradiction diagnostic: %v", diagnostic)
	}
}

func TestTacticDecidedAndEligibility(t *testing.T) {
	for _, tc := range []struct{ declarations, body, want string }{
		{"", "match true", "match scrutinee is already decided: true"},
		{"", "match Some(c)", "match scrutinee is already decided: Some(c)"},
		{"", "match (c, c)", "match scrutinee is already decided: (c, c)"},
		{"func loop(c Bool) Bool { loop(c) }", "match loop(c)", "may not terminate"},
		{"", "match (1 / (if c { 1 } else { 2 }) == 0)", "division"},
	} {
		p := loadSource(t, "package main\n"+tc.declarations+"\nlaw p(c Bool) { c == c }; proof p(c) { "+tc.body+" }; func main() IO[Unit] { ioPure(unit()) }")
		errors := CheckProgram(p)
		if len(errors) != 1 || !strings.Contains(errors[0].Msg, tc.want) {
			t.Errorf("%s: %v, want %s", tc.body, errors, tc.want)
		}
	}
}

func TestAbsurdAssumptionReplay(t *testing.T) {
	e := newEvaluator(loadFixture(t, "tactics_forms.gt"))
	x := e.fresh("x")
	// Neither the first fact nor a later fact may certify itself. With a later
	// conflicting fact, replay the second against the first, not vice versa.
	e.addAssumption(x, true)
	first := e.assumptions
	condition, _, err := e.contradiction()
	if err != nil || condition != nil || e.assumptions != first {
		t.Fatalf("satisfiable replay: condition=%v err=%v", condition, err)
	}
	e.addAssumption(x, false)
	last := e.assumptions
	condition, value, err := e.contradiction()
	if err != nil || condition != x || value.kind != "bool" || !value.flag || e.assumptions != last {
		t.Fatalf("ordered replay: condition=%v value=%v err=%v", condition, value, err)
	}
	e.assumptions = first
	e.solutions[x.id] = boolean(false)
	condition, value, err = e.contradiction()
	if err != nil || condition != x || value.kind != "bool" || value.flag {
		t.Fatalf("solution replay: condition=%v value=%v err=%v", condition, value, err)
	}
	delete(e.solutions, x.id)
	e.assumptions = nil
	if err := e.absurd(); err == nil {
		t.Fatal("absurd with no assumptions was accepted")
	}
}

func TestAbsurdOrderCycles(t *testing.T) {
	for _, domain := range []string{"Int", "String"} {
		e := newEvaluator(loadFixture(t, "tactics_forms.gt"))
		x, y, z := e.fresh("x"), e.fresh("y"), e.fresh("z")
		e.addAssumption(scalarComparison(domain, "<", x, y), true)
		e.addAssumption(scalarComparison(domain, "<=", y, z), true)
		if impossible, err := e.impossibleOrder(scalarComparison(domain, "<", x, z)); err != nil || impossible {
			t.Fatalf("%s satisfiable: impossible=%v err=%v", domain, impossible, err)
		}
		e.addAssumption(scalarComparison(domain, "<=", z, x), true)
		if condition, _, err := e.contradiction(); err != nil || condition == nil {
			t.Fatalf("%s cycle: condition=%v err=%v", domain, condition, err)
		}
	}
}

func TestTacticInductionBinderIsolation(t *testing.T) {
	p := loadFixture(t, "tactics_forms.gt")
	e := newEvaluator(p)
	law, err := e.extractLaw("induction")
	if err != nil {
		t.Fatal(err)
	}
	env, vars := e.proofEnvironment(law, p.Proofs[law.Name])
	tactic := p.Proofs[law.Name].Decl.Body.(*ast.TMatch)
	split := vars[tactic.Var.Name]
	for _, arm := range e.inductionArms(tactic, split.typ) {
		branch := copyProofVars(vars)
		value := e.splitPattern(arm.Pat, split, branch)
		before := copyProofVars(branch)
		if _, err := e.induction(law, tactic, arm, value, split, env, branch); err != nil {
			t.Fatal(err)
		}
		for name, variable := range before {
			if branch[name].value != variable.value {
				t.Errorf("induction rewrite argument overwrote case binder %s", name)
			}
		}
	}
}
