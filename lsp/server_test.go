package lsp

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

type testClient struct {
	t          *testing.T
	input      *io.PipeWriter
	output     *bufio.Reader
	done       chan error
	initialize json.RawMessage
}

func client(t *testing.T) *testClient {
	t.Helper()
	in, input := io.Pipe()
	output, out := io.Pipe()
	c := &testClient{t: t, input: input, output: bufio.NewReader(output), done: make(chan error, 1)}
	go func() { c.done <- Serve(in, out); out.Close(); in.Close() }()
	t.Cleanup(func() {
		input.Close()
		output.Close()
		select {
		case <-c.done:
		case <-time.After(5 * time.Second):
			t.Error("server did not stop")
		}
	})
	c.send(map[string]any{"jsonrpc": "2.0", "id": 1, "method": "initialize", "params": map[string]any{}})
	response := c.receive()
	c.initialize = response
	if !strings.Contains(string(response), `"textDocumentSync":1`) {
		t.Fatalf("initialize: %s", response)
	}
	return c
}
func (c *testClient) send(value any) {
	c.t.Helper()
	data, err := json.Marshal(value)
	if err != nil {
		c.t.Fatal(err)
	}
	if _, err = fmt.Fprintf(c.input, "Content-Length: %d\r\n\r\n%s", len(data), data); err != nil {
		c.t.Fatal(err)
	}
}
func (c *testClient) receive() json.RawMessage {
	c.t.Helper()
	type result struct {
		data []byte
		err  error
	}
	done := make(chan result, 1)
	go func() { data, err := readMessage(c.output); done <- result{data, err} }()
	select {
	case r := <-done:
		if r.err != nil {
			c.t.Fatal(r.err)
		}
		return r.data
	case <-time.After(5 * time.Second):
		c.t.Fatal("timed out reading server")
		return nil
	}
}
func (c *testClient) notify(method string, params any) json.RawMessage {
	c.send(map[string]any{"jsonrpc": "2.0", "method": method, "params": params})
	return c.receive()
}
func (c *testClient) request(method string, params any) json.RawMessage {
	c.send(map[string]any{"jsonrpc": "2.0", "id": 2, "method": method, "params": params})
	return c.receive()
}
func openParams(uri, text string) any {
	return map[string]any{"textDocument": map[string]any{"uri": uri, "languageId": "gofx", "version": 1, "text": text}}
}
func changeParams(uri, text string, version int) any {
	return map[string]any{"textDocument": map[string]any{"uri": uri, "version": version}, "contentChanges": []any{map[string]any{"text": text}}}
}
func positionParams(uri, source, needle string, offset int) any {
	i := strings.Index(source, needle) + offset
	before := source[:i]
	line := strings.Count(before, "\n")
	col := len([]rune(before[strings.LastIndex(before, "\n")+1:]))
	return map[string]any{"textDocument": map[string]any{"uri": uri}, "position": Position{Line: line, Character: col}}
}
func diagnostics(t *testing.T, data []byte) []Diagnostic {
	t.Helper()
	var message struct {
		Params struct {
			Diagnostics []Diagnostic `json:"diagnostics"`
		} `json:"params"`
	}
	if err := json.Unmarshal(data, &message); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), `"method":"textDocument/publishDiagnostics"`) {
		t.Fatalf("not diagnostics: %s", data)
	}
	return message.Params.Diagnostics
}
func TestDiagnosticsOpenChangeClose(t *testing.T) {
	c := client(t)
	uri := fileURI(filepath.Join(t.TempDir(), "main.gfx"))
	bad := "package main\nfunc main() IO[Unit] { println(123) }\n"
	ds := diagnostics(t, c.notify("textDocument/didOpen", openParams(uri, bad)))
	if len(ds) != 1 || ds[0].Severity != 1 || ds[0].Message != "type mismatch: expected String, got Int" || ds[0].Range.Start != (Position{1, 31}) || ds[0].Range.End != (Position{1, 34}) {
		t.Fatalf("diagnostics: %+v", ds)
	}
	good := "package main\nfunc main() IO[Unit] { let unused = 1; println(\"ok\") }\n"
	ds = diagnostics(t, c.notify("textDocument/didChange", changeParams(uri, good, 2)))
	if len(ds) != 1 || ds[0].Severity != 2 || ds[0].Message != "unused is unused; prefix with _ to silence" {
		t.Fatalf("warnings: %+v", ds)
	}
	broken := "package main\nfunc main("
	ds = diagnostics(t, c.notify("textDocument/didChange", changeParams(uri, broken, 3)))
	if len(ds) != 1 || ds[0].Severity != 1 || !strings.Contains(ds[0].Message, "expected") {
		t.Fatalf("parse error: %+v", ds)
	}
	ds = diagnostics(t, c.notify("textDocument/didClose", map[string]any{"textDocument": map[string]any{"uri": uri}}))
	if len(ds) != 0 {
		t.Fatalf("close: %+v", ds)
	}
}
func TestHoverDefinitionAndSymbols(t *testing.T) {
	dir := t.TempDir()
	defs := "package main\n// Keeps its argument.\nfunc identity[A](value A) A { value }\n// A record.\ntype Box = Box{value: Int}\nclass Render[A] { render(value A) String }\ninstance Render[Box] { render(value) { show(value.value) } }\nalias Count = Int\nfunc one() Count { 1 }\n"
	if err := os.WriteFile(filepath.Join(dir, "defs.gfx"), []byte(defs), 0600); err != nil {
		t.Fatal(err)
	}
	src := "package main\nfunc main() IO[Unit] { let box = Box{value: identity(one())}; println(render(box)); println(show(box.value)) }\n"
	uri := fileURI(filepath.Join(dir, "main.gfx"))
	c := client(t)
	if ds := diagnostics(t, c.notify("textDocument/didOpen", openParams(uri, src))); len(ds) != 0 {
		t.Fatal(ds)
	}
	for _, tt := range []struct{ needle, want string }{{"identity(one", "func(Int) Int"}, {"one())", "func() Int"}, {"box.value", "Box"}} {
		data := c.request("textDocument/hover", positionParams(uri, src, tt.needle, 0))
		if !strings.Contains(string(data), tt.want) {
			t.Fatalf("hover %s: %s", tt.needle, data)
		}
		if tt.needle == "identity(one" && !strings.Contains(string(data), "Keeps its argument.") {
			t.Fatalf("missing doc: %s", data)
		}
	}
	for _, tt := range []struct {
		needle            string
		offset, line, col int
	}{{"identity(one", 0, 2, 5}, {"Box{", 0, 4, 11}, {"box.value", 4, 4, 15}, {"render(box)", 0, 5, 18}} {
		data := c.request("textDocument/definition", positionParams(uri, src, tt.needle, tt.offset))
		var message struct {
			Result Location `json:"result"`
		}
		json.Unmarshal(data, &message)
		if message.Result.URI != fileURI(filepath.Join(dir, "defs.gfx")) || message.Result.Range.Start != (Position{tt.line, tt.col}) {
			t.Fatalf("definition %s: %s", tt.needle, data)
		}
	}
	defsURI := fileURI(filepath.Join(dir, "defs.gfx"))
	data := c.request("textDocument/definition", positionParams(defsURI, defs, "Count {", 0))
	if !strings.Contains(string(data), `"line":7`) {
		t.Fatalf("alias definition: %s", data)
	}
	data = c.request("textDocument/hover", positionParams(defsURI, defs, "identity[A]", 0))
	if !strings.Contains(string(data), "func(A) A") {
		t.Fatalf("polymorphic declaration hover: %s", data)
	}
	data = c.request("textDocument/definition", positionParams(defsURI, defs, "Render[Box]", 7))
	var typeDefinition struct {
		Result Location `json:"result"`
	}
	if err := json.Unmarshal(data, &typeDefinition); err != nil {
		t.Fatal(err)
	}
	if typeDefinition.Result.Range.Start != (Position{4, 5}) {
		t.Fatalf("type definition: %s", data)
	}
	data = c.request("textDocument/documentSymbol", map[string]any{"textDocument": map[string]any{"uri": defsURI}})
	var message struct {
		Result []DocumentSymbol `json:"result"`
	}
	if err := json.Unmarshal(data, &message); err != nil {
		t.Fatal(err)
	}
	topKinds := map[string]int{}
	for _, symbol := range message.Result {
		topKinds[symbol.Name] = symbol.Kind
	}
	if topKinds["Box"] != 23 || topKinds["Render[Box]"] != 3 {
		t.Fatalf("top-level symbol kinds: %v", topKinds)
	}
	kinds := map[string]int{}
	var collect func([]DocumentSymbol)
	collect = func(ss []DocumentSymbol) {
		for _, s := range ss {
			kinds[s.Name] = s.Kind
			collect(s.Children)
		}
	}
	collect(message.Result)
	for name, kind := range map[string]int{"identity": 12, "Box": 9, "value": 8, "Render": 11, "render": 6, "Count": 26, "one": 12} {
		if kinds[name] != kind {
			t.Errorf("symbol %s kind=%d want=%d; %s", name, kinds[name], kind, data)
		}
	}
}

