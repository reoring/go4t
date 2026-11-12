package laws

import (
	"strings"
	"testing"

	"github.com/reoring/go4t/ast"
	"github.com/reoring/go4t/parser"
	"github.com/reoring/go4t/types"
)

func TestDeclsRequiresCounterexample(t *testing.T) {
	p := loadFixture(t, "decls_requires.gt")
	diagnostics := diagnosticsByLaw(t, p)
	if len(diagnostics) != 1 || diagnostics["refuted"] == nil {
		t.Fatalf("diagnostics=%v", diagnostics)
	}
	if !strings.Contains(diagnostics["refuted"].Msg, "counterexample") {
		t.Fatalf("missing counterexample: %v", diagnostics["refuted"])
	}
}

func TestDeclsTaintTermination(t *testing.T) {
	p := loadSource(t, `package main
extern func foreign() Int = "example.invalid/foreign".Read
let value: Int = foreign()
law tainted() { value == value }
proof tainted() { refl }
func main() IO[Unit] { ioPure(unit()) }
`)
	diagnostics := CheckProgram(p)
	if len(diagnostics) != 1 || !strings.Contains(diagnostics[0].Msg, "depends on extern example.invalid/foreign.Read") {
		t.Fatalf("diagnostics=%v", diagnostics)
	}
}

func TestDeclsEvalUnnamed(t *testing.T) {
	file, err := parser.Parse(`package main
let answer = 42
law constantValue() { answer == 42 }
proof constantValue() { refl }
func main() IO[Unit] { main() }
`)
	if err != nil {
		t.Fatal(err)
	}
	p, err := types.Check([]*ast.Module{{Files: []*ast.File{file}}})
	if err != nil {
		t.Fatal(err)
	}
	if diagnostics := CheckProgram(p); len(diagnostics) != 0 {
		t.Fatal(diagnostics)
	}
}
