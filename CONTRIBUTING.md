# Contributing to go4t

go4t is pre-1.0. The grammar, standard library and Go ABI can still change
between releases; `SPEC.md` is the contract for the current behaviour.

## Build and test

Requires a Go toolchain (the minimum version is the `go` line in `go.mod`).

```sh
go build ./cmd/go4t          # the compiler
go vet ./...
go test ./...                # unit, fixture, doc-snippet and property tests
bash selfhost/bootstrap.sh   # the self-hosted compiler reproduces its own output
go test ./bench -run . -bench . -benchtime=1x -timeout=12m   # benchmark budgets
```

CI runs these (`.github/workflows/ci.yml`) on the minimum Go version and on
the current stable release, plus every `examples/*/main.gt`. The wall-clock
budgets in `./bench` run in their own job on an idle runner; under
`go test ./...` they share the machine with every other package and can
exceed their ceilings on a loaded one.

Fuzz targets exist for the lexer, parser and formatter. They run their seed
corpus as ordinary tests, and CI fuzzes each for a short fixed time; to fuzz
longer:

```sh
go test ./lexer  -run '^$' -fuzz '^FuzzLexer$'  -fuzztime 60s
go test ./parser -run '^$' -fuzz '^FuzzParser$' -fuzztime 60s
go test ./format -run '^$' -fuzz '^FuzzFormat$' -fuzztime 60s
```

## Where things live

| Path | Role |
| --- | --- |
| `lexer/`, `parser/`, `ast/`, `format/` | Source → AST → canonical source |
| `types/` | Inference, type classes, deriving, diagnostics |
| `laws/` | Law checker: normalizer, tactics, termination and taint analysis, sampled laws |
| `codegen/`, `rt/` | Go emission and the runtime it links |
| `extern/`, `cmd/go4t/bind*.go` | `import "go:…"` bindings and shims |
| `std/` | Standard library, written in go4t |
| `lsp/`, `editors/` | Language server and editor clients |
| `selfhost/` | go4t compiler written in go4t |
| `examples/` | Runnable programs; CI executes them |
| `docs/adr/` | One record per design decision |

## Rules of the road

- **SPEC first.** A change in behaviour changes `SPEC.md` in the same commit.
  Tutorial and SPEC snippets are type-checked by `go test ./docs`.
- **A design decision gets an ADR** (`docs/adr/NNNN-title.md`, English):
  context, options, decision, measured consequences. SPEC states the rule; the
  ADR says why.
- **The law checker must never accept a false law.** A change under `laws/`
  that adds a reduction, a model or a decision rule extends the trusted base:
  list it in `docs/laws-trust.md`, argue its soundness in the ADR, and cover it
  in the open-term property test (`laws/property_test.go`), which compares
  normal forms with the real runtime. Prefer leaving a term stuck to guessing.
- **Diagnostics are part of the interface.** Message templates are pinned by
  tests and quoted in the docs; change both together.
- **Generated code is not committed** unless an example exists to show it.
  Bindings for `import "go:…"` are produced at build time and cached.
- Format Go with `gofmt` and go4t with `go4t fmt`. Commit subjects follow
  `area: what changed`.

## Reporting bugs

Open an issue with the smallest `.gt` program that shows the problem, the
command, the output, and `go version`. A law the checker accepts but that is
false for some input is a soundness bug: say so in the title. For security
reports see [SECURITY.md](SECURITY.md).
