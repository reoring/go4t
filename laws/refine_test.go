package laws

import (
	"strings"
	"testing"

	"go4t/types"
)

func TestScrutineeRefinement(t *testing.T) {
	p := loadFixture(t, "refine.gt")
	diagnostics := diagnosticsByLaw(t, p)
	bad := map[string]string{
		"differentScrutinees": "stuck on: o#1.mode",
		"noSiblingLeak": "stuck normal forms differ",
		"noIfSiblingLeak": "stuck normal forms differ",
		"differentClosures": "stuck normal forms differ",
	}
	for name := range p.Laws {
		diagnostic := diagnostics[name]
		if want := bad[name]; want != "" {
			if diagnostic == nil || !strings.Contains(diagnostic.Msg, want) {
				t.Errorf("%s: got %v, want %q", name, diagnostic, want)
			}
		} else if diagnostic != nil {
			t.Errorf("%s: unexpected diagnostic: %v", name, diagnostic)
		}
	}
	if len(diagnostics) != len(bad) {
		t.Fatalf("diagnostics = %v", diagnostics)
	}
	if !strings.Contains(diagnostics["differentScrutinees"].Msg, "stuck on: o#1.x") {
		t.Fatal(diagnostics["differentScrutinees"])
	}

	e := newEvaluator(p)
	law, err := e.extractLaw("nestedDryRun")
	if err != nil { t.Fatal(err) }
	env, _ := e.proofEnvironment(law, p.Proofs[law.Name])
	left, right, err := e.equation(law, env)
	if err != nil { t.Fatal(err) }
	want := "match o#1.mode { DryRun => Skip; Live => Skip }"
	if e.display(left) != want || e.display(right) != want {
		t.Fatalf("nested normal forms: %s vs %s, want %s", e.display(left), e.display(right), want)
	}
	if e.assumptions != nil {
		t.Fatal("expression refinement leaked into the surrounding proof")
	}
	t.Logf("nested-match normal form: %s", e.display(left))
}

func TestResidualRefinement(t *testing.T) {
	e := newEvaluator(&types.Program{})
	x := e.fresh("x")
	y := e.fresh("y")
	choice := func(condition, left, right *term) *term {
		return &term{kind: "if", args: []*term{condition, left, right}}
	}
	match := func(body *term) *term {
		return &term{kind: "match", args: []*term{x}, arms: []normalArm{
			{pattern: integer(0), guard: boolean(true), body: body},
			{pattern: &term{kind: "bind", id: y.id, name: y.name}, guard: boolean(true), body: integer(2)},
		}}
	}
	for _, tc := range []struct {
		name string
		input, want *term
	}{
		{"if", choice(x, choice(x, integer(1), integer(9)), choice(x, integer(9), integer(2))), choice(x, integer(1), integer(2))},
		{"match", match(match(integer(1))), match(integer(1))},
		{"integer subterm", match(&term{kind: "ring", name: "+", args: []*term{x, integer(1)}}), match(integer(1))},
		{"selected if", choice(boolean(true), integer(1), &term{kind: "binary", name: "/", args: []*term{integer(1), integer(0)}}), integer(1)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := e.normal(tc.input)
			if err != nil { t.Fatal(err) }
			if !alphaEqual(got, tc.want, nil) {
				t.Fatalf("normal = %s, want %s", e.display(got), e.display(tc.want))
			}
			if e.assumptions != nil { t.Fatal("residual refinement leaked") }
		})
	}

	// On an error, restore the incoming proof hypothesis, not just an empty stack.
	parent := &assumption{condition: e.fresh("outer"), value: boolean(true)}
	e.assumptions = parent
	failure := &term{kind: "binary", name: "/", args: []*term{integer(1), integer(0)}}
	for _, input := range []*term{choice(x, failure, integer(2)), match(failure)} {
		if _, err := e.normal(input); err == nil || !strings.Contains(err.Error(), "division by zero") {
			t.Fatalf("normalization error = %v", err)
		}
		if e.assumptions != parent { t.Fatal("error path lost the surrounding hypothesis") }
	}
}
