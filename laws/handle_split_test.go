package laws

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/reoring/go4t/ast"
	"github.com/reoring/go4t/modules"
	"github.com/reoring/go4t/types"
)

// A handle's binding declaration is unit-like, but two handles are distinct Go
// identities: no proof may refine one to its constructor.
func TestHandleCaseSplitRejected(t *testing.T) {
	for name, proof := range map[string]string{
		"named":  "match a { DBHandle => match b }",
		"bare":   "match a",
		"nested": "match p { Pair(DBHandle, DBHandle) => refl }",
	} {
		t.Run(name, func(t *testing.T) {
			t.Setenv("GO4T_PATH", "")
			dir := t.TempDir()
			source := "package main\ntype DB = DBHandle\ntype Pair = Pair(DB, DB)\n" +
				"func pick(c Bool, a DB, b DB) DB { if c { a } else { b } }\n" +
				"law same(a DB, b DB, p Pair) { pick(true, a, b) == pick(false, a, b) } proof { " + proof + " }\n" +
				"func main() IO[Unit] { ioPure(unit()) }\n"
			if err := os.WriteFile(filepath.Join(dir, "laws.gt"), []byte(source), 0o600); err != nil {
				t.Fatal(err)
			}
			loaded, err := modules.Load([]string{dir})
			if err != nil {
				t.Fatal(err)
			}
			for _, module := range loaded {
				if module.Name == "main" {
					module.ForeignHandleKeys = map[string]ast.ForeignHandle{"DB": {PackagePath: "database/sql", TypeName: "DB", Struct: true}}
				}
			}
			p, err := types.Check(loaded)
			if err != nil {
				t.Fatal(err)
			}
			errors := CheckProgram(p)
			if len(errors) != 1 || !strings.Contains(errors[0].Msg, "cannot case-split opaque handle") {
				t.Fatalf("handle split: errors=%v", errors)
			}
		})
	}
}
