package laws

import (
	"regexp"
	"strings"
	"testing"

	"github.com/reoring/go4t/ast"
	"github.com/reoring/go4t/types"
)

func TestComparisonDisplay(t *testing.T) {
	p := loadFixture(t, "display.gt")
	errors := diagnosticsByLaw(t, p)
	for _, name := range []string{"int", "string", "bool"} {
		t.Run(name, func(t *testing.T) {
			err := errors[name]
			assertNormalForms(t, err)
			got := err.Error()
			want := "LHS: ((a == b, a != b), ((a < b, a <= b), (b < a, b <= a)))"
			if name == "bool" {
				want = "LHS: ((a == b, a != b), ((a < b, a <= b), (a > b, a >= b)))"
			}
			if !strings.Contains(got, want) {
				t.Errorf("missing %q in diagnostic: %s", want, got)
			}
		})
	}
	err := errors["raw"]
	assertNormalForms(t, err)
	got := err.Error()
	if !strings.Contains(got, "LHS: if a < b { 1 } else { 2 }") {
		t.Errorf("unrelated conditional changed: %s", got)
	}
	err = errors["rawShow"]
	assertNormalForms(t, err)
	if got := err.Error(); !strings.Contains(got, "LHS: rt.ShowInt(a)") {
		t.Errorf("unrelated primitive changed: %s", got)
	}
}

func TestStaleComparisonDisplay(t *testing.T) {
	p := loadFixture(t, "guard.gt")
	body := p.Proofs["staleIff"].Decl.Body
	p.Proofs["staleIff"].Decl.Body = &ast.TRefl{Pos: body.GetPos(), End: body.GetEnd()}
	err := diagnosticsByLaw(t, p)["staleIff"]
	assertNormalForms(t, err)
	// Residual pattern binders really do shadow t/at in this diagnostic.
	got := regexp.MustCompile(`#[0-9]+`).ReplaceAllString(err.Error(), "")
	for _, side := range []string{"LHS: ", "RHS: "} {
		_, form, _ := strings.Cut(got, side)
		form, _, _ = strings.Cut(form, "\n")
		if !strings.Contains(form, "at + t <= now") {
			t.Errorf("missing readable comparison in %s%s", side, form)
		}
		if strings.Contains(form, "rt.LtInt") {
			t.Errorf("unfolded ordering in %s%s", side, form)
		}
	}
}

func TestComparisonLaws(t *testing.T) {
	p := loadFixture(t, "compare.gt")
	errors := diagnosticsByLaw(t, p)
	bad := map[string]bool{"strictIsNotWeak": true, "weakIsNotStrict": true, "customOrder": true, "customEqual": true}
	for name := range p.Laws {
		if (errors[name] != nil) != bad[name] {
			t.Errorf("%s: unexpected result %v", name, errors[name])
		}
	}
	for name := range bad {
		assertNormalForms(t, errors[name])
	}
}

func TestSourceFaithfulDisplay(t *testing.T) {
	e := newEvaluator(&types.Program{})
	at, now := e.fresh("at"), e.fresh("now")
	difference, err := e.ring("-", now, at)
	if err != nil {
		t.Fatal(err)
	}
	if got := e.display(difference); got != "now - at" {
		t.Fatalf("difference = %q", got)
	}
	a, b := e.fresh("a"), e.fresh("b")
	sum := &term{kind: "sum", args: []*term{
		integer(1),
		{kind: "product", number: -1, args: []*term{b}, indexes: []int{1}},
		{kind: "product", number: 2, args: []*term{a}, indexes: []int{1}},
	}}
	if got := e.display(sum); got != "2 * a - b + 1" {
		t.Fatalf("polynomial = %q", got)
	}
	product := &term{kind: "product", number: -9223372036854775808, args: []*term{a}, indexes: []int{1}}
	if got := e.display(product); got != "-9223372036854775808 * a" {
		t.Fatalf("minInt coefficient = %q", got)
	}
	comparison := &term{kind: "primitive", name: "rt.LtInt", args: []*term{difference, integer(1)}}
	left := &term{kind: "if", args: []*term{comparison, a, b}}
	d := e.displayTerms(left, integer(0))
	stuck, _ := d.diagnose(left, integer(0))
	if stuck != "stuck on: now - at < 1" || !strings.Contains(d.display(left), strings.TrimPrefix(stuck, "stuck on: ")) {
		t.Fatalf("inconsistent comparison spelling: %s; %s", stuck, d.display(left))
	}
}

func TestDisplayNameCollision(t *testing.T) {
	e := newEvaluator(&types.Program{})
	a, b := e.fresh("same"), e.fresh("same")
	d := e.displayTerms(a, b)
	if d.display(a) != "same#1" || d.display(b) != "same#2" {
		t.Fatalf("ambiguous variables: %s vs %s", d.display(a), d.display(b))
	}
	_, differs := d.diagnose(a, b)
	if differs != "differs at: same#1 vs same#2" {
		t.Fatal(differs)
	}
	if got := e.display(a); got != "same" {
		t.Fatalf("collision leaked into another diagnostic: %s", got)
	}
	tuple := &term{kind: "tuple", args: []*term{a, a}}
	if got := e.display(tuple); got != "(same, same)" {
		t.Fatalf("same identity treated as a collision: %s", got)
	}
}
