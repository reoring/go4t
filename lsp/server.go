package lsp

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"go4t/ast"
	"go4t/laws"
	"go4t/modules"
	"go4t/parser"
	"go4t/std"
	"go4t/types"
)

const stdScheme = "go4t-std"

type document struct {
	uri, text string
	version   int
}
type snapshot struct {
	sources     map[string]string
	index       *types.PositionIndex
	program     *types.Program
	diagnostics map[string][]Diagnostic
}
type server struct {
	out                   io.Writer
	initialized, shutdown bool
	documents             map[string]document
	programs              map[string]*snapshot
	goBinder              func(directory string) modules.GoBinder
}

// Options supplies a fresh import binder for each checked source directory.
type Options struct {
	GoBinder func(directory string) modules.GoBinder
}

// Serve reads Content-Length framed JSON-RPC messages until exit or EOF. It
// writes only protocol messages to out. The caller owns both streams. Requests
// are processed in order, so every response observes all preceding full changes.
func Serve(in io.Reader, out io.Writer, options ...Options) error {
	s := &server{out: out, documents: map[string]document{}, programs: map[string]*snapshot{}}
	for _, option := range options {
		s.goBinder = option.GoBinder
	}
	reader := bufio.NewReader(in)
	for {
		data, err := readMessage(reader)
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
		var message rpcMessage
		if !json.Valid(data) {
			if err = s.reply(nil, nil, &rpcError{-32700, "Parse error"}); err != nil {
				return err
			}
			continue
		}
		if err = json.Unmarshal(data, &message); err != nil || message.JSONRPC != "2.0" || message.Method == "" || !validID(message.ID) {
			if err = s.reply(nil, nil, &rpcError{-32600, "Invalid Request"}); err != nil {
				return err
			}
			continue
		}
		if message.Method == "exit" {
			if s.shutdown {
				return nil
			}
			return errors.New("LSP exit before shutdown")
		}
		if err = s.handle(message); err != nil {
			return err
		}
	}
}
func validID(id json.RawMessage) bool {
	if len(id) == 0 {
		return true
	}
	var value any
	if json.Unmarshal(id, &value) != nil {
		return false
	}
	switch value.(type) {
	case string, float64:
		return true
	}
	return false
}
func (s *server) handle(m rpcMessage) error {
	request := len(m.ID) != 0
	fail := func(code int, text string) error {
		if !request {
			return nil
		}
		return s.reply(m.ID, nil, &rpcError{code, text})
	}
	if s.shutdown {
		return fail(-32600, "Server has shut down")
	}
	if m.Method == "initialize" {
		if !request || s.initialized {
			return fail(-32600, "Server already initialized")
		}
		s.initialized = true
		return s.reply(m.ID, map[string]any{"capabilities": map[string]any{"positionEncoding": "utf-16", "textDocumentSync": 1, "hoverProvider": true, "definitionProvider": true, "documentSymbolProvider": true}, "serverInfo": map[string]string{"name": "go4t"}}, nil)
	}
	if !s.initialized {
		return fail(-32002, "Server not initialized")
	}
	if m.Method == "shutdown" {
		if !request {
			return nil
		}
		s.shutdown = true
		return s.reply(m.ID, nil, nil)
	}
	if m.Method == "initialized" || m.Method == "$/cancelRequest" || m.Method == "$/setTrace" {
		return nil
	}
	if m.Method == "go4t/stdlib" {
		if request {
			var params struct {
				URI string `json:"uri"`
			}
			if json.Unmarshal(m.Params, &params) != nil {
				return fail(-32602, "Invalid params")
			}
			uri, err := url.Parse(params.URI)
			if err != nil || uri.Scheme != stdScheme || uri.Host != "" || uri.RawQuery != "" || uri.Fragment != "" || !strings.HasPrefix(uri.Path, "/std/") {
				return fail(-32602, "expected an embedded std URI")
			}
			source, err := std.FS.ReadFile(strings.TrimPrefix(uri.Path, "/std/"))
			if err != nil {
				return fail(-32602, "unknown embedded std source")
			}
			return s.reply(m.ID, string(source), nil)
		}
		return nil
	}
	switch m.Method {
	case "textDocument/didOpen", "textDocument/didChange", "textDocument/didClose", "textDocument/hover", "textDocument/definition", "textDocument/documentSymbol":
	default:
		return fail(-32601, "Method not found")
	}
	var params struct {
		TextDocument struct {
			URI     string `json:"uri"`
			Text    string `json:"text"`
			Version int    `json:"version"`
		} `json:"textDocument"`
		ContentChanges []struct {
			Text  *string         `json:"text"`
			Range json.RawMessage `json:"range"`
		} `json:"contentChanges"`
		Position *Position `json:"position"`
	}
	if json.Unmarshal(m.Params, &params) != nil {
		return fail(-32602, "Invalid params")
	}
	path, err := uriPath(params.TextDocument.URI)
	if err != nil {
		return fail(-32602, err.Error())
	}
	switch m.Method {
	case "textDocument/didOpen":
		if request {
			return fail(-32600, "didOpen must be a notification")
		}
		s.documents[path] = document{params.TextDocument.URI, params.TextDocument.Text, params.TextDocument.Version}
		return s.check(filepath.Dir(path), "")
	case "textDocument/didChange":
		if request {
			return fail(-32600, "didChange must be a notification")
		}
		doc, ok := s.documents[path]
		if !ok || params.TextDocument.Version <= doc.version {
			return nil
		}
		if len(params.ContentChanges) == 0 {
			return nil
		}
		for _, change := range params.ContentChanges {
			if change.Text == nil || len(change.Range) > 0 && string(change.Range) != "null" {
				return nil
			}
		}
		doc.text = *params.ContentChanges[len(params.ContentChanges)-1].Text
		doc.version = params.TextDocument.Version
		s.documents[path] = doc
		return s.check(filepath.Dir(path), "")
	case "textDocument/didClose":
		if request {
			return fail(-32600, "didClose must be a notification")
		}
		delete(s.documents, path)
		if err := s.publish(path, nil); err != nil {
			return err
		}
		dir := filepath.Dir(path)
		for other := range s.documents {
			if filepath.Dir(other) == dir {
				return s.check(dir, path)
			}
		}
		if previous := s.programs[dir]; previous != nil {
			for _, other := range sortedPaths(previous.diagnostics) {
				if other != path {
					if err := s.publish(other, nil); err != nil {
						return err
					}
				}
			}
		}
		delete(s.programs, dir)
		return nil
	}
	if !request {
		return nil
	}
	snap := s.programs[filepath.Dir(path)]
	if snap == nil || snap.index == nil {
		if m.Method == "textDocument/documentSymbol" {
			return s.reply(m.ID, []DocumentSymbol{}, nil)
		}
		return s.reply(m.ID, nil, nil)
	}
	if m.Method == "textDocument/documentSymbol" {
		symbols := make([]DocumentSymbol, 0)
		for _, symbol := range snap.index.Symbols(path) {
			symbols = append(symbols, snap.symbol(symbol))
		}
		return s.reply(m.ID, symbols, nil)
	}
	if params.Position == nil || params.Position.Line < 0 || params.Position.Character < 0 {
		return fail(-32602, "Invalid position")
	}
	pos, ok := sourcePosition(snap.sources[path], *params.Position)
	if !ok {
		return s.reply(m.ID, nil, nil)
	}
	binding := snap.index.At(path, pos)
	if binding == nil {
		return s.reply(m.ID, nil, nil)
	}
	if m.Method == "textDocument/definition" {
		def := binding.Definition
		if def == nil {
			return s.reply(m.ID, nil, nil)
		}
		return s.reply(m.ID, Location{fileURI(def.File), sourceRange(snap.sources[def.File], def.NamePos, def.NameEnd)}, nil)
	}
	text := ""
	if binding.Type != nil {
		text = snap.program.TypeString(binding.Type, path)
	} else if binding.Definition != nil && binding.Definition.Type != nil {
		text = snap.program.TypeString(binding.Definition.Type, path)
	}
	if text != "" && binding.Name != "" {
		text = binding.Name + " : " + text
	} else if text == "" {
		text = binding.Name
	}
	if binding.Definition != nil && binding.Definition.Doc != "" {
		text += "\n\n" + binding.Definition.Doc
	}
	return s.reply(m.ID, struct {
		Contents any   `json:"contents"`
		Range    Range `json:"range"`
	}{map[string]string{"kind": "plaintext", "value": text}, sourceRange(snap.sources[path], binding.Pos, binding.End)}, nil)
}
func uriPath(uri string) (string, error) {
	u, err := url.Parse(uri)
	if err != nil || u.Scheme != "file" || u.Host != "" && u.Host != "localhost" || u.RawQuery != "" || u.Fragment != "" || !filepath.IsAbs(filepath.FromSlash(u.Path)) {
		return "", errors.New("expected an absolute file URI")
	}
	if filepath.Ext(u.Path) != ".gt" {
		return "", errors.New("expected a .gt document")
	}
	return filepath.Clean(filepath.FromSlash(u.Path)), nil
}
func fileURI(path string) string {
	if strings.HasPrefix(path, "std/") {
		return (&url.URL{Scheme: stdScheme, Path: "/" + path}).String()
	}
	return (&url.URL{Scheme: "file", Path: filepath.ToSlash(path)}).String()
}
func (s *server) check(dir, closed string) error {
	previous := s.programs[dir]
	byFile := map[string][]Diagnostic{}
	snap := &snapshot{sources: make(map[string]string), diagnostics: byFile}
	s.programs[dir] = snap
	report := func(path string, err error) {
		pos, end := ast.Pos{Line: 1, Col: 1}, ast.Pos{Line: 1, Col: 2}
		msg := err.Error()
		if e, ok := err.(*types.Error); ok {
			if e.File != "" {
				path = e.File
			}
			pos, end, msg = e.Pos, e.End, e.Msg
		} else if e, ok := err.(*modules.Error); ok {
			if e.File != "" {
				path = e.File
			}
			pos, end, msg = e.Pos, e.End, e.Msg
		} else if e, ok := err.(interface {
			GetPos() ast.Pos
			GetEnd() ast.Pos
		}); ok {
			pos, end = e.GetPos(), e.GetEnd()
			msg = strings.TrimPrefix(msg, pos.String()+": ")
		} else {
			var lexical ast.Pos
			if n, scanErr := fmt.Sscanf(msg, "%d:%d:", &lexical.Line, &lexical.Col); n == 2 && scanErr == nil && lexical.Line > 0 && lexical.Col > 0 {
				pos, end = lexical, ast.Pos{}
				msg = strings.TrimPrefix(msg, pos.String()+": ")
			}
		}
		byFile[path] = append(byFile[path], Diagnostic{sourceRange(snap.sources[path], pos, end), 1, "go4t", msg})
	}
	paths := map[string]bool{}
	entries, err := os.ReadDir(dir)
	if err != nil && !os.IsNotExist(err) {
		for path := range s.documents {
			if filepath.Dir(path) == dir {
				report(path, err)
			}
		}
	}
	for _, entry := range entries {
		if !entry.IsDir() && filepath.Ext(entry.Name()) == ".gt" {
			paths[filepath.Join(dir, entry.Name())] = true
		}
	}
	for path := range s.documents {
		if filepath.Dir(path) == dir {
			paths[path] = true
		}
	}
	ordered := make([]string, 0, len(paths))
	for path := range paths {
		ordered = append(ordered, path)
	}
	sort.Strings(ordered)
	var files []*ast.File
	for _, path := range ordered {
		var source string
		if doc, ok := s.documents[path]; ok {
			source = doc.text
		} else {
			data, err := os.ReadFile(path)
			if err != nil {
				report(path, err)
				continue
			}
			source = string(data)
		}
		snap.sources[path] = source
		file, err := parser.Parse(source)
		if err != nil {
			report(path, err)
			continue
		}
		file.Name = path
		files = append(files, file)
	}
	if len(byFile) == 0 && len(files) > 0 {
		var options modules.Options
		if s.goBinder != nil {
			options.GoBinder = s.goBinder(dir)
		}
		loaded, err := modules.LoadFiles(files, dir, options)
		var program *types.Program
		if err == nil {
			for _, module := range loaded {
				for _, file := range module.Files {
					snap.sources[file.Name] = file.Source
				}
			}
			program, err = types.Check(loaded)
		}
		if err != nil {
			report(ordered[0], err)
		} else {
			snap.program = program
			snap.index = program.PositionIndex()
			for _, warning := range program.Warnings {
				path := warning.File
				byFile[path] = append(byFile[path], Diagnostic{sourceRange(snap.sources[path], warning.Pos, warning.End), 2, "go4t", warning.Msg})
			}
			for _, failure := range laws.CheckProgram(program) {
				report(ordered[0], failure)
			}
		}
	}
	publish := map[string]bool{}
	for path := range s.documents {
		if filepath.Dir(path) == dir {
			publish[path] = true
		}
	}
	for path := range byFile {
		publish[path] = true
	}
	if previous != nil {
		for path := range previous.diagnostics {
			publish[path] = true
		}
	}
	ordered = ordered[:0]
	for path := range publish {
		if path != closed {
			ordered = append(ordered, path)
		}
	}
	sort.Strings(ordered)
	for _, path := range ordered {
		if err := s.publish(path, byFile[path]); err != nil {
			return err
		}
	}
	return nil
}

