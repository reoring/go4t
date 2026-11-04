# Examples

[日本語](README.ja.md)

Commands run from the checkout root. Network-using examples are marked.

| Example | Shows | Run |
| --- | --- | --- |
| [go-shell](go-shell) | Minimal Go shell + go4t decision-only template. | see its README (generate and run in a temporary Go module) |
| [approval-controller](approval-controller) | Real AppThrust controller: observe/decide/apply in go4t, Go-owned client and loop; Go reconciler 271 → 88 lines, six original tests unchanged. | `cd examples/approval-controller && go generate ./... && go test ./... -count=1` |
| [attachment-repair-controller](attachment-repair-controller) | Multi-step workflow decided in go4t; 22 original tests unchanged. | `cd examples/attachment-repair-controller && go generate ./... && go test ./...` |
| [node-drain-controller](node-drain-controller) | Controller written from scratch with 10 checked laws (cordon-first, DaemonSet transparency, exact timeout). | `go4t check examples/node-drain-controller/decide/decide.gt && cd examples/node-drain-controller && go generate ./... && go test ./...` |
| [node-drain-shell](node-drain-shell) | Go-owned native clientset and loop call go4t observe/decide/apply IO functions; 15 decision/projection laws, kind-verified drain and release. | see its README, “Generate and run” (needs a disposable cluster) |
| [node-drain-typed](node-drain-typed) | Whole controller in go4t through `go:` typed-clientset imports; 15 decision/projection laws; three-hour soak completed 839/839 cycles. | see its README (needs a disposable cluster); [soak commands](node-drain-typed/soak/README.md) |
| [sql-service](sql-service) | SQLite + Prometheus HTTP service via `go:` imports. | see its README |
| [go-import](go-import) | `import "go:net/http"` / `go:strings` bound at build time: tuples, handle fields, upcasts, package vars (loopback). | `go4t run examples/go-import` |
| [go-chan](go-chan) | Receive from Go `time.After`; communicate between producer and consumer fibers through a buffered channel. | `go4t run examples/go-chan` |
| [go-generics](go-generics) | Generic Go package functions instantiated from concrete call-site types, including callbacks and returned closures in IO. | `go4t run examples/go-generics` |
| [todo](todo) | CLI + JSON file persistence + `std/http` routes, derived codecs, and 3 checked laws (server uses network). | `go4t run examples/todo -- list` |
| [typed-extern](typed-extern) | Executes via `go:net/url`; regenerated bindings and ABI shims are inspection output, with handle-containing records. | `go4t run examples/typed-extern` |
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
| [ref](ref) | IO-only mutable references: HTTP request counter, in-memory key/value store, and 50 atomic concurrent increments (loopback traffic). | `go4t run examples/ref` |
| [state](state) | Pure `State` transitions. | `go4t run examples/state/main.gt` |
| [collections](collections) | Persistent Map/Set/Array. | `go4t run examples/collections/main.gt` |
| [mutmap](mutmap) | IO-only ordered word-frequency counting; freeze into a pure Map and print a deterministic top three. | `go4t run examples/mutmap` |
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
