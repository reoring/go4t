package laws

import (
	"strings"
	"testing"

	"github.com/reoring/go4t/ast"
	"github.com/reoring/go4t/parser"
)

const implicitProofNote = "no proof was written for this law; it was checked by refl"

func TestLawDeclarationRewrite(t *testing.T) {
	if errors := CheckProgram(loadFixture(t, "lawdecl_surface.gt")); len(errors) != 0 {
		t.Fatal(errors)
	}
}

func TestLawImplicitCounterexample(t *testing.T) {
	for _, body := range []string{"a + 1 == 1 + a", "a + 1 == a", "a != a", "a <= a"} {
		const prefix = "package main\nlaw l(a Int) { "
		const main = "\nfunc main() IO[Unit] { ioPure(unit()) }\n"
		implicit := loadSource(t, prefix+body+" }"+main)
		explicit := loadSource(t, prefix+body+" }\nproof l(a) { refl }"+main)
		got, want := CheckProgram(implicit), CheckProgram(explicit)
		if rejected := body == "a + 1 == a" || body == "a != a"; rejected != (len(got) == 1) {
			t.Fatalf("%s: rejected=%v, diagnostics=%v", body, rejected, got)
		}
		if len(got) != len(want) {
			t.Fatalf("%s: implicit %v, written %v", body, got, want)
		}
		if len(got) == 0 {
			continue
		}
		if got[0].Msg != want[0].Msg+"\n"+implicitProofNote || !strings.Contains(got[0].Msg, "counterexample:") {
			t.Fatalf("%s: implicit %v, written %v", body, got, want)
		}
		law := implicit.Laws["l"].Decl
		if got[0].File == "" || got[0].Pos != law.LHS.GetPos() || got[0].End != law.RHS.GetEnd() {
			t.Fatalf("implicit diagnostic span: %+v", got[0])
		}
	}
}

func TestLawMultipleRequiresSatisfiability(t *testing.T) {
	const main = "\nfunc main() IO[Unit] { ioPure(unit()) }\n"
	for _, clause := range []string{" requires a > 0 requires a < 0", " requires a > 0 && a < 0"} {
		p := loadSource(t, "package main\nlaw l(a Int)"+clause+" { a == a }"+main)
		errors := CheckProgram(p)
		if len(errors) != 1 || !strings.Contains(errors[0].Msg, "requires can never hold:") || !strings.Contains(errors[0].Msg, "is false given the conditions before it") {
			t.Fatalf("%s: %v", clause, errors)
		}
		if errors[0].Pos != p.Laws["l"].Decl.Requires.GetPos() || errors[0].End != p.Laws["l"].Decl.Requires.GetEnd() {
			t.Fatalf("requires diagnostic span = %+v", errors[0])
		}
	}
	for _, clauses := range []string{" requires 0 <= a && a <= 10 requires a * 2 == 21", " requires 0 <= a && a <= 10 && a * 2 == 21"} {
		p := loadSource(t, "package main\nlaw l(a Int)"+clauses+" { a == a }"+main)
		if errors := CheckProgram(p); len(errors) != 0 || len(p.Warnings) != 1 || !strings.Contains(p.Warnings[0].Msg, "no arguments satisfying requires were found in 128 candidates; the law may say nothing") {
			t.Fatalf("witness: errors=%v warnings=%v", errors, p.Warnings)
		}
	}
}

func TestLawDeclarationSample(t *testing.T) {
	p := loadSource(t, `package main
import "std/regex"
law sampled(text String) { regex.regexReplace("z", text, "z") == text } proof { sample(10) }
func main() IO[Unit] { ioPure(unit()) }
`)
	if errors := CheckProgram(p); len(errors) != 0 || len(SampleNotices(p)) != 1 {
		t.Fatalf("sample: errors=%v notices=%v", errors, SampleNotices(p))
	}
}

func TestLawDeclarationRequiresSourceSlicing(t *testing.T) {
	for _, tc := range []struct{ source, left, right string }{
		{"law l(a Int) { a + 1 == 1 + a }", "a + 1", "1 + a"},
		{"law l(a Int) requires a > 0 requires a < 10 { a > 0 } proof { refl }", "a > 0", "true"},
		{"law l(a Bool) { a && (a == a); } proof { if a { refl } else { refl } }", "a && (a == a)", "true"},
		{"law l(a Bool) { (a == a) == true } proof { refl }", "(a == a)", "true"},
		{"law l(a Int) { (a + 1 == 1 + a); } proof { refl }", "a + 1", "1 + a"},
		{"law l(a Bool) { a; // trailing\n }", "a", "true"},
	} {
		source := "package main\n" + tc.source
		f, err := parser.Parse(source)
		if err != nil {
			t.Fatal(err)
		}
		law := f.Decls[0].(*ast.LawDecl)
		if left, right := propertyLawSource(source, law, 0), propertyLawSource(source, law, 1); left != tc.left || right != tc.right {
			t.Fatalf("%s: sides %q, %q; want %q, %q", tc.source, left, right, tc.left, tc.right)
		}
	}
}
