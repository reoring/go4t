package lsp

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
)

func TestIOTailLiftHover(t *testing.T) {
	source := `package main
func value(n Int) IO[Int] { n }
func literal() IO[Int] { 42 }
func main() IO[Unit] {
  let action: IO[Int] = 7
  do { n <- action; println(show(n)) }
}
`
	uri := fileURI(filepath.Join(t.TempDir(), "main.gt"))
	c := client(t)
	if ds := diagnostics(t, c.notify("textDocument/didOpen", openParams(uri, source))); len(ds) != 0 {
		t.Fatal(ds)
	}
	for _, tc := range []struct {
		needle string
		offset int
		want   string
	}{
		{"n Int", 0, "n : Int"},
		{"{ n }", 2, "n : IO[Int]"},
		{"42", 0, "IO[Int]"},
		{"action:", 0, "action : IO[Int]"},
		{"7", 0, "IO[Int]"},
	} {
		data := c.request("textDocument/hover", positionParams(uri, source, tc.needle, tc.offset))
		var message struct {
			Result struct {
				Contents struct{ Value string }
			}
		}
		if err := json.Unmarshal(data, &message); err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(message.Result.Contents.Value, tc.want) {
			t.Errorf("hover %s: %s, want %q", tc.needle, data, tc.want)
		}
	}
	data := c.request("textDocument/definition", positionParams(uri, source, "{ n }", 2))
	var definition struct {
		Result Location
	}
	if err := json.Unmarshal(data, &definition); err != nil {
		t.Fatal(err)
	}
	if definition.Result.URI != uri || definition.Result.Range.Start != (Position{Line: 1, Character: 11}) {
		t.Fatalf("lifted use lost its parameter definition: %s", data)
	}
}