func TestScriptedSession(t *testing.T) {
	path, err := filepath.Abs("../examples/wc/main.gfx")
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	source, uri := string(data), fileURI(path)
	c := client(t)
	t.Logf("initialize: %s", c.initialize)
	opened := c.notify("textDocument/didOpen", openParams(uri, source))
	if ds := diagnostics(t, opened); len(ds) != 0 {
		t.Fatal(ds)
	}
	t.Logf("didOpen: %s", opened)
	broken := strings.Replace(source, "fold(tokens", "missingFold(tokens", 1)
	changed := c.notify("textDocument/didChange", changeParams(uri, broken, 2))
	ds := diagnostics(t, changed)
	if len(ds) != 1 || ds[0].Message != "unknown identifier missingFold" {
		t.Fatal(ds)
	}
	t.Logf("injected error: %s", changed)
	restored := c.notify("textDocument/didChange", changeParams(uri, source, 3))
	if ds := diagnostics(t, restored); len(ds) != 0 {
		t.Fatal(ds)
	}
	t.Logf("restored: %s", restored)
	hover := c.request("textDocument/hover", positionParams(uri, source, "fold(tokens", 0))
	if !strings.Contains(string(hover), "fold : func(") {
		t.Fatal(string(hover))
	}
	t.Logf("hover fold: %s", hover)
	definition := c.request("textDocument/definition", positionParams(uri, source, "increment(acc", 0))
	line := strings.Count(source[:strings.Index(source, "func increment")], "\n")
	if !strings.Contains(string(definition), fmt.Sprintf(`"line":%d,"character":5`, line)) {
		t.Fatal(string(definition))
	}
	t.Logf("definition increment: %s", definition)
}

