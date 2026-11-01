package lsp

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestHTTPServiceSession replays the framed stdio exchange documented in
// docs/service-walkthrough.md against the real three-file example module.
func TestHTTPServiceSession(t *testing.T) {
	t.Setenv("GOFX_PATH", "")
	dir, err := filepath.Abs("../examples/http-service")
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
	tests, service := read("service_test.gfx"), read("service.gfx")
	testsURI := fileURI(filepath.Join(dir, "service_test.gfx"))
	serviceURI := fileURI(filepath.Join(dir, "service.gfx"))
	c := client(t)
	c.send(map[string]any{"jsonrpc": "2.0", "method": "initialized", "params": map[string]any{}})
	if ds := diagnostics(t, c.notify("textDocument/didOpen", openParams(testsURI, tests))); len(ds) != 0 {
		t.Fatal(ds)
	}
	c.send(map[string]any{"jsonrpc": "2.0", "method": "textDocument/didOpen", "params": openParams(serviceURI, service)})
	for _, uri := range []string{serviceURI, testsURI} {
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
		if message.Params.URI != uri || message.Params.Version != 1 || len(ds) != 0 {
			t.Fatalf("diagnostics: %s; want uri=%s version=1 and no diagnostics", data, uri)
		}
	}
	position := positionParams(testsURI, tests, "aggregate(sampleOrders())", 0)
	data := c.request("textDocument/hover", position)
	var hover struct {
		Result struct {
			Contents struct{ Kind, Value string } `json:"contents"`
			Range    Range                        `json:"range"`
		} `json:"result"`
	}
	if err := json.Unmarshal(data, &hover); err != nil {
		t.Fatal(err)
	}
	if hover.Result.Contents.Kind != "plaintext" || hover.Result.Contents.Value != "aggregate : func(List[Order]) Map[String, Float]" || hover.Result.Range != (Range{Position{9, 2}, Position{9, 11}}) {
		t.Fatalf("aggregate hover: %s", data)
	}
	data = c.request("textDocument/definition", position)
	var definition struct {
		Result Location `json:"result"`
	}
	if err := json.Unmarshal(data, &definition); err != nil {
		t.Fatal(err)
	}
	if definition.Result != (Location{serviceURI, Range{Position{14, 5}, Position{14, 14}}}) {
		t.Fatalf("cross-file aggregate definition: %s", data)
	}
	if data := c.request("shutdown", map[string]any{}); !strings.Contains(string(data), `"result":null`) {
		t.Fatalf("shutdown: %s", data)
	}
	c.send(map[string]any{"jsonrpc": "2.0", "method": "exit"})
}
