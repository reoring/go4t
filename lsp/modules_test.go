package lsp

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestModuleGraphNavigation(t *testing.T) {
	root := t.TempDir()
	for name, source := range map[string]string{
		"gofx.mod": "module sample\n",
		"lib/defs.gfx": `package lib
// Keeps the supplied value.
func identity[A](value A) A { value }
type Box = Box{value: Int}
class Render[A] { render(value A) String }
instance Render[Box] { render(value) { show(value.value) } }
`,
		"app/other.gfx": "package main\nfunc extra() Int { 2 }\n",
	} {
		path := filepath.Join(root, name)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(source), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	source := `package main
import m "sample/lib"
import "std/list"
func value(box m.Box) Int { match box { m.Box(n) => m.identity(n) } }
func main() IO[Unit] {
  let box = m.Box{value: extra()}
  println(m.render(box) + show(value(box)) + show(list.sort([2, 1])))
}
`
	uri := fileURI(filepath.Join(root, "app", "main.gfx"))
	c := client(t)
	if ds := diagnostics(t, c.notify("textDocument/didOpen", openParams(uri, source))); len(ds) != 0 {
		t.Fatal(ds)
	}
	for _, tc := range []struct {
		needle            string
		offset, line, col int
	}{
		{"m.Box)", 2, 3, 5},
		{"m.Box(n)", 2, 3, 11},
		{"m.Box{", 2, 3, 11},
		{"m.identity(n)", 2, 2, 5},
		{"m.render(box)", 2, 4, 18},
	} {
		data := c.request("textDocument/definition", positionParams(uri, source, tc.needle, tc.offset))
		var response struct {
			Result Location `json:"result"`
		}
		if err := json.Unmarshal(data, &response); err != nil {
			t.Fatal(err)
		}
		if response.Result.URI != fileURI(filepath.Join(root, "lib", "defs.gfx")) || response.Result.Range.Start != (Position{tc.line, tc.col}) {
			t.Fatalf("qualified definition %s: %s", tc.needle, data)
		}
	}
	hover := c.request("textDocument/hover", positionParams(uri, source, "m.identity(n)", 2))
	if !strings.Contains(string(hover), "identity : func(Int) Int") || !strings.Contains(string(hover), "Keeps the supplied value.") {
		t.Fatalf("qualified hover: %s", hover)
	}
	data := c.request("textDocument/definition", positionParams(uri, source, "list.sort", 5))
	var response struct {
		Result Location `json:"result"`
	}
	if err := json.Unmarshal(data, &response); err != nil {
		t.Fatal(err)
	}
	if response.Result.URI != "gofx-std:///std/list/list.gfx" {
		t.Fatalf("stdlib definition: %s", data)
	}
	data = c.request("gofx/stdlib", map[string]string{"uri": response.Result.URI})
	if !strings.Contains(string(data), "func sort[") {
		t.Fatalf("embedded source: %s", data)
	}
}

func TestImportedDiagnosticsClear(t *testing.T) {
	dir := t.TempDir()
	dep := filepath.Join(dir, "dep", "dep.gfx")
	if err := os.Mkdir(filepath.Dir(dep), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(dep, []byte("package dep\nfunc bad() Int { missing }\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	source := "package main\nimport \"dep\"\nfunc main() IO[Unit] { println(\"ok\") }\n"
	uri := fileURI(filepath.Join(dir, "main.gfx"))
	c := client(t)
	c.send(map[string]any{"jsonrpc": "2.0", "method": "textDocument/didOpen", "params": openParams(uri, source)})
	first, second := c.receive(), c.receive()
	if len(diagnostics(t, first))+len(diagnostics(t, second)) != 1 {
		t.Fatalf("missing imported diagnostic: %s %s", first, second)
	}
	if err := os.WriteFile(dep, []byte("package dep\nfunc bad() Int { 1 }\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	c.send(map[string]any{"jsonrpc": "2.0", "method": "textDocument/didChange", "params": changeParams(uri, source, 2)})
	first, second = c.receive(), c.receive()
	if len(diagnostics(t, first))+len(diagnostics(t, second)) != 0 {
		t.Fatalf("imported diagnostic not cleared: %s %s", first, second)
	}
}