func TestProtocolLifecycleAndErrors(t *testing.T) {
	c := client(t)
	for _, tt := range []struct {
		method string
		params any
		code   int
	}{
		{"unknown", nil, -32601},
		{"initialize", nil, -32600},
		{"textDocument/hover", map[string]any{"textDocument": map[string]any{"uri": "https://example.com/main.gfx"}}, -32602},
	} {
		data := c.request(tt.method, tt.params)
		if !strings.Contains(string(data), fmt.Sprintf(`"code":%d`, tt.code)) {
			t.Fatalf("%s: %s", tt.method, data)
		}
	}
	shutdown := c.request("shutdown", nil)
	if !strings.Contains(string(shutdown), `"result":null`) {
		t.Fatal(string(shutdown))
	}
	data := c.request("textDocument/hover", nil)
	if !strings.Contains(string(data), `"code":-32600`) {
		t.Fatal(string(data))
	}
	c.send(map[string]any{"jsonrpc": "2.0", "method": "exit"})
}

func TestUTF16Positions(t *testing.T) {
	c := client(t)
	source := "package main\r\nfunc main() IO[Unit] { println(\"😀\"); println(missing) }\r\n"
	uri := fileURI(filepath.Join(t.TempDir(), "unicode.gfx"))
	ds := diagnostics(t, c.notify("textDocument/didOpen", openParams(uri, source)))
	if len(ds) != 1 || ds[0].Range.Start != (Position{1, 46}) || ds[0].Range.End != (Position{1, 53}) {
		t.Fatalf("UTF-16 range: %+v", ds)
	}
	for _, pos := range []Position{{1, 33}, {-1, 0}, {100, 0}} {
		if _, ok := sourcePosition(source, pos); ok {
			t.Errorf("invalid position accepted: %+v", pos)
		}
	}
}

