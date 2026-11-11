package lsp

import (
	"path/filepath"
	"testing"
)

func TestRecursionWarningDiagnostic(t *testing.T) {
	source := `package main
func sumTo(n Int) Int { if n == 0 { 0 } else { n + sumTo(n - 1) } }
func main() IO[Unit] { println(show(sumTo(5))) }
`
	uri := fileURI(filepath.Join(t.TempDir(), "main.gt"))
	c := client(t)
	ds := diagnostics(t, c.notify("textDocument/didOpen", openParams(uri, source)))
	want := "sumTo calls itself in a non-tail position and no argument is a structural part of its parameter; go4t cannot bound the recursion depth, and stack exhaustion cannot be recovered. Use an accumulator so the call is a tail call"
	if len(ds) != 1 || ds[0].Severity != 2 || ds[0].Source != "go4t" || ds[0].Message != want || ds[0].Range != (Range{Start: Position{Line: 1, Character: 51}, End: Position{Line: 1, Character: 63}}) {
		t.Fatalf("diagnostics = %#v", ds)
	}
}
