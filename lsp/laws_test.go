package lsp

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"go4t/format"
)

func TestLawDiagnosticsAndSymbols(t *testing.T) {
	source, err := os.ReadFile("../laws/testdata/cancel_bad1.gt")
	if err != nil {
		t.Fatal(err)
	}
	text := strings.Replace(string(source), "ioPure(unit())", "let unused = 1; ioPure(unit())", 1)
	file := filepath.Join(t.TempDir(), "cancel_bad1.gt")
	if err := os.WriteFile(file, []byte(text), 0o644); err != nil {
		t.Fatal(err)
	}
	uri := fileURI(file)
	c := client(t)
	data := c.notify("textDocument/didOpen", openParams(uri, text))
	var published struct {
		Params struct {
			URI string `json:"uri"`
		} `json:"params"`
	}
	if err := json.Unmarshal(data, &published); err != nil {
		t.Fatal(err)
	}
	if published.Params.URI != uri {
		t.Fatalf("diagnostic URI = %s, want %s", published.Params.URI, uri)
	}
	ds := diagnostics(t, data)
	if len(ds) != 2 {
		t.Fatalf("diagnostics = %+v, want warning and law error", ds)
	}
	if ds[0].Severity != 2 || ds[0].Message != "unused is unused; prefix with _ to silence" {
		t.Fatalf("warning = %+v", ds[0])
	}
	if ds[1].Severity != 1 || ds[1].Source != "go4t" || !strings.HasPrefix(ds[1].Message, "law cancelIdempotent: ") || ds[1].Range.Start != (Position{16, 52}) || ds[1].Range.End != (Position{16, 56}) {
		t.Fatalf("law diagnostic = %+v", ds[1])
	}
	data = c.request("textDocument/documentSymbol", map[string]any{"textDocument": map[string]any{"uri": uri}})
	var symbols struct {
		Result []DocumentSymbol `json:"result"`
	}
	if err := json.Unmarshal(data, &symbols); err != nil {
		t.Fatal(err)
	}
	counts := map[string]int{}
	for _, symbol := range symbols.Result {
		if symbol.Name == "cancelIdempotent" || symbol.Name == "cancelPending" {
			counts[symbol.Name]++
			if symbol.Kind != 12 {
				t.Errorf("law/proof kind = %d, want Function (12)", symbol.Kind)
			}
		}
	}
	if counts["cancelIdempotent"] != 2 || counts["cancelPending"] != 2 {
		t.Fatalf("missing law/proof symbols: %s", data)
	}
	fixed := strings.Replace(text, "Cancelled(n) => Pending(n)", "Cancelled(n) => Cancelled(n)", 1)
	ds = diagnostics(t, c.notify("textDocument/didChange", changeParams(uri, fixed, 2)))
	if len(ds) != 1 || ds[0].Severity != 2 {
		t.Fatalf("fixed proof must clear law error, preserving warning: %+v", ds)
	}
}

func TestSampleInductionRoundTrip(t *testing.T) {
	text := `package main
import "std/regex"
func app(xs List[Int], ys List[Int]) List[Int] { match xs { Nil => ys; Cons(x, rest) => Cons(x, app(rest, ys)) } }
law identity(xs List[Int]) { app(xs, []) == xs }
proof identity(xs) { induction xs { Nil => refl; Cons(_, rest) => refl } }
law bare(xs List[Int]) { app(xs, []) == xs }
proof bare(xs) { induction xs }
law replacement(s String) { regex.regexReplace("z", s, "z") == s }
proof replacement(s) { sample(8) }
func main() IO[Unit] { ioPure(unit()) }
`
	file := filepath.Join(t.TempDir(), "laws.gt")
	if err := os.WriteFile(file, []byte(text), 0o644); err != nil {
		t.Fatal(err)
	}
	c := client(t)
	uri := fileURI(file)
	if ds := diagnostics(t, c.notify("textDocument/didOpen", openParams(uri, text))); len(ds) != 0 {
		t.Fatalf("new tactics: %+v", ds)
	}
	formatted, err := format.Source([]byte(text))
	if err != nil {
		t.Fatal(err)
	}
	if ds := diagnostics(t, c.notify("textDocument/didChange", changeParams(uri, string(formatted), 2))); len(ds) != 0 {
		t.Fatalf("formatted tactics: %+v", ds)
	}
	response := c.request("textDocument/documentSymbol", map[string]any{"textDocument": map[string]any{"uri": uri}})
	var symbols struct {
		Result []DocumentSymbol `json:"result"`
	}
	if err := json.Unmarshal(response, &symbols); err != nil {
		t.Fatal(err)
	}
	counts := map[string]int{}
	for _, symbol := range symbols.Result {
		counts[symbol.Name]++
		if strings.Contains(symbol.Name, "induction") {
			t.Fatalf("synthetic binder exposed: %+v", symbol)
		}
	}
	for _, name := range []string{"identity", "bare", "replacement"} {
		if counts[name] != 2 {
			t.Fatalf("%s symbols: %v", name, counts)
		}
	}
}
