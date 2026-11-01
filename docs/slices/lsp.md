# lsp — Wave B specification delta

## CLI and grammar

Add `gofx lsp`: run the LSP 3.17 server on stdin/stdout until `exit` or EOF.
Additional command-line arguments are usage errors. No source grammar, prelude
signature, checker acceptance rule, runtime representation or generated Go ABI
changes. `SPEC.md` and `DESIGN.md` are intentionally unchanged.

The parser now retains `ast.File.Source` and `ast.File.Comments`. `ast.Comment`
contains `Pos, End ast.Pos`, `Text string` (including `//`, excluding the newline),
and `Trailing bool`. This is the same representation agreed with **fmt**, not a
parallel declaration-comment convention. Declaration documentation is the
contiguous sequence of non-trailing `//` lines ending immediately above the
declaration. Blank lines break attachment; comment markers and surrounding line
whitespace are stripped, and lines are joined with newlines. Strings containing
`//` are not comments. Extern docs attach above `extern`, even if `func` starts on
a following line. Local named functions also retain declaration docs in the index.

## Program and lifecycle semantics

- Each opened file's directory supplies all immediate `.gfx` files, ordered by
  filename, with the embedded prelude implicitly first. Non-prelude package clauses
  must agree. The ordinary `types.Check` rules apply, including `main`'s signature.
- `didOpen` supplies an unsaved overlay, including files not yet present on disk.
  `didChange` replaces the whole buffer. Versions must strictly increase; stale
  versions, incremental ranges and changes to unopened documents are ignored.
- Every accepted open/change rebuilds that directory's checked snapshot. Other open
  overlays in the same directory participate in checking. Disk is never modified.
- Closing a document clears its published diagnostics and removes its overlay. If
  other documents in the directory remain open, the directory is checked again
  using the closed document's disk contents. Otherwise its snapshot is released.
- Diagnostics are published for open documents, files with diagnostics, and files
  whose prior diagnostics need clearing. Open-document versions are included.
- Syntax failures may produce one error per parsed file. The checker is fail-fast:
  a failed check publishes its first error, while a successful check publishes its
  warnings. A failed check discards the previous index rather than using stale or
  partially checked annotations. Hover/definition then return null and document
  symbols return an empty array until a successful recheck.

## Protocol surface

`lsp.Serve(in io.Reader, out io.Writer) error` owns no streams and processes messages
in order. Messages use JSON-RPC 2.0 with byte-counted `Content-Length` and CRLF
headers. Request IDs are echoed without coercion. Notifications have no response.
The server writes no logging to its protocol stream. Body size is bounded at
16 MiB and headers at 8192 bytes; malformed/truncated framing returns an error.

`initialize` advertises:

```json
{
  "definitionProvider": true,
  "documentSymbolProvider": true,
  "hoverProvider": true,
  "positionEncoding": "utf-16",
  "textDocumentSync": 1
}
```

Supported methods:

- `initialize`, `initialized`, `shutdown`, `exit`.
- `textDocument/didOpen`, `textDocument/didChange`, `textDocument/didClose`.
- `textDocument/hover`, `textDocument/definition`, `textDocument/documentSymbol`.
- Outbound `textDocument/publishDiagnostics`.
- `gofx/prelude`: read-only extension returning the embedded source string for the
  `gofx-prelude:///prelude.gfx` virtual document. The VS Code client registers its
  content provider. Builtin types without declarations have no definition target.

`$/cancelRequest` and `$/setTrace` notifications are accepted without a response;
processing is synchronous, so completed requests need no cancellation machinery.
Unknown requests fail with `-32601`, `Method not found`; unknown notifications are
ignored. Other protocol errors include:

| Code | Message / condition |
| --- | --- |
| -32700 | `Parse error` — malformed JSON body |
| -32600 | `Invalid Request` — malformed JSON-RPC envelope or ID |
| -32002 | `Server not initialized` — request before initialize |
| -32600 | `Server already initialized` — duplicate initialize |
| -32600 | `Server has shut down` — request after shutdown |
| -32602 | `Invalid params`, `Invalid position`, `expected an absolute file URI`, or `expected a .gfx document` |

`shutdown` returns null. `exit` after shutdown succeeds; `exit` before shutdown
returns the Go error `LSP exit before shutdown`, giving the CLI a nonzero status.
EOF also ends serving without writing spurious protocol output.

## Diagnostics, hover and navigation

Checker `Error.Pos/End` and warning spans are converted from one-based rune
columns to zero-based UTF-16 positions, with exclusive ends. Parser errors retain
their exposed spans; legacy lexer error positions are extracted from their
`line:column:` prefix. CRLF and surrogate pairs are handled. Severity is 1 for
errors and 2 for warnings; source is `gofx`. Checker message strings are preserved,
including `unknown identifier missingFold` and
`unused is unused; prefix with _ to silence`.

Hover returns plaintext `name : <types.TypeString(type)>`, followed by adjacent
declaration docs when present. Use sites display their instantiated annotation
type; declarations display their polymorphic signature. The existing printer's
`Tuple2[...]` spelling is preserved. There is no alternate LSP type printer.

