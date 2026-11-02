# Examples

[日本語](README.ja.md)

Commands run from the checkout root. Network-using examples are marked.

| Example | Shows | Run |
| --- | --- | --- |
| [go-shell](go-shell) | Minimal Go shell + go4t core template. | `go4t run examples/go-shell/core.gt` |
| [approval-controller](approval-controller) | Real controller; 6 original tests unchanged. | `cd examples/approval-controller && go generate ./... && go test ./...` |
| [attachment-repair-controller](attachment-repair-controller) | Multi-step workflow decided in go4t; 22 original tests unchanged. | `cd examples/attachment-repair-controller && go generate ./... && go test ./...` |
| [node-drain-controller](node-drain-controller) | Controller written from scratch with 10 checked laws (cordon-first, DaemonSet transparency, exact timeout). | `go4t check examples/node-drain-controller/decide/decide.gt && cd examples/node-drain-controller && go generate ./... && go test ./...` |
| [sql-service](sql-service) | SQLite + Prometheus HTTP service via `bind`. | see its README |
| [typed-extern](typed-extern) | Generated bindings, ABI adapters, opaque method handles. | `go4t run examples/typed-extern` |
| [deps](deps) | Offline Go-module dependency with local replacement. | `go4t run examples/deps/app` |
| [calc](calc) | Expression interpreter with `Result` errors. | `go4t run examples/calc/main.gt` |
| [cards](cards) | Poker classification, derived ordering, seeded `State`. | `go4t run examples/cards/main.gt` |
| [scheduler](scheduler) | Custom ordering, guards, bounded plan. | `go4t run examples/scheduler/main.gt` |
| [routes](routes) | Shortest paths and record updates. | `go4t run examples/routes/main.gt` |
| [parser](parser) | User-defined parser monad. | `go4t run examples/parser/main.gt` |
| [monoid](monoid) | User `Monoid` instances with generic folds. | `go4t run examples/monoid/main.gt` |
| [multi-constraint](multi-constraint) | Multiple classes per parameter, superclasses. | `go4t run examples/multi-constraint/main.gt` |
| [deriving](deriving) | Derived `Functor`/`Foldable`/`Traversable`/`Debug`. | `go4t run examples/deriving/main.gt` |
| [json-deriving](json-deriving) | JSON schema derivation and nested error paths. | `go4t run examples/json-deriving/main.gt` |
| [http](http) | `std/http` server/client, JSON routes, cancellation (loopback). | `go4t run examples/http/main.gt -- 8080` |
| [sales-report](sales-report) | Parallel fetches, typed IO recovery, fallbacks. | `go4t run examples/sales-report` |
| [concurrency](concurrency) | Fork/await, ordered parallel work, race, timeout. | `go4t run examples/concurrency/main.gt` |
| [state](state) | Pure `State` transitions. | `go4t run examples/state/main.gt` |
| [collections](collections) | Persistent Map/Set/Array. | `go4t run examples/collections/main.gt` |
| [lists](lists) | List Functor/Monad, cartesian `do`. | `go4t run examples/lists/main.gt` |
| [tree](tree) | Recursive Functor and constrained rendering. | `go4t run examples/tree/main.gt` |
| [records](records) | Records, interpolation, let polymorphism. | `go4t run examples/records/main.gt` |
| [alias-fields](alias-fields) | Aliases and common-field updates. | `go4t run examples/alias-fields/main.gt` |
| [eqord](eqord) | Custom equality/ordering. | `go4t run examples/eqord/main.gt` |
| [float-num](float-num) | Float parsing/formatting, IEEE edges. | `go4t run examples/float-num/main.gt` |
| [strings](strings) | Unicode strings, parsing, padding. | `go4t run examples/strings/main.gt` |
| [stdlib-extras](stdlib-extras) | Regex, stable grouping, tables, `Lazy`. | `go4t run examples/stdlib-extras/main.gt` |
| [pipe-partial](pipe-partial) | Pipelines, `_` placeholders, sections. | `go4t run examples/pipe-partial/main.gt` |
| [local-recursion](local-recursion) | Recursive lambdas, mutual local recursion. | `go4t run examples/local-recursion/main.gt` |
| [io-effects](io-effects) | File read/validate/write/append. | `out=$(mktemp -d); go4t run examples/io-effects/main.gt -- examples/io-effects/config.txt "$out/report"` |
| [wc](wc) | Stdin word frequencies. | `go4t run examples/wc/main.gt -- 3 < examples/wc/input.txt` |
| [cli-tools](cli-tools) | Pure/IO tests and eval; also `go4t test examples/cli-tools/main.gt`. | `go4t run examples/cli-tools/main.gt` |
| [modules](modules) | Directory loading, aliases, shadow warning. | `go4t run examples/modules` |
| [lsp](lsp) | Cross-module hover/definition playground. | `go4t run examples/lsp/main.gt` |
| [diagnostics](diagnostics) | Exhaustiveness and alias-mismatch diagnostics (`bad_*.gt` fail on purpose). | `go4t run examples/diagnostics/main.gt` |
| [fmt](fmt) | Formatter input; `go4t fmt -d examples/fmt/before.gt`. | `go4t run examples/fmt/before.gt` |
| [codegen-quality](codegen-quality) | Million-step tail recursion, source locations. | `go4t run examples/codegen-quality/main.gt` |
| [perf](perf) | Native scalars, bounded specialization, tail loops. | `go4t run examples/perf/main.gt` |
| [bank](bank), [inventory](inventory), [orders](orders) | Domain validation with HTTP notification (**network**). | `go4t run examples/<name>/main.gt` |

Negative fixtures: `go4t test examples/cli-tools/failing.gt` exits 1,
`go4t check examples/diagnostics/bad_hint.gt` exits 1,
`go4t check examples/diagnostics/bad_unused.gt` warns and exits 0.