func TestFramingBoundaries(t *testing.T) {
	for _, input := range []string{"Content-Length: -1\r\n\r\n", "Content-Length: 16777217\r\n\r\n", "Other: x\r\n\r\n", "Content-Length: 1\r\nContent-Length: 1\r\n\r\nx", "Content-Length: 2\r\n\r\nx"} {
		if _, err := readMessage(bufio.NewReader(strings.NewReader(input))); err == nil {
			t.Errorf("accepted %q", input)
		}
	}
	var out strings.Builder
	bad := "{"
	err := Serve(strings.NewReader(fmt.Sprintf("Content-Length: %d\r\n\r\n%s", len(bad), bad)), &out)
	if err != nil || !strings.Contains(out.String(), `"code":-32700`) {
		t.Fatalf("parse error: %v %s", err, out.String())
	}
}

func TestOverlaysVersionsAndCloseReadDisk(t *testing.T) {
	dir := t.TempDir()
	defs := "package main\nfunc one() Int { 1 }\n"
	defsPath := filepath.Join(dir, "defs.gfx")
	if err := os.WriteFile(defsPath, []byte(defs), 0600); err != nil {
		t.Fatal(err)
	}
	source := "package main\nfunc main() IO[Unit] { println(show(one())) }\n"
	uri, defsURI := fileURI(filepath.Join(dir, "main.gfx")), fileURI(defsPath)
	c := client(t)
	if ds := diagnostics(t, c.notify("textDocument/didOpen", openParams(uri, source))); len(ds) != 0 {
		t.Fatal(ds)
	}
	overlay := "package main\nfunc one() String { \"one\" }\n"
	c.send(map[string]any{"jsonrpc": "2.0", "method": "textDocument/didOpen", "params": openParams(defsURI, overlay)})
	for range 2 {
		if ds := diagnostics(t, c.receive()); len(ds) != 0 {
			t.Fatal(ds)
		}
	}
	got := c.request("textDocument/hover", positionParams(uri, source, "one()", 0))
	if !strings.Contains(string(got), "func() String") {
		t.Fatal(string(got))
	}
	c.send(map[string]any{"jsonrpc": "2.0", "method": "textDocument/didChange", "params": changeParams(defsURI, defs, 1)})
	got = c.request("textDocument/hover", positionParams(uri, source, "one()", 0))
	if !strings.Contains(string(got), "func() String") {
		t.Fatalf("stale version applied: %s", got)
	}
	c.send(map[string]any{"jsonrpc": "2.0", "method": "textDocument/didClose", "params": map[string]any{"textDocument": map[string]any{"uri": defsURI}}})
	for range 2 {
		if ds := diagnostics(t, c.receive()); len(ds) != 0 {
			t.Fatal(ds)
		}
	}
	got = c.request("textDocument/hover", positionParams(uri, source, "one()", 0))
	if !strings.Contains(string(got), "func() Int") {
		t.Fatalf("disk not restored: %s", got)
	}
}

func TestFailedCheckDiscardsIndexAndLexerSpan(t *testing.T) {
	c := client(t)
	uri := fileURI(filepath.Join(t.TempDir(), "main.gfx"))
	source := "package main\nfunc main() IO[Unit] { println(\"ok\") }\n"
	c.notify("textDocument/didOpen", openParams(uri, source))
	ds := diagnostics(t, c.notify("textDocument/didChange", changeParams(uri, strings.Replace(source, "println", "missing", 1), 2)))
	if len(ds) != 1 {
		t.Fatal(ds)
	}
	got := c.request("textDocument/hover", positionParams(uri, source, "println", 0))
	if !strings.Contains(string(got), `"result":null`) {
		t.Fatalf("stale index: %s", got)
	}
	ds = diagnostics(t, c.notify("textDocument/didChange", changeParams(uri, "package main\n#\n", 3)))
	if len(ds) != 1 || ds[0].Range != (Range{Position{1, 0}, Position{1, 1}}) || ds[0].Message != "unexpected character '#'" {
		t.Fatalf("lexical diagnostics: %+v", ds)
	}
}