Definitions target identifier-only declaration ranges for functions, externs,
locals/parameters/pattern bindings, constructors, data types, aliases, type
parameters, fields and class methods. Class method calls navigate to their class
signature, not an arbitrarily chosen instance. Common fields navigate to the
first constructor's declaring field, matching the checker's selected `Ann.Ctor`.
The index respects lexical shadowing, recursive lets and mutually recursive local
function groups. It includes separately lexed interpolation expression tokens.
Cross-file locations are ordinary absolute file URIs.

Document symbols are hierarchical, in declaration order. LSP kinds: function 12,
data type 23 (Struct), constructor 9, field 8, class 11 (Interface), method 6,
alias 26 (TypeParameter), instance container 3 (Namespace). Generated deriving
nodes without user source spans are excluded. Constructor fields and class or
instance methods are children of their enclosing declaration.

## Additive types API

`types/index.go` adds:

```go
func (p *Program) PositionIndex() *PositionIndex
func (x *PositionIndex) At(file string, pos ast.Pos) *Binding
func (x *PositionIndex) Symbols(file string) []*Symbol
```

`Binding` contains `Name`, occurrence `Pos/End`, instantiated `Type`, and
`Definition *Symbol`. `Symbol` contains `Name`, `File`, `Doc`, `Kind`, `Type`, full
`Pos/End`, identifier-only `NamePos/NameEnd`, and `Children []*Symbol`.

The API accepts **successfully checked programs only**. Index construction is
explicit and does not modify existing annotations/checking behavior; ordinary
compilation pays no index-building cost. Lookups are file-specific and use an
exclusive end. A fresh index must be built after rechecking or changing source.
Source-free synthesized declarations cannot supply a definition location.

## Client and examples

`editors/vscode/package.json` and `extension.js` provide a minimal client using
only VS Code and Node builtin APIs, with no npm dependencies/build step. It
spawns `gofx lsp`, synchronizes full buffers, registers the three providers,
publishes diagnostics, supports virtual prelude source, and bounds shutdown.
`gofx.executablePath` configures the executable. Installation/development-host
commands are in `editors/README.md`.

`examples/lsp/main.gfx` and `helpers.gfx` demonstrate cross-file hover/docs and
definitions. `examples/lsp/README.md` records actual output plus the actual JSON
responses for initialize → opening `examples/wc/main.gfx` → injected error →
restore → hover `fold` → definition of user function `increment`.

## Verification and cleanup

All Go checks below ran in this worktree through:

```text
env SERIALIZE_VM_TESTS_MODE=shared /home/vagrant/.codex/skills/serialize-vm-tests/scripts/serialize-vm-tests --
```

Observed commands/results:

- `go test ./lsp` — passed the initial diagnostics, hover, cross-file definition
  and symbol-kind contracts.
- `go test ./lexer ./parser ./types ./lsp` — initially failed only the newly added
  UTF-16 test's hand-counted expected column (expected 45, actual 46); correcting
  the expectation and surrogate-half position fixed the test, not production
  range conversion. The final invocation passed all four packages.
- `go test ./lsp ./cmd/gofx -run 'TestDiagnostics|TestHover|TestProtocol|TestUTF16|TestFraming|TestLSP' -count=1`
  — passed both packages, including a subprocess exercising real CLI dispatch.
- `go test ./lsp -run '^TestScriptedSession$' -v -count=1` — passed and emitted
  the JSON reproduced in the example README.
- `go run ./cmd/gofx check examples/wc/main.gfx` — exit 0, no diagnostics; run once.
- `go run ./cmd/gofx run examples/lsp/main.gfx examples/lsp/helpers.gfx` — exit 0,
  stdout `Hello, LSP!`.
- `go test ./types ./lsp` — passed after the final interpolation/local-doc index
  refinement, including the scripted session as part of the server package.
- `gofmt -w` was applied only to Go files changed/added by this slice.

Tests cover open/change/close, warning severity, parse and lexer errors, stale
index rejection, disk/overlay restoration, stale versions, UTF-16 boundaries,
polymorphic/monomorphic types, declaration doc adjacency, lexical shadowing,
recursive locals, interpolation definitions, cross-file functions/constructors/
fields/methods, type/alias lookup, document-symbol kinds, JSON-RPC errors and framing bounds.

The generated `examples/lsp/main_gen/` directory was removed. Test temporaries
use `t.TempDir`; pipe and subprocess resources are closed. No commits, pushes,
full repository suite, separate vet, live access, or interactive VS Code acceptance
were performed. The extension's UI remains unexercised.

## Parallel integration and deviations

- **fmt:** keep its newer `scanComment`/interpolation-aware lexer implementation
  when merging the identical comment-capture seam. Add this slice's `File.Source`
  and `Parse` assignment alongside fmt's `Comments`, `PackagePos/PackageEnd` and
  formatting metadata. No duplicate `Comment` type should remain.
- **modules:** this independent slice follows the assigned directory/file checker
  API (`types.Check([]*ast.File)`). Modules changes that API to module inputs and
  alpha-renames symbols. Its integration must adapt the server's checker call and
  preserve source spelling/positions for index token matching. This was coordinated
  with Modules; no guessed future API or compatibility shim was introduced.
- The full comment collection is shared with fmt instead of adding a second,
  declaration-only lexer. The `gofx/prelude` request is the sole protocol extension,
  needed to open embedded definition targets. No deviation from the requested
  directory-program LSP behavior; no unresolved blocker in this standalone slice.
