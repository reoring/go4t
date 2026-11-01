# Sales-report LSP verification

## Observed stdio session

On 2026-11-01, from the repository root:

```sh
go build -o /tmp/go4t-lsp-verify ./cmd/go4t
/tmp/go4t-lsp-verify lsp
```

A Python subprocess client sent UTF-8 JSON with `Content-Length: <byte count>\r\n\r\n` framing and read each framed response. The server exited 0 after shutdown/exit; stderr was empty. No LSP defect was encountered.

The trimmed exchange below preserves response text and zero-based UTF-16 ranges. `$SALES` replaces the absolute `file:///workspace/wip/go4t/.wt/lsp-verify/examples/sales-report` URI prefix. Text payload placeholders are explicitly defined below; they are not literal wire values. Each arrow is one complete framed JSON body. `documentSymbol` is trimmed to `summarize`; its full top-level names were `fetchRate`, `fetchRates`, `Row`, `amountJpy`, `summarize`, `table`, `sampleCsv`, `loadCsv`, `main`.

- `<main.gt>` / `<domain.gt>`: exact on-disk files in `examples/sales-report/`.
- `<unused>`: original main source with `  let unused = 1\n` inserted immediately before the `do` in `main` (line 101, one-based).
- `<type-error>`: original main source with the first `parseCsv(text)` replaced by `parseCsv(123)`. This replaces, rather than combines with, the unused-let buffer: the checker publishes warnings only after successful checking.

```text
→ {"jsonrpc":"2.0","method":"initialize","params":{},"id":1}
← {"jsonrpc":"2.0","id":1,"result":{"capabilities":{"definitionProvider":true,"documentSymbolProvider":true,"hoverProvider":true,"positionEncoding":"utf-16","textDocumentSync":1},"serverInfo":{"name":"go4t"}}}
→ {"jsonrpc":"2.0","method":"initialized","params":{}}
→ {"jsonrpc":"2.0","method":"textDocument/didOpen","params":{"textDocument":{"uri":"$SALES/main.gt","languageId":"go4t","version":1,"text":"<main.gt>"}}}
← {"jsonrpc":"2.0","method":"textDocument/publishDiagnostics","params":{"uri":"$SALES/main.gt","version":1,"diagnostics":[]}}
→ {"jsonrpc":"2.0","method":"textDocument/didOpen","params":{"textDocument":{"uri":"$SALES/domain.gt","languageId":"go4t","version":1,"text":"<domain.gt>"}}}
← {"jsonrpc":"2.0","method":"textDocument/publishDiagnostics","params":{"uri":"$SALES/domain.gt","version":1,"diagnostics":[]}}
← {"jsonrpc":"2.0","method":"textDocument/publishDiagnostics","params":{"uri":"$SALES/main.gt","version":1,"diagnostics":[]}}
→ {"jsonrpc":"2.0","method":"textDocument/hover","params":{"textDocument":{"uri":"$SALES/main.gt"},"position":{"line":110,"character":32}},"id":2}
← {"jsonrpc":"2.0","id":2,"result":{"contents":{"kind":"plaintext","value":"summarize : func(Map[Currency, Float], List[Sale], func(Sale) String) List[Row]"},"range":{"start":{"line":110,"character":32},"end":{"line":110,"character":41}}}}
→ {"jsonrpc":"2.0","method":"textDocument/hover","params":{"textDocument":{"uri":"$SALES/main.gt"},"position":{"line":110,"character":42}},"id":3}
← {"jsonrpc":"2.0","id":3,"result":{"contents":{"kind":"plaintext","value":"rates : Map[Currency, Float]"},"range":{"start":{"line":110,"character":42},"end":{"line":110,"character":47}}}}
→ {"jsonrpc":"2.0","method":"textDocument/definition","params":{"textDocument":{"uri":"$SALES/main.gt"},"position":{"line":104,"character":13}},"id":4}
← {"jsonrpc":"2.0","id":4,"result":{"uri":"$SALES/domain.gt","range":{"start":{"line":69,"character":5},"end":{"line":69,"character":13}}}}
→ {"jsonrpc":"2.0","method":"textDocument/documentSymbol","params":{"textDocument":{"uri":"$SALES/main.gt"}},"id":5}
← {"jsonrpc":"2.0","id":5,"result":[{"name":"summarize","detail":"func(Map[Currency, Float], List[Sale], func(Sale) String) List[Row]","kind":12,"range":{"start":{"line":56,"character":0},"end":{"line":65,"character":1}},"selectionRange":{"start":{"line":56,"character":5},"end":{"line":56,"character":14}}}]}
→ {"jsonrpc":"2.0","method":"textDocument/didChange","params":{"textDocument":{"uri":"$SALES/main.gt","version":2},"contentChanges":[{"text":"<unused>"}]}}
← {"jsonrpc":"2.0","method":"textDocument/publishDiagnostics","params":{"uri":"$SALES/domain.gt","version":1,"diagnostics":[]}}
← {"jsonrpc":"2.0","method":"textDocument/publishDiagnostics","params":{"uri":"$SALES/main.gt","version":2,"diagnostics":[{"range":{"start":{"line":100,"character":6},"end":{"line":100,"character":12}},"severity":2,"source":"go4t","message":"unused is unused; prefix with _ to silence"}]}}
→ {"jsonrpc":"2.0","method":"textDocument/didChange","params":{"textDocument":{"uri":"$SALES/main.gt","version":3},"contentChanges":[{"text":"<type-error>"}]}}
← {"jsonrpc":"2.0","method":"textDocument/publishDiagnostics","params":{"uri":"$SALES/domain.gt","version":1,"diagnostics":[]}}
← {"jsonrpc":"2.0","method":"textDocument/publishDiagnostics","params":{"uri":"$SALES/main.gt","version":3,"diagnostics":[{"range":{"start":{"line":104,"character":22},"end":{"line":104,"character":25}},"severity":1,"source":"go4t","message":"type mismatch: expected String, got Int"}]}}
→ {"jsonrpc":"2.0","method":"textDocument/didChange","params":{"textDocument":{"uri":"$SALES/main.gt","version":4},"contentChanges":[{"text":"<main.gt>"}]}}
← {"jsonrpc":"2.0","method":"textDocument/publishDiagnostics","params":{"uri":"$SALES/domain.gt","version":1,"diagnostics":[]}}
← {"jsonrpc":"2.0","method":"textDocument/publishDiagnostics","params":{"uri":"$SALES/main.gt","version":4,"diagnostics":[]}}
→ {"jsonrpc":"2.0","method":"shutdown","params":{},"id":6}
← {"jsonrpc":"2.0","id":6,"result":null}
→ {"jsonrpc":"2.0","method":"exit"}
```

