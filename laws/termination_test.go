package laws

import (
	"strings"
	"testing"
)

func TestTerminationReachability(t *testing.T) {
	for _, tc := range []struct{ name, declarations, value, want string }{
		{"structural", `func f(xs List[Int]) Int { match xs { Nil => 0; Cons(_, rest) => 1 + f(rest) } }`, "f(xs)", ""},
		{"nested", `func f(xs List[Int]) Int { match xs { Nil => 0; Cons(_, Nil) => 1; Cons(_, Cons(_, rest)) => 2 + f(rest) } }`, "f(xs)", ""},
		{"second position", `func f(n Int, xs List[Int]) Int { match xs { Nil => n; Cons(_, rest) => f(n, rest) } }`, "f(n, xs)", ""},
		{"changed prefix", `func f(n Int, xs List[Int]) Int { match xs { Nil => n; Cons(_, rest) => f(n + 1, rest) } }`, "f(n, xs)", "depends on f which may not terminate"},
		{"not structural", `func f(n Int) Int { if n == 0 { 0 } else { f(n - 1) } }`, "f(n)", "depends on f which may not terminate"},
		{"direct cycle", `func f(n Int) Int { f(n) }`, "f(n)", "depends on f which may not terminate"},
		{"dead branch", `func f(n Int) Int { if true { 0 } else { f(n) } }`, "f(n)", "depends on f which may not terminate"},
		{"transitive", `func f(n Int) Int { g(n) }; func g(n Int) Int { g(n) }`, "f(n)", "depends on g which may not terminate"},
		{"mutual decrease", `func f(xs List[Int]) Int { match xs { Nil => 0; Cons(_, r) => g(r) } }; func g(xs List[Int]) Int { match xs { Nil => 0; Cons(_, r) => f(r) } }`, "f(xs)", ""},
		{"mutual reset", `func f(xs List[Int], ys List[Int]) Int { match xs { Nil => 0; Cons(_, r) => g(r, ys) } }; func g(xs List[Int], ys List[Int]) Int { match ys { Nil => 0; Cons(_, r) => f(xs, r) } }`, "f(xs, xs)", "may not terminate"},
		{"mutual no decrease", `func f(xs List[Int]) Int { g(xs) }; func g(xs List[Int]) Int { match xs { Nil => 0; Cons(_, r) => f(r) } }`, "f(xs)", "may not terminate"},
		{"unused loop", `func unused(n Int) Int { unused(n) }`, "n", ""},
		{"callback", `func loop(n Int) Int { loop(n) }; func apply(f func(Int) Int, n Int) Int { f(n) }`, "apply(loop, n)", "depends on loop which may not terminate"},
		{"resolved dictionary", `type Box = Box(Int); instance Eq[Box] { eq(a, b) { eq(a, b) } }; func same[A Eq](a A, b A) Bool { eq(a, b) }`, "same(Box(n), Box(n))", "depends on Eq[Box].eq which may not terminate"},
		{"method transitive", `type Box = Box(Int); func loop(n Int) Bool { loop(n) }; instance Eq[Box] { eq(a, b) { loop(0) } }`, "(Box(n) == Box(n))", "depends on loop which may not terminate"},
		{"local loop", `func f(n Int) Int { func local(x Int) Int { local(x) }; local(n) }`, "f(n)", "depends on local which may not terminate"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p := loadSource(t, "package main\n"+tc.declarations+"\nlaw total(n Int, xs List[Int]) { "+tc.value+" == "+tc.value+" }\nproof total(n, xs) { refl }\nfunc main() IO[Unit] { ioPure(unit()) }\n")
			errors := CheckProgram(p)
			if tc.want == "" {
				if errors != nil {
					t.Fatal(errors)
				}
				return
			}
			if len(errors) != 1 || !strings.Contains(errors[0].Msg, tc.want) {
				t.Fatalf("got %v, want %q", errors, tc.want)
			}
			if errors[0].Pos != p.Laws["total"].Decl.Pos {
				t.Fatalf("termination diagnostic not at law: %v", errors[0])
			}
		})
	}
}

func TestWholeBodyTaintAndProofExpressions(t *testing.T) {
	for _, body := range []string{
		`func f(n Int) Int { if true { n } else { foreign(n) } }; law total(n Int) { f(n) == n }; proof total(n) { refl }`,
		`law total(n Int) { n == n }; proof total(n) { if foreign(n) == 0 { refl } else { refl } }`,
		`law identity(n Int) { n + 1 == 1 + n }; proof identity(n) { refl }; law total(n Int) { n == n }; proof total(n) { rewrite identity(foreign(n)) { refl } }`,
	} {
		p := loadSource(t, "package main\nextern func foreign(n Int) Int = \"example.invalid\".Foreign\n"+body+"\nfunc main() IO[Unit] { ioPure(unit()) }\n")
		errors := CheckProgram(p)
		if len(errors) != 1 || !strings.Contains(errors[0].Msg, "law depends on extern example.invalid.Foreign") {
			t.Fatalf("taint escaped: %v", errors)
		}
	}
}
