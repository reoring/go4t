package lsp

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
)

func TestPolymorphicLawHover(t *testing.T) {
	const source = `package main
law appendNil[A](xs List[A]) { append(xs, Nil) == xs }
proof appendNil(xs) { induction xs }
law use(xs List[Int]) { append(xs, Nil) == xs }
proof use(xs) { rewrite appendNil(xs) { refl } }
func main() IO[Unit] { ioPure(unit()) }
`
	uri := fileURI(filepath.Join(t.TempDir(), "main.gt"))
	c := client(t)
	if ds := diagnostics(t, c.notify("textDocument/didOpen", openParams(uri, source))); len(ds) != 0 {
		t.Fatal(ds)
	}
	for _, tt := range []struct {
		needle string
		offset int
		want   string
	}{
		{"law appendNil", 4, "func(List[A]) List[A]"},
		{"rewrite appendNil", 8, "func(List[Int]) List[Int]"},
		{"List[A]", 5, "A"},
	} {
		data := c.request("textDocument/hover", positionParams(uri, source, tt.needle, tt.offset))
		var response struct {
			Result *struct {
				Contents struct{ Value string }
			}
		}
		if err := json.Unmarshal(data, &response); err != nil {
			t.Fatal(err)
		}
		if response.Result == nil || !strings.Contains(response.Result.Contents.Value, tt.want) {
			t.Fatalf("hover %s: %s, want %s", tt.needle, data, tt.want)
		}
	}
}
