# go4t editor integration

## Server

Build/install the existing CLI using Go 1.22 or later:

```sh
go install ./cmd/go4t
```

Configure any LSP-capable editor to launch `go4t lsp` with stdin/stdout connected to
its language client. This is a stdio server, not a TCP listener. Stdout contains
only Content-Length-framed JSON-RPC 2.0 messages; startup/transport errors go to the
CLI's stderr. No third-party Go packages are required.

Supported LSP 3.17 methods:

- `initialize`, `initialized`, `shutdown`, `exit`.
- `textDocument/didOpen`, `didChange`, `didClose`, with **full document sync**.
- `textDocument/publishDiagnostics`: errors and warnings, with source ranges.
- `textDocument/hover`: the checker's type text plus adjacent declaration docs.
- `textDocument/definition`: identifiers, constructors, types, fields and methods.
- `textDocument/documentSymbol`: hierarchical declaration symbols.

Positions use zero-based UTF-16 columns, including non-BMP characters. Full-change
versions must increase. An opened buffer overrides disk; closing it clears its
diagnostics and restores disk content for other open files in that directory.
The entry directory's `.gt` files form one module. The shared module loader
resolves its transitive imports using `go4t.mod`, `GO4T_PATH`, and embedded `std/`,
with only the fixed prelude exports implicit. Keep unrelated entry programs in
separate directories.
The usual checker rules, including the required `main`, apply.

Errors invalidate the previous index: hover/definition return null until the
program checks successfully again. The checker reports its first type error, or
all available warnings after success. Parse failures can report one error per
file. There is no stale successful snapshot masquerading as the current buffer.
This server does not advertise completion, incremental edits, rename, file
watching, workspace symbols or formatting.

Builtin types have no source definition. Embedded standard definitions use
`go4t-std:///std/<module>/<file>.gt`. Clients can request `go4t/stdlib` with
`{"uri":"go4t-std:///std/list/list.gt"}` to read the corresponding embedded source.
The supplied VS Code client handles this automatically.

## Minimal VS Code client

`vscode/package.json` and `vscode/extension.js` form a dependency-free extension:
only VS Code's extension API and Node's built-in child-process/Buffer APIs are
used. No npm install or JavaScript build is necessary.

Build the CLI and launch the dependency-free extension from the repository root:

```sh
go build -o /tmp/go4t ./cmd/go4t
code --extensionDevelopmentPath="$PWD/editors/vscode" examples/sales-report/main.gt
```

Alternatively, install it without a VSIX or npm tooling by copying `vscode/` to
an isolated extension directory:

```sh
extensions=$(mktemp -d)
mkdir -p "$extensions/local.go4t-0.0.1"
cp -R editors/vscode/. "$extensions/local.go4t-0.0.1/"
code --extensions-dir "$extensions" examples/sales-report/main.gt
```

Remove that temporary directory after closing the editor. Set
`go4t.executablePath` to an absolute path before opening a `.gt` file:

```json
{
  "go4t.executablePath": "/tmp/go4t"
}
```

The extension activates for `.gt` files, spawns `go4t lsp`, forwards full-text
open/change/close notifications, and registers hover, definition and document
symbol providers. Problems appear in VS Code's Problems panel. The **go4t** output
channel contains child stderr and lifecycle errors. Reload the window after
changing the executable path or after a server failure. Hover docs are treated as
plain text, not trusted Markdown/commands. Deactivation sends shutdown/exit and
bounds child termination.

The real `go4t lsp` stdio session against both sales-report files is verified:
full `summarize` signature, `rates` hover, `parseCsv` navigation into `domain.gt`,
document symbols, unused-let warning and type-error ranges, and clean restore.
See [`docs/lsp-verification.md`](../docs/lsp-verification.md) for the exact trimmed
requests/responses and `lsp/sales_report_test.go` for the regression replay.

VS Code attachment was **not** verified in this environment: both
`code --version` and `codium --version` failed with `command not found`.
Thus no temporary extension installation, headless `--disable-gpu` launch, or
language-client attachment log could be captured. The setup above follows the
existing unpacked extension layout; it is not evidence of an editor-host run.
On a machine with the launcher, add `--disable-gpu --verbose` to the isolated
launch and inspect the **go4t** output channel and `~/.config/Code/logs`
(`~/.config/VSCodium/logs` for VSCodium). A display server may still be required;
`--disable-gpu` alone does not supply one.

Current protocol, module, and boundary contracts are in
[`SPEC.md` §8.5 and §10.8](../SPEC.md#85-editor-protocol).
