package lsp

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestTacticAbsurdDiagnostics(t *testing.T) {
	source := "package main\nlaw identity(c Bool) { c == c }\nproof identity(c) { match c { true => absurd; false => refl } }\nfunc main() IO[Unit] { ioPure(unit()) }\n"
	file := filepath.Join(t.TempDir(), "tactics.gt")
	if err := os.WriteFile(file, []byte(source), 0o600); err != nil {
		t.Fatal(err)
	}
	c := client(t)
	uri := fileURI(file)
	ds := diagnostics(t, c.notify("textDocument/didOpen", openParams(uri, source)))
	if len(ds) != 1 || !strings.Contains(ds[0].Message, "absurd: the assumptions in this case are not contradictory") || ds[0].Range.Start != (Position{2, 38}) || ds[0].Range.End != (Position{2, 44}) {
		t.Fatalf("absurd diagnostic = %+v", ds)
	}
	fixed := strings.Replace(source, "match c { true => absurd; false => refl }", "match c", 1)
	if ds := diagnostics(t, c.notify("textDocument/didChange", changeParams(uri, fixed, 2))); len(ds) != 0 {
		t.Fatalf("bare Bool match = %+v", ds)
	}
}