func sortedPaths(diagnostics map[string][]Diagnostic) []string {
	paths := make([]string, 0, len(diagnostics))
	for path := range diagnostics {
		paths = append(paths, path)
	}
	sort.Strings(paths)
	return paths
}
func (s *server) publish(path string, diagnostics []Diagnostic) error {
	if diagnostics == nil {
		diagnostics = []Diagnostic{}
	}
	params := struct {
		URI         string       `json:"uri"`
		Version     *int         `json:"version,omitempty"`
		Diagnostics []Diagnostic `json:"diagnostics"`
	}{URI: fileURI(path), Diagnostics: diagnostics}
	if doc, ok := s.documents[path]; ok {
		params.URI = doc.uri
		params.Version = &doc.version
	}
	return s.notify("textDocument/publishDiagnostics", params)
}

var symbolKinds = [...]int{
	types.FunctionSymbol: 12, types.TypeSymbol: 23, types.ConstructorSymbol: 9,
	types.FieldSymbol: 8, types.ClassSymbol: 11, types.MethodSymbol: 6,
	types.AliasSymbol: 26, types.VariableSymbol: 13, types.TypeParameterSymbol: 26,
	types.InstanceSymbol: 3, types.LawSymbol: 12, types.ProofSymbol: 12,
}

func (s *snapshot) symbol(symbol *types.Symbol) DocumentSymbol {
	kind := symbolKinds[symbol.Kind]
	src := s.sources[symbol.File]
	result := DocumentSymbol{Name: symbol.Name, Kind: kind, Range: sourceRange(src, symbol.Pos, symbol.End), SelectionRange: sourceRange(src, symbol.NamePos, symbol.NameEnd)}
	if symbol.Type != nil {
		result.Detail = s.program.TypeString(symbol.Type, symbol.File)
	}
	for _, child := range symbol.Children {
		result.Children = append(result.Children, s.symbol(child))
	}
	return result
}
func sourcePosition(source string, pos Position) (ast.Pos, bool) {
	line, actual := sourceLine(source, pos.Line)
	if pos.Line < 0 || actual != pos.Line || pos.Character < 0 {
		return ast.Pos{}, false
	}
	col, units := 1, 0
	for _, r := range line {
		if units == pos.Character {
			return ast.Pos{Line: pos.Line + 1, Col: col}, true
		}
		if r > 0xffff {
			units += 2
		} else {
			units++
		}
		if units > pos.Character {
			return ast.Pos{}, false
		}
		col++
	}
	return ast.Pos{Line: pos.Line + 1, Col: col}, units == pos.Character
}
func lspPosition(source string, pos ast.Pos) Position {
	line := pos.Line - 1
	if line < 0 {
		line = 0
	}
	text, line := sourceLine(source, line)
	units, col := 0, 1
	for _, r := range text {
		if col >= pos.Col {
			break
		}
		if r > 0xffff {
			units += 2
		} else {
			units++
		}
		col++
	}
	return Position{line, units}
}

// sourceLine returns a view, not a split/copy of the entire document. Out-of-file
// line numbers clamp to the final line; callers can reject using the actual index.
func sourceLine(source string, line int) (string, int) {
	actual := 0
	for actual < line {
		i := strings.IndexByte(source, '\n')
		if i < 0 {
			break
		}
		source = source[i+1:]
		actual++
	}
	if i := strings.IndexByte(source, '\n'); i >= 0 {
		source = source[:i]
	}
	return strings.TrimSuffix(source, "\r"), actual
}

func sourceRange(source string, start, end ast.Pos) Range {
	if end == (ast.Pos{}) || end.Line < start.Line || end.Line == start.Line && end.Col <= start.Col {
		end = ast.Pos{Line: start.Line, Col: start.Col + 1}
	}
	return Range{lspPosition(source, start), lspPosition(source, end)}
}
