package laws

import (
	"strings"
	"testing"
)

func TestOrPatternLaws(t *testing.T) {
	p := loadFixture(t, "patterns_or.gt")
	diagnostics := diagnosticsByLaw(t, p)
	bad := map[string]bool{"notWholeAlternative": true, "notEveryColor": true, "notEveryBinding": true}
	for name := range p.Laws {
		if (diagnostics[name] != nil) != bad[name] {
			t.Errorf("%s: unexpected result %v", name, diagnostics[name])
		}
	}
	for name := range bad {
		assertNormalForms(t, diagnostics[name])
		if diagnostics[name] != nil && !strings.Contains(diagnostics[name].Msg, "counterexample:") {
			t.Errorf("%s: missing counterexample: %v", name, diagnostics[name])
		}
	}
	if diagnostic := diagnostics["notWholeAlternative"]; diagnostic != nil {
		if !strings.Contains(diagnostic.Msg, "counterexample: x = B") || !strings.Contains(diagnostic.Msg, "LHS: B\n  RHS: A") {
			t.Fatalf("stuck or-arm did not retain the B alternative:\n%s", diagnostic.Msg)
		}
	}
}

func TestOrPatternStructuralRecursion(t *testing.T) {
	for _, tt := range []struct {
		body        string
		wantFailure bool
	}{
		{`match xs { Cons(A | B, rest) => f(rest); Cons(C, rest) => f(rest); Nil => 0 }`, false},
		// The second alternative binds rest to the original value, not a child.
		{`match xs { Cons(_, rest) | rest => f(rest) }`, true},
	} {
		p := loadSource(t, "package main\ntype Color = A | B | C deriving Debug\nfunc f(xs List[Color]) Int { "+tt.body+" }\nlaw total(xs List[Color]) { f(xs) == f(xs) }\nproof total(xs) { refl }\nfunc main() IO[Unit] { ioPure(unit()) }\n")
		diagnostics := CheckProgram(p)
		if !tt.wantFailure {
			if len(diagnostics) != 0 {
				t.Fatal(diagnostics)
			}
		} else if len(diagnostics) != 1 || !strings.Contains(diagnostics[0].Msg, "may not terminate") {
			t.Fatalf("non-decreasing alternative escaped termination checking: %v", diagnostics)
		}
	}
}
