package lsp

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
)

func TestSourceAliasHover(t *testing.T) {
	source := `package main
alias Count = Int
alias Pair[A] = (A, A)
func identity[A](x A) A { x }
func declared(x Pair[Count]) Pair[Count] { x }
func count(x Count) Count { x }
func main() IO[Unit] {
 let annotated: Count = 1
 let inferred = identity(count(annotated))
 println(show(inferred))
}
`
	uri := fileURI(filepath.Join(t.TempDir(), "main.gt"))
	c := client(t)
	if ds := diagnostics(t, c.notify("textDocument/didOpen", openParams(uri, source))); len(ds) != 0 {
		t.Fatal(ds)
	}
	for _, tt := range []struct{ needle, want string }{
		{"declared(x", "declared : func(Pair[Count]) Pair[Count]"},
		{"count(x", "count : func(Count) Count"},
		{"annotated:", "annotated : Count"},
		{"annotated))", "annotated : Count"},
		{"inferred =", "inferred : Int"},
		{"identity(count", "identity : func(Int) Int"},
	} {
		data := c.request("textDocument/hover", positionParams(uri, source, tt.needle, 0))
		var message struct {
			Result struct {
				Contents struct{ Value string }
			}
		}
		if err := json.Unmarshal(data, &message); err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(message.Result.Contents.Value, tt.want) {
			t.Errorf("hover %s: %s, want %q", tt.needle, data, tt.want)
		}
	}
}

func TestHoverElidesLongTypes(t *testing.T) {
	nested := strings.Repeat("Option[", 24) + "Int" + strings.Repeat("]", 24)
	source := "package main\nfunc nested(x " + nested + ") " + nested + " { x }\nfunc main() IO[Unit] { ioPure(unit()) }\n"
	uri := fileURI(filepath.Join(t.TempDir(), "main.gt"))
	c := client(t)
	if ds := diagnostics(t, c.notify("textDocument/didOpen", openParams(uri, source))); len(ds) != 0 {
		t.Fatal(ds)
	}
	data := c.request("textDocument/hover", positionParams(uri, source, "nested(x", 0))
	if !strings.Contains(string(data), "func(Option[Option[…]]) Option[Option[…]]") {
		t.Fatal(string(data))
	}
}

func TestAliasMismatchDiagnostics(t *testing.T) {
	source := "package main\nalias Count = Int\nfunc bad() Count { true }\nfunc main() IO[Unit] { ioPure(unit()) }\n"
	uri := fileURI(filepath.Join(t.TempDir(), "main.gt"))
	c := client(t)
	ds := diagnostics(t, c.notify("textDocument/didOpen", openParams(uri, source)))
	if len(ds) != 1 || ds[0].Message != "type mismatch: expected Count, got Bool" {
		t.Fatal(ds)
	}
}

func TestHoverUsesFileImportAliases(t *testing.T) {
	for _, alias := range []string{"http", "web"} {
		source := "package main\nimport " + alias + " \"std/http\"\n" +
			"func identity(x " + alias + ".Response) " + alias + ".Response { x }\n" +
			"func main() IO[Unit] { ioPure(unit()) }\n"
		uri := fileURI(filepath.Join(t.TempDir(), "main.gt"))
		c := client(t)
		if ds := diagnostics(t, c.notify("textDocument/didOpen", openParams(uri, source))); len(ds) != 0 {
			t.Fatal(ds)
		}
		for _, tt := range []struct{ needle, want string }{
			{"identity(x", "identity : func(" + alias + ".Response) " + alias + ".Response"},
			{"x }", "x : " + alias + ".Response"},
		} {
			data := c.request("textDocument/hover", positionParams(uri, source, tt.needle, 0))
			if !strings.Contains(string(data), tt.want) {
				t.Errorf("hover %s: %s, want %q", tt.needle, data, tt.want)
			}
		}
		data := c.request("textDocument/documentSymbol", map[string]any{"textDocument": map[string]string{"uri": uri}})
		if !strings.Contains(string(data), "func("+alias+".Response) "+alias+".Response") {
			t.Errorf("document symbols: %s", data)
		}
	}
}
