# gofx editor integration

## Server

Build/install the existing CLI using Go 1.22 or later:

```sh
go install ./cmd/gofx
```

Configure any LSP-capable editor to launch `gofx lsp` with stdin/stdout connected to
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
The entry directory's `.gfx` files form one module. The shared module loader
resolves its transitive imports using `gofx.mod`, `GOFX_PATH`, and embedded `std/`,
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
`gofx-std:///std/<module>/<file>.gfx`. Clients can request `gofx/stdlib` with
`{"uri":"gofx-std:///std/list/list.gfx"}` to read the corresponding embedded source.
The supplied VS Code client handles this automatically.

## Minimal VS Code client

`vscode/package.json` and `vscode/extension.js` form a dependency-free extension:
only VS Code's extension API and Node's built-in child-process/Buffer APIs are
used. No npm install or JavaScript build is necessary.

For development, from the repository root with the `code` launcher installed:

```sh
code --extensionDevelopmentPath="$PWD/editors/vscode" examples/lsp/main.gfx
```

Alternatively, copy this directory's `vscode` subdirectory to a local VS Code
extension directory, then reload the editor. Set `gofx.executablePath` to an
absolute executable path if `gofx` is not on the extension host's PATH:

```json
{
  "gofx.executablePath": "/absolute/path/to/gofx"
}
```

The extension activates for `.gfx` files, spawns `gofx lsp`, forwards full-text
open/change/close notifications, and registers hover, definition and document
symbol providers. Problems appear in VS Code's Problems panel. The **gofx** output
channel contains child stderr and lifecycle errors. Reload the window after
changing the executable path or after a server failure. Hover docs are treated as
plain text, not trusted Markdown/commands. Deactivation sends shutdown/exit and
bounds child termination.

The protocol, command wiring and example are tested by Go tests; no interactive
VS Code session was exercised in this slice. See
[`examples/lsp/README.md`](../examples/lsp/README.md) for the observed scripted
session and exact JSON responses. Current protocol, module, and boundary contracts
are in [`SPEC.md` §8.5 and §10.8](../SPEC.md#85-editor-protocol).
