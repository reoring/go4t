package laws

import (
	"fmt"
	"strings"
	"testing"

	"github.com/reoring/go4t/ast"
	"github.com/reoring/go4t/modules"
	"github.com/reoring/go4t/parser"
	"github.com/reoring/go4t/types"
)

func TestSampleChecks(t *testing.T) {
	p := loadFixture(t, "sample/valid.gt")
	if errors := CheckProgram(p); len(errors) != 0 {
		t.Fatal(errors)
	}
	notices := SampleNotices(p)
	if len(notices) != 6 {
		t.Fatalf("notices = %v", notices)
	}
	for _, notice := range notices {
		if !strings.Contains(notice, ": law ") || !strings.Contains(notice, "tested by go4t test (sample ") || !strings.HasSuffix(notice, "), not proved") || strings.Contains(notice, "\n") {
			t.Fatalf("notice = %q", notice)
		}
	}
}

func TestSampleAndInductionDiagnostics(t *testing.T) {
	for _, tc := range []struct{ declarations, want string }{
		{`law f(x Int) { x == x }; proof f(x) { sample(1) }`, "prove it instead"},
		{`law f(x Int) { x == x }; proof f(x) { sample(100000) }`, "prove it instead"},
		{`law f(x Int) { x == x }; proof f(x) { sample(0) }`, "sample count must be an Int literal in 1..100000"},
		{`law f(x Int) { x == x }; proof f(x) { sample(100001) }`, "sample count must be an Int literal in 1..100000"},
		{`law f(x Int) { x == x }; proof f(x) { sample(-1) }`, "sample count must be an Int literal in 1..100000"},
		{`law f(x Int) { x == x }; proof f(x) { sample(x) }`, "sample count must be an Int literal in 1..100000"},
		{`law f(x Int) { x == x }; proof f(x) { sample(1.0) }`, "sample count must be an Int literal in 1..100000"},
		{`law f(x Int) { x == x }; proof f(x) { if x > 0 { sample(1) } else { refl } }`, "sample must be the whole proof body"},
		{`law f(x func(Int) Int) { 1 == 1 }; proof f(x) { sample(1) }`, "sample: no usable generator for func(Int) Int"},
		{`law f(x IO[Int]) { 1 == 1 }; proof f(x) { sample(1) }`, "sample: no usable generator for IO[Int]"},
		{`law f(x Array[Int]) { 1 == 1 }; proof f(x) { sample(1) }`, "sample: no usable generator for Array[Int]"},
		{`law f(x Ref[Int]) { 1 == 1 }; proof f(x) { sample(1) }`, "sample: no usable generator for Ref[Int]"},
		{`type Bad = Bad(func(Int) Int); law f(x Bad) { 1 == 1 }; proof f(x) { sample(1) }`, "sample: no usable generator for func(Int) Int"},
		{`type Loop = Loop(Loop) deriving Debug; law f(x Loop) { regex.regexMatch("a", "a") == true }; proof f(x) { sample(1) }`, "no usable generator for Loop within depth 4"},
		{`type NoEq = NoEq; law f() { (if regex.regexMatch("a", "a") { NoEq } else { NoEq }) == NoEq }; proof f() { sample(1) }`, "no instance Eq[NoEq]"},
		{`law f() { regex.regexMatch("a", "a") == true }; proof f() { sample(1) }; law g() { true == true }; proof g() { rewrite f() { refl } }`, "cannot rewrite sampled law f: tested, not proved"},
		{`func loop(x Int) Int { loop(x) }; law f(x Int) { loop(x) == loop(x) }; proof f(x) { sample(1) }`, "prove it instead"},
		{`law f(x Int) { x == x }; proof f(x) { induction x }`, "cannot split Int"},
		{`law f(x List[Int]) { x == x }; proof f(x) { induction x { Nil => refl } }`, "non-exhaustive match"},
		{`law f(x List[Int]) { x == x }; proof f(x) { induction x { _ => refl } }`, "induction arms require constructor patterns with field binders"},
		{`law f(x List[Int]) { x == x }; proof f(x) { match x { Nil => refl; Cons(_, tail) => induction tail } }`, "induction variable tail is not a quantified variable"},
	} {
		t.Run(tc.want+"/"+tc.declarations, func(t *testing.T) {
			file, err := parser.Parse("package main\nimport \"std/regex\"\n" + tc.declarations + "\nfunc main() IO[Unit] { ioPure(unit()) }\n")
			if err == nil {
				var mods []*ast.Module
				mods, err = modules.LoadFiles([]*ast.File{file}, t.TempDir())
				if err == nil {
					var p *types.Program
					p, err = types.Check(mods)
					if err == nil {
						if diagnostics := CheckProgram(p); len(diagnostics) != 0 {
							err = fmt.Errorf("%v", diagnostics)
						}
					}
				}
			}
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error = %v, want %s", err, tc.want)
			}
		})
	}
}
