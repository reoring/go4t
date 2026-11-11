package laws

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/reoring/go4t/modules"
	"github.com/reoring/go4t/types"
)

func TestPolymorphicInductionRewriteRequires(t *testing.T) {
	p := loadFixture(t, "polylaws_induction.gt")
	if diagnostics := CheckProgram(p); len(diagnostics) != 0 {
		t.Fatal(diagnostics)
	}
	e := newEvaluator(p)
	law, err := e.extractLaw("guarded")
	if err != nil {
		t.Fatal(err)
	}
	if result := e.searchCandidates(law, true); !result.witness {
		t.Fatalf("Int-instantiated requires witness was not found: %+v", result)
	}
	if p.Laws["guarded"].ParamTypes[0] != p.Laws["guarded"].TParams[0] {
		t.Fatal("witness search changed the rigid law type")
	}
}

func TestPolymorphicCounterexample(t *testing.T) {
	p := loadFixture(t, "polylaws_false.gt")
	diagnostics := diagnosticsByLaw(t, p)
	for name, header := range map[string]string{
		"allEqual":    "counterexample (A = Int): x = 1, y = 0",
		"bothTypes":   "counterexample (A = Int, B = Int): x = 1, y = 0, z = 0",
		"wrongLength": "counterexample (A = Int): xs = Cons(0, Nil)",
	} {
		if err := diagnostics[name]; err == nil || !strings.Contains(err.Msg, header) {
			t.Errorf("%s: got %v, want %q", name, err, header)
		}
	}
}

func TestPolymorphicSampleBoundary(t *testing.T) {
	p := loadFixture(t, "polylaws_induction.gt")
	if err := checkSample(p, p.Laws["appendNil"]); err == nil || err.Error() != "sample does not support type parameters" {
		t.Fatalf("sample boundary: %v", err)
	}
}

func TestPolymorphicEligibility(t *testing.T) {
	p := loadSource(t, `package main
law function[A](f func(A) A) { f == f }
proof function(f) { refl }
func main() IO[Unit] { ioPure(unit()) }`)
	if diagnostics := CheckProgram(p); len(diagnostics) != 1 || !strings.Contains(diagnostics[0].Msg, "higher-order quantified parameters are not supported") {
		t.Fatal(diagnostics)
	}
	p = loadFixture(t, "polylaws_induction.gt")
	e := newEvaluator(p)
	for _, law := range p.Laws {
		for _, param := range law.TParams {
			if reason := e.unsupportedType(param, true, make(map[string]bool)); reason != "" {
				t.Fatal(reason)
			}
		}
	}
	if reason := e.unsupportedType(types.Float, true, make(map[string]bool)); reason != "Float is not supported in laws" {
		t.Fatal(reason)
	}
}

func TestPolymorphicRewriteAcrossModules(t *testing.T) {
	t.Setenv("GO4T_PATH", "")
	root := t.TempDir()
	for name, source := range map[string]string{
		"main.gt": `package main
import "rules"
law both(xs List[Int], ys List[Bool]) { (append(xs, Nil), append(ys, Nil)) == (xs, ys) }
proof both(xs, ys) { rewrite rules.appendNil(xs) { rewrite rules.appendNil(ys) { refl } } }
func main() IO[Unit] { ioPure(unit()) }`,
		"rules/law.gt": `package rules
law appendNil[A](xs List[A]) { append(xs, Nil) == xs }`,
		"rules/proof.gt": `package rules
proof appendNil(xs) { induction xs }`,
	} {
		file := filepath.Join(root, name)
		if err := os.MkdirAll(filepath.Dir(file), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(file, []byte(source), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	loaded, err := modules.Load([]string{root})
	if err != nil {
		t.Fatal(err)
	}
	p, err := types.Check(loaded)
	if err != nil {
		t.Fatal(err)
	}
	if diagnostics := CheckProgram(p); len(diagnostics) != 0 {
		t.Fatal(diagnostics)
	}
}

func TestPolymorphicOpaqueModels(t *testing.T) {
	if diagnostics := CheckProgram(loadFixture(t, "polylaws_models.gt")); len(diagnostics) != 0 {
		t.Fatal(diagnostics)
	}
}

func TestPolymorphicRejectedOperations(t *testing.T) {
	t.Setenv("GO4T_PATH", "")
	for _, tt := range []struct{ name, source, want string }{
		{"Int arithmetic", `law bad[A](x A) { x + 0 == x }; proof bad(x) { refl }`, "type mismatch: expected A, got Int"},
		{"list sum", `law bad[A](xs List[A]) { list.sum(xs) == 0 }; proof bad(xs) { refl }`, "type mismatch"},
		{"map key", `law bad[A](m Map[A, Int], key A) { map.mapLookup(m, key) == None }; proof bad(m, key) { refl }`, "Ord[A]"},
		{"derived Eq", `type Box[A] = Box(A) deriving Eq; law bad[A](x Box[A]) { eq(x, x) == true }; proof bad(x) { refl }`, "Eq[A]"},
		{"derived Ord", `type Box[A] = Box(A) deriving Eq, Ord; law bad[A](x Box[A]) { compare(x, x) == EQ }; proof bad(x) { refl }`, "Ord[A]"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			file := filepath.Join(dir, "polylaws_rejected.gt")
			source := "package main\nimport \"std/list\"\nimport \"std/map\"\n" + tt.source + "\nfunc main() IO[Unit] { ioPure(unit()) }\n"
			if err := os.WriteFile(file, []byte(source), 0o600); err != nil {
				t.Fatal(err)
			}
			loaded, err := modules.Load([]string{dir})
			if err != nil {
				t.Fatal(err)
			}
			if _, err := types.Check(loaded); err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("got %v, want %q", err, tt.want)
			}
		})
	}
}
