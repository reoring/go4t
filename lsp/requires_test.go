package lsp

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRequiresLawDiagnostics(t *testing.T) {
	source := "package main\nlaw successor(a Int) requires a < 9223372036854775807 { (a + 1 > a) == true }\nproof successor(a) { refl }\nfunc main() IO[Unit] { ioPure(unit()) }\n"
	file := filepath.Join(t.TempDir(), "requires.gt")
	if err := os.WriteFile(file, []byte(source), 0o600); err != nil {
		t.Fatal(err)
	}
	c := client(t)
	uri := fileURI(file)
	if ds := diagnostics(t, c.notify("textDocument/didOpen", openParams(uri, source))); len(ds) != 0 {
		t.Fatal(ds)
	}
	bad := strings.Replace(source, "requires a < 9223372036854775807", "requires a", 1)
	ds := diagnostics(t, c.notify("textDocument/didChange", changeParams(uri, bad, 2)))
	if len(ds) != 1 || !strings.Contains(ds[0].Message, "expected Bool, got Int") || ds[0].Range.Start.Line != 1 {
		t.Fatalf("precondition diagnostic = %+v", ds)
	}
}
