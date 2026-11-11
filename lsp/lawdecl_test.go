package lsp

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLawImplicitAndInlineTooling(t *testing.T) {
	const source = `package main
law implicit(a Int) { a + 1 == 1 + a }
law inline(a Bool) { a || !a } proof { if a { refl } else { refl } }
func main() IO[Unit] { ioPure(unit()) }
`
	file := filepath.Join(t.TempDir(), "laws.gt")
	if err := os.WriteFile(file, []byte(source), 0o600); err != nil {
		t.Fatal(err)
	}
	uri := fileURI(file)
	c := client(t)
	if ds := diagnostics(t, c.notify("textDocument/didOpen", openParams(uri, source))); len(ds) != 0 {
		t.Fatal(ds)
	}
	data := c.request("textDocument/documentSymbol", map[string]any{"textDocument": map[string]any{"uri": uri}})
	var symbols struct {
		Result []DocumentSymbol `json:"result"`
	}
	if err := json.Unmarshal(data, &symbols); err != nil {
		t.Fatal(err)
	}
	counts := map[string]int{}
	for _, symbol := range symbols.Result {
		counts[symbol.Name]++
	}
	if counts["implicit"] != 1 || counts["inline"] != 1 {
		t.Fatalf("law symbols = %s", data)
	}
	for _, tc := range []struct {
		needle string
		offset int
		want   string
	}{
		{"implicit(a", 0, "func(Int) Int"},
		{"if a", 3, "a : Bool"},
		{"proof {", 0, `"result":null`},
	} {
		data := c.request("textDocument/hover", positionParams(uri, source, tc.needle, tc.offset))
		if !strings.Contains(string(data), tc.want) {
			t.Errorf("hover %s: %s", tc.needle, data)
		}
	}
	bad := strings.Replace(source, "a + 1 == 1 + a", "a + 1 == a", 1)
	ds := diagnostics(t, c.notify("textDocument/didChange", changeParams(uri, bad, 2)))
	if len(ds) != 1 || ds[0].Range.Start.Line != 1 || !strings.HasSuffix(ds[0].Message, "no proof was written for this law; it was checked by refl") {
		t.Fatalf("implicit diagnostic = %+v", ds)
	}
}
