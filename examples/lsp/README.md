# Language server example

Open `main.gt`, hover `helpers.keep` or `helpers.greeting`, and go to their
definitions in `helpers/helpers.gt`. Both the server and CLI load the entry
module and its transitive imports through the shared module loader.

```sh
go run ./cmd/go4t run examples/lsp/main.gt
```

Observed output:

```text
Hello, LSP!
```

## Scripted session against the real word-count example

From the repository root:

```sh
go test ./lsp -run '^TestScriptedSession$' -v -count=1
```

This test starts `lsp.Serve` with two in-memory pipes and exchanges real
Content-Length-framed JSON-RPC messages. It reads `examples/wc/main.gt` from disk,
then sends:

1. `initialize` (request ID 1).
2. `textDocument/didOpen` with that file's complete contents, version 1.
3. `textDocument/didChange`, version 2, replacing `fold(tokens` with
   `missingFold(tokens` in the **in-memory document only**.
4. `textDocument/didChange`, version 3, restoring the original contents.
5. `textDocument/hover` at `fold`, request ID 2.
6. `textDocument/definition` at the call to `increment`, request ID 2 reused
   after the previous request completes. Positions are computed from current source.

The JSON transcript below is the original pre-module formatting receipt;
the test computes current positions after imports and canonical formatting.

Observed responses from the command above are reproduced verbatim below (the
absolute URI reflects the worktree where the test ran). Transport headers are
removed by the test's framing reader, not part of these JSON payloads.

### Initialize

```json
{"jsonrpc":"2.0","id":1,"result":{"capabilities":{"definitionProvider":true,"documentSymbolProvider":true,"hoverProvider":true,"positionEncoding":"utf-16","textDocumentSync":1},"serverInfo":{"name":"go4t"}}}
```

### Open: no diagnostics

```json
{"jsonrpc":"2.0","method":"textDocument/publishDiagnostics","params":{"uri":"file:///workspace/wip/go4t/.wt/lsp/examples/wc/main.gt","version":1,"diagnostics":[]}}
```

### Injected error, including its exclusive source span

```json
{"jsonrpc":"2.0","method":"textDocument/publishDiagnostics","params":{"uri":"file:///workspace/wip/go4t/.wt/lsp/examples/wc/main.gt","version":2,"diagnostics":[{"range":{"start":{"line":40,"character":17},"end":{"line":40,"character":28}},"severity":1,"source":"go4t","message":"unknown identifier missingFold"}]}}
```

### Restore: clear the diagnostic

```json
{"jsonrpc":"2.0","method":"textDocument/publishDiagnostics","params":{"uri":"file:///workspace/wip/go4t/.wt/lsp/examples/wc/main.gt","version":3,"diagnostics":[]}}
```

### Hover on `fold`

```json
{"jsonrpc":"2.0","id":2,"result":{"contents":{"kind":"plaintext","value":"fold : func(List[String], List[Tuple2[String, Int]], func(List[Tuple2[String, Int]], String) List[Tuple2[String, Int]]) List[Tuple2[String, Int]]"},"range":{"start":{"line":40,"character":17},"end":{"line":40,"character":21}}}}
```

`Tuple2` is the existing checker printer's representation, not a new LSP-specific
type printer. At a polymorphic declaration the same printer retains its rigid
type parameters; a use site displays its instantiated type. `fold` has no directly
adjacent declaration doc comment in the current prelude, so this hover contains
only its type. The `keep` and `greeting` playground declarations demonstrate docs.

### Definition of the user's `increment` function

```json
{"jsonrpc":"2.0","id":2,"result":{"uri":"file:///workspace/wip/go4t/.wt/lsp/examples/wc/main.gt","range":{"start":{"line":2,"character":5},"end":{"line":2,"character":14}}}}
```

Observed test result: `PASS`, `ok go4t/lsp`. No edits were made to the word-count
example. `go run ./cmd/go4t check examples/wc/main.gt` also exited 0 without
diagnostics (run once).

The protocol tests additionally cover open/change/close diagnostics, warning
severity, UTF-16 spans, full-text overlays and stale versions, monomorphic and
polymorphic hover text, declaration docs, cross-file definitions, document-symbol
kinds, error responses, malformed framing, and shutdown. The command test
`go test ./cmd/go4t -run '^TestLSPStdioCommand$'` starts the real `run(["lsp"])`
dispatch in a subprocess and checks its framed stdout.
