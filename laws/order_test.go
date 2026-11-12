package laws

import (
	"strings"
	"testing"
)

func TestOrderRules(t *testing.T) {
	for _, tc := range []struct{ name, params, requires, left, right, proof string }{
		{"antisymmetry", "a Int, b Int", "a <= b && b <= a", "a == b", "true", "refl"},
		{"equalityDisequality", "a Int, b Int, c Int", "a == b && b != c", "a == c", "false", "refl"},
		{"negatedStrict", "a Int, b Int", "!(a < b)", "b <= a", "true", "refl"},
		{"negatedWeak", "a Int, b Int", "!(a <= b)", "b < a", "true", "refl"},
		{"disequality", "a Int, b Int", "a <= b && a != b", "a < b", "true", "refl"},
		{"refute", "a Int, b Int, c Int", "a <= b && b < c", "c <= a", "false", "refl"},
		{"literalBound", "a Int, b Int", "a <= b && b < 100", "a < 101", "true", "refl"},
		{"opaqueWrapping", "a Int, b Int, c Int", "a + 1 < b && b < c", "a + 1 < c", "true", "refl"},
		{"ifConjunction", "a Int, b Int, c Int", "", "if a < b && b < c { a < c } else { true }", "true", "if a < b && b < c { refl } else { refl }"},
		{"stuckIf", "a Int, b Int, c Int", "", "if a < b && b < c { a < c } else { false }", "if a < b && b < c { true } else { false }", "refl"},
		{"oneSummand", "a Int, b Int", "0 <= a && a <= 100 && 0 <= b && b <= 100 && a < b", "a + 2 < b + 2", "true", "refl"},
		{"twoSummands", "a Int, b Int, c Int", "0 <= a && a <= 100 && 0 <= b && b <= 100 && 0 <= c && c <= 100 && a <= b && b <= c", "a < c + 1", "true", "refl"},
		{"twoSummandsRefute", "a Int, b Int, c Int", "0 <= a && a <= 100 && 0 <= b && b <= 100 && 0 <= c && c <= 100 && a <= b && b <= c", "c + 1 <= a", "false", "refl"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			requires := ""
			if tc.requires != "" {
				requires = " requires " + tc.requires
			}
			params := strings.Split(tc.params, ", ")
			for i := range params {
				params[i], _, _ = strings.Cut(params[i], " ")
			}
			source := "package main\nlaw test(" + tc.params + ")" + requires + " { (" + tc.left + ") == (" + tc.right + ") }\nproof test(" + strings.Join(params, ", ") + ") { " + tc.proof + " }\nfunc main() IO[Unit] { ioPure(unit()) }"
			if errors := CheckProgram(loadSource(t, source)); len(errors) != 0 {
				t.Fatal(errors)
			}
		})
	}
}

func TestOrderDoesNotUseModularDifference(t *testing.T) {
	for _, source := range []string{
		"law bad(a Int) requires a == (-9223372036854775807 - 1) { (a - 1 < a) == true }",
		"law bad(a Int) requires a == 9223372036854775807 { (a + 1 > a) == true }",
		"law bad(a Int) requires a >= 0 { (a + a >= a) == true }",
		"law bad(a Int) requires a >= 0 { (-a > a) == true }",
	} {
		p := loadSource(t, "package main\n"+source+"\nproof bad(a) { refl }\nfunc main() IO[Unit] { ioPure(unit()) }")
		if errors := CheckProgram(p); len(errors) != 1 {
			t.Fatalf("accepted false law %s: %v", source, errors)
		}
	}
}

func TestRequiresEligibilityAndInduction(t *testing.T) {
	for _, tc := range []struct{ source, want string }{
		{"extern func unsafe() Bool = \"untrusted\".Unsafe\nlaw bad(a Int) requires unsafe() { a == a }\nproof bad(a) { refl }", "depends on extern untrusted.Unsafe"},
		{"func loop(a Int) Bool { loop(a) }\nlaw bad(a Int) requires loop(a) { a == a }\nproof bad(a) { refl }", "may not terminate"},
		{"law bad(f func(Int) Bool) requires f(0) { 1 == 1 }\nproof bad(f) { refl }", "higher-order"},
		{"law bad(xs List[Int]) requires xs != Nil { xs == xs }\nproof bad(xs) { induction xs }", "precondition not established:"},
	} {
		p := loadSource(t, "package main\n"+tc.source+"\nfunc main() IO[Unit] { ioPure(unit()) }")
		errors := CheckProgram(p)
		if len(errors) != 1 || !strings.Contains(errors[0].Msg, tc.want) {
			t.Fatalf("want %s: %v", tc.want, errors)
		}
	}
	p := loadSource(t, `package main
func carry(xs List[Int], a Int) Int { match xs { Nil => a + 1; Cons(_, rest) => carry(rest, a) } }
law carried(xs List[Int], a Int) requires a < 9223372036854775807 { carry(xs, a) == a + 1 }
proof carried(xs, a) { induction xs }
func main() IO[Unit] { ioPure(unit()) }
`)
	if errors := CheckProgram(p); len(errors) != 0 {
		t.Fatal(errors)
	}
}
