package lsp

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// TestSalesReportSession replays the real CLI stdio session recorded in
// docs/lsp-verification.md against the same framed Serve transport.
func TestSalesReportSession(t *testing.T) {
	dir, err := filepath.Abs("../examples/sales-report")
	if err != nil {
		t.Fatal(err)
	}
	read := func(name string) string {
		t.Helper()
		data, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			t.Fatal(err)
		}
		return string(data)
	}
	main, domain := read("main.gfx"), read("domain.gfx")
	mainURI, domainURI := fileURI(filepath.Join(dir, "main.gfx")), fileURI(filepath.Join(dir, "domain.gfx"))
	c := client(t)
	c.send(map[string]any{"jsonrpc": "2.0", "method": "initialized", "params": map[string]any{}})
	if ds := diagnostics(t, c.notify("textDocument/didOpen", openParams(mainURI, main))); len(ds) != 0 {
		t.Fatal(ds)
	}
	// Once both documents are open every check publishes both, in path order.
	published := func(wantVersion int, want []Diagnostic) {
		t.Helper()
		for _, uri := range []string{domainURI, mainURI} {
			data := c.receive()
			ds := diagnostics(t, data)
			var message struct {
				Params struct {
					URI     string `json:"uri"`
					Version int    `json:"version"`
				} `json:"params"`
			}
			if err := json.Unmarshal(data, &message); err != nil {
				t.Fatal(err)
			}
			version, expected := 1, []Diagnostic(nil)
			if uri == mainURI {
				version, expected = wantVersion, want
			}
			if message.Params.URI != uri || message.Params.Version != version || len(ds) != len(expected) || len(ds) > 0 && !reflect.DeepEqual(ds, expected) {
				t.Fatalf("diagnostics: %s; want uri=%s version=%d diagnostics=%+v", data, uri, version, expected)
			}
		}
	}
	c.send(map[string]any{"jsonrpc": "2.0", "method": "textDocument/didOpen", "params": openParams(domainURI, domain)})
	published(1, nil)
	hover := func(needle, want string, span Range) {
		t.Helper()
		data := c.request("textDocument/hover", positionParams(mainURI, main, needle, 0))
		var response struct {
			Result struct {
				Contents struct{ Kind, Value string } `json:"contents"`
				Range    Range                        `json:"range"`
			} `json:"result"`
		}
		if err := json.Unmarshal(data, &response); err != nil {
			t.Fatal(err)
		}
		if response.Result.Contents.Kind != "plaintext" || response.Result.Contents.Value != want || response.Result.Range != span {
			t.Fatalf("hover %s: %s", needle, data)
		}
	}
	hover("summarize(rates, sales, _.product)", "summarize : func(Map[Currency, Float], List[Sale], func(Sale) String) List[Row]", Range{Position{110, 32}, Position{110, 41}})
	hover("rates, sales, _.product)", "rates : Map[Currency, Float]", Range{Position{110, 42}, Position{110, 47}})
	definition := c.request("textDocument/definition", positionParams(mainURI, main, "parseCsv(text)", 0))
	var target struct {
		Result Location `json:"result"`
	}
	if err := json.Unmarshal(definition, &target); err != nil {
		t.Fatal(err)
	}
	if target.Result != (Location{domainURI, Range{Position{69, 5}, Position{69, 13}}}) {
		t.Fatalf("cross-file definition: %s", definition)
	}
	symbols := c.request("textDocument/documentSymbol", map[string]any{"textDocument": map[string]any{"uri": mainURI}})
	var outline struct {
		Result []DocumentSymbol `json:"result"`
	}
	if err := json.Unmarshal(symbols, &outline); err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, symbol := range outline.Result {
		names = append(names, symbol.Name)
		if symbol.Name == "summarize" && (symbol.Kind != 12 || symbol.SelectionRange != (Range{Position{56, 5}, Position{56, 14}})) {
			t.Fatalf("summarize symbol: %+v", symbol)
		}
	}
	if !reflect.DeepEqual(names, []string{"fetchRate", "fetchRates", "Row", "amountJpy", "summarize", "table", "sampleCsv", "loadCsv", "main"}) {
		t.Fatalf("outline: %s", symbols)
	}
	unused := strings.Replace(main, "  do {\n    argv", "  let unused = 1\n  do {\n    argv", 1)
	c.send(map[string]any{"jsonrpc": "2.0", "method": "textDocument/didChange", "params": changeParams(mainURI, unused, 2)})
	published(2, []Diagnostic{{Range{Position{100, 6}, Position{100, 12}}, 2, "gofx", "unused is unused; prefix with _ to silence"}})
	broken := strings.Replace(main, "parseCsv(text)", "parseCsv(123)", 1)
	c.send(map[string]any{"jsonrpc": "2.0", "method": "textDocument/didChange", "params": changeParams(mainURI, broken, 3)})
	published(3, []Diagnostic{{Range{Position{104, 22}, Position{104, 25}}, 1, "gofx", "type mismatch: expected String, got Int"}})
	for _, method := range []string{"hover", "definition"} {
		data := c.request("textDocument/"+method, positionParams(mainURI, broken, "summarize(rates, sales, _.product)", 0))
		if !strings.Contains(string(data), `"result":null`) {
			t.Fatalf("stale %s index: %s", method, data)
		}
	}
	c.send(map[string]any{"jsonrpc": "2.0", "method": "textDocument/didChange", "params": changeParams(mainURI, main, 4)})
	published(4, nil)
	hover("summarize(rates, sales, _.product)", "summarize : func(Map[Currency, Float], List[Sale], func(Sale) String) List[Row]", Range{Position{110, 32}, Position{110, 41}})
	if data := c.request("shutdown", map[string]any{}); !strings.Contains(string(data), `"result":null`) {
		t.Fatalf("shutdown: %s", data)
	}
	c.send(map[string]any{"jsonrpc": "2.0", "method": "exit"})
}
