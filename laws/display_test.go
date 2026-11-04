package laws

import (
	"regexp"
	"strings"
	"testing"

	"github.com/reoring/go4t/ast"
)

func TestComparisonDisplay(t *testing.T) {
	p := loadFixture(t, "display.gt")
	errors := diagnosticsByLaw(t, p)
	ids := regexp.MustCompile(`#[0-9]+`)
	for _, name := range []string{"int", "string", "bool"} {
		t.Run(name, func(t *testing.T) {
			err := errors[name]
			assertNormalForms(t, err)
			got := ids.ReplaceAllString(err.Error(), "")
			want := "LHS: (((a == b), (a != b)), (((a < b), (a <= b)), ((b < a), (b <= a))))"
			if name == "bool" {
				want = "LHS: (((a == b), (a != b)), (((a < b), (a <= b)), ((a > b), (a >= b))))"
			}
			if !strings.Contains(got, want) {
				t.Errorf("missing %q in diagnostic: %s", want, got)
			}
		})
	}
	err := errors["raw"]
	assertNormalForms(t, err)
	got := ids.ReplaceAllString(err.Error(), "")
	if !strings.Contains(got, "LHS: if (a < b) { 1 } else { 2 }") {
		t.Errorf("unrelated conditional changed: %s", got)
	}
	err = errors["rawShow"]
	assertNormalForms(t, err)
	if got := ids.ReplaceAllString(err.Error(), ""); !strings.Contains(got, "LHS: rt.ShowInt(a)") {
		t.Errorf("unrelated primitive changed: %s", got)
	}
}

func TestStaleComparisonDisplay(t *testing.T) {
	p := loadFixture(t, "guard.gt")
	body := p.Proofs["staleIff"].Decl.Body
	p.Proofs["staleIff"].Decl.Body = &ast.TRefl{Pos: body.GetPos(), End: body.GetEnd()}
	err := diagnosticsByLaw(t, p)["staleIff"]
	assertNormalForms(t, err)
	got := regexp.MustCompile(`#[0-9]+`).ReplaceAllString(err.Error(), "")
	for _, side := range []string{"LHS: ", "RHS: "} {
		_, form, _ := strings.Cut(got, side)
		form, _, _ = strings.Cut(form, "\n")
		if !strings.Contains(form, "(at + t <= now)") {
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