## Regression replay

`lsp/sales_report_test.go` replays the exchange through the existing Content-Length-framed test client and `Serve` (the same entry point used by `go4t lsp`). It reads the real two-file example, asserts exact hover signatures and ranges, cross-file definition, declaration order and the summarize symbol range, both diagnostic severities/messages/ranges and document versions, clearing after restore, and absence of stale hover/definition after a type error. It changes overlays only; source files remain untouched.

```sh
/home/vagrant/.codex/skills/serialize-vm-tests/scripts/serialize-vm-tests -- go test ./lsp
```

Observed result: `ok go4t/lsp 0.247s` (exit 0).
The required CLI acceptance checks also exited 0:

```sh
go run ./cmd/go4t run examples/sales-report
go run ./cmd/go4t check examples/wc/main.gt
```

The sales run loaded six sales, rendered product/region/day tables and JSON,
and reported `broken: line 10: qty is not a number: "many"`.
The word-count check produced no output. Generated `sales.csv` and `report.json`
and the temporary CLI binary were removed after the session.

## VS Code / VSCodium availability

The exact launcher probe was:

```sh
bash -c 'code --version; codium --version; command -v code; command -v codium'
```

Observed stderr:

```text
bash: line 1: code: command not found
bash: line 1: codium: command not found
```

The probe exited 1 and neither `command -v` returned a path. Neither launcher is installed on PATH in this environment. Therefore extension installation into a temporary `--extensions-dir`, a headless `--disable-gpu` extension-host launch, and capture of `~/.config/Code/logs` showing client attachment could not be exercised. No editor attachment is claimed. The verified transport here is the real CLI subprocess, not a VS Code client. See [editor setup](../editors/README.md) for installing and launching the dependency-free extension on a machine with VS Code.

An additional filename search found no VS Code/VSCodium installation under
`/usr/bin/*code*`, `/usr/local/bin/*code*`, `/opt/*code*`, `/usr/share/code*/`,
`/snap/bin/*code*`, or `/home/vagrant/.local/bin/*code*`; only unrelated utilities
matched. No editor binary was available for the conditional host test.
