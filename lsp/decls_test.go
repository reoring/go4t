package lsp

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
)

func TestDeclsConstantHover(t *testing.T) {
	source := `package main
type Row = {qty: Int}
let limit = 3
let row: Row = Row{qty: limit}
func main() IO[Unit] { println(show(row.qty)) }
`
	uri := fileURI(filepath.Join(t.TempDir(), "main.gt"))
	c := client(t)
	if ds := diagnostics(t, c.notify("textDocument/didOpen", openParams(uri, source))); len(ds) != 0 {
		t.Fatal(ds)
	}
	for _, tc := range []struct{ needle, want string }{
		{"limit =", "limit : Int"},
		{"limit}", "limit : Int"},
		{"row.qty", "row : Row"},
	} {
		data := c.request("textDocument/hover", positionParams(uri, source, tc.needle, 0))
		var response struct {
			Result struct{ Contents struct{ Value string } }
		}
		if err := json.Unmarshal(data, &response); err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(response.Result.Contents.Value, tc.want) {
			t.Errorf("hover %s: %s", tc.needle, data)
		}
	}
	data := c.request("textDocument/definition", positionParams(uri, source, "limit}", 0))
	var response struct {
		Result struct{ Range struct{ Start Position } }
	}
	if err := json.Unmarshal(data, &response); err != nil {
		t.Fatal(err)
	}
	if response.Result.Range.Start.Line != 2 {
		t.Fatalf("definition: %s", data)
	}
}
