package lsp

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
)

func TestExpectedTypeHover(t *testing.T) {
	source := `package main
type Sale = Sale{product: String, qty: Int}
func productKey() Option[func(Sale) String] { Some(_.product) }
func qtyKey() func(Sale) Int { match true { true => _.qty; false => _.qty } }
func main() IO[Unit] {
  let sale = Sale{product: "tea", qty: 2}
  let project: func(Sale) String = _.product
  let key: func(Sale) String = func(s) => s.product
  println(project(sale) + key(sale) + show(qtyKey()(sale)))
}
`
	uri := fileURI(filepath.Join(t.TempDir(), "main.gt"))
	c := client(t)
	if ds := diagnostics(t, c.notify("textDocument/didOpen", openParams(uri, source))); len(ds) != 0 {
		t.Fatal(ds)
	}
	for _, tc := range []struct{ needle, want string }{
		{"productKey()", "productKey : func() Option[func(Sale) String]"},
		{"project:", "project : func(Sale) String"},
		{"key:", "key : func(Sale) String"},
		{"s.product", "s : Sale"},
	} {
		data := c.request("textDocument/hover", positionParams(uri, source, tc.needle, 0))
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
}
