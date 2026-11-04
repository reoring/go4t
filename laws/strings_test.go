package laws

import (
	"strings"
	"sync"
	"testing"

	"github.com/reoring/go4t/ast"
)

func TestStringConstantFolding(t *testing.T) {
	p := loadFixture(t, "strings.gt")
	if errors := CheckProgram(p); errors != nil {
		t.Fatal(errors)
	}
	count := 0
	for _, mod := range p.Modules {
		if mod.ImportPath != "std/string" {
			continue
		}
		for _, file := range mod.Files {
			for _, decl := range file.Decls {
				f, ok := decl.(*ast.FuncDecl)
				if !ok || f.Extern == nil {
					continue
				}
				count++
				if !primitive(f.Extern) {
					t.Errorf("missing string primitive %s", f.Extern.Name)
				}
				name := p.ModuleNames[f.Name]
				law := "fold" + strings.ToUpper(name[:1]) + name[1:]
				if p.Laws[law] == nil {
					t.Errorf("missing constant-folding fixture for %s", name)
				}
			}
		}
	}
	if count != 32 {
		t.Fatalf("string extern inventory = %d, want 32", count)
	}
	// Concurrent compiler clients share the runtime's shape ABI, but not terms
	// or evaluator state. Calls must restore the shape table before unlocking.
	var group sync.WaitGroup
	for range 4 {
		group.Add(1)
		go func() {
			defer group.Done()
			if errors := CheckProgram(p); errors != nil {
				t.Error(errors)
			}
		}()
	}
	group.Wait()
}

func TestStringBoundariesAndOpaqueCalls(t *testing.T) {
	p := loadSource(t, `package main
import "std/string"
law boundaries() {
 (string.parseInt("not an integer"), string.codePoint(""), string.indexOf("abc", "z"), string.charAt("é", -1), string.charAt("é", 2), string.splitOnce(":", "none")) ==
 (None, None, None, None, None, None)
}
proof boundaries() { refl }
law clamped() { (string.substring("éabc", -9, 2), string.substring("abc", 9, 1), string.repeat("a", -1), string.fromCodePoint(-1)) == ("éa", "", "", "�") }
proof clamped() { refl }
law symbolic(s String) { string.trim(string.trim(s)) == string.trim(s) }
proof symbolic(s) { refl }
func main() IO[Unit] { ioPure(unit()) }
`)
	errors := CheckProgram(p)
	if len(errors) != 1 || !strings.Contains(errors[0].Msg, "law symbolic:") {
		t.Fatalf("diagnostics = %v", errors)
	}
	assertNormalForms(t, errors[0])
	if !strings.Contains(errors[0].Msg, "rt.Trim(rt.Trim(") {
		t.Fatalf("symbolic string call was folded: %v", errors[0])
	}
}
