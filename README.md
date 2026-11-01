# go4t — Go for types

[日本語](README.ja.md)

go4t is a small, statically typed functional language that compiles to ordinary
Go. It exists for one job: **writing the decision core of a Go program** — the
part that turns observed state into a next step — in a form where every state
has a name, every case is checked, and nothing performs I/O.

Go stays the shell. go4t is the core. You do not rewrite your service in go4t;
you move the `if ok { … } else if … { return … }` ladders into it and call the
result from Go as a typed function.

```go4t
package decide

type Ttl       = NoTtl | InvalidTtl | TtlNanos(Int)                 deriving Eq, Debug
type Timestamp = NoTimestamp | InvalidTimestamp | AtNanos(Int)      deriving Eq, Debug
type Freshness = Fresh(Int) | TtlInvalid | TimestampMissing
               | TimestampInvalid | Stale                            deriving Eq, Debug

func freshness(ttl Ttl, observedAt Timestamp, nowNanos Int) Freshness {
  match (ttl, observedAt) {
    (NoTtl, _)                                          => Fresh(0)
    (InvalidTtl, _)                                     => TtlInvalid
    (TtlNanos(_), NoTimestamp)                          => TimestampMissing
    (TtlNanos(_), InvalidTimestamp)                     => TimestampInvalid
    (TtlNanos(t), AtNanos(at)) if nowNanos >= at + t    => Stale
    (TtlNanos(t), AtNanos(at))                          => Fresh(at + t - nowNanos)
  }
}

func main() IO[Unit] { println(debug(freshness(TtlNanos(10), AtNanos(5), 12))) }
```

`go4t build --emit-go decide --package decide decide/decide.gt` turns that into
a Go package. The Go side fills the inputs and switches on the result:

```go
switch step := decide.Freshness(ttl, observedAt, now.UnixNano()).(type) {
case decide.FreshnessStale:
    return requeue(ctx, obj)
case decide.FreshnessTtlInvalid:
    return reject(ctx, obj, "evidence TTL invalid")
// …
}
```

Remove an arm from the `match` and `go4t check` fails with
`missing: (TtlNanos(_), InvalidTimestamp)`. That is the whole value proposition.

## Why not just Go?

Go is a good shell language and a weak core language. In a reconciler, a
workflow engine, a policy evaluator or a pricing rule, the hard part is a
decision over many partially-known facts. Go expresses those facts as
`(value, ok)` pairs, sentinel values, nullable pointers and boolean flags; the
decision becomes a ladder of early returns whose coverage only tests can
establish, and tests rarely enumerate every combination.

go4t gives the core what Go lacks:

| Need in a decision core | go4t | Go |
| --- | --- | --- |
| A state that is one of N named things | sum types (`A \| B(Int) \| C`) | consts + comments, or `*T` + `bool` |
| "Did I handle every case?" | exhaustive `match`, compile error listing the missing patterns | `default:` and hope |
| Errors as data | `Result[E, A]`, `Option[A]` with `do` sequencing | `(T, error)` and `if err != nil` |
| Guarantee the core performs no I/O | effects are `IO[A]` values; a pure function cannot run one | discipline |
| Equality, ordering, debug printing of nested data | `deriving Eq, Ord, Debug, Show` | hand-written or `reflect.DeepEqual` |
| Generic operations over containers | type classes, higher-kinded (`Functor`, `Monad`, `Monoid`) | per-type copies |

Everything compiles to readable Go, runs at Go speed, and uses `go.mod` for
dependencies. Any Go package can be called through generated typed bindings.

## Where it has been used, and what we learned

The language was validated by porting real code, not by writing samples.

**Two production Kubernetes controllers** (`examples/approval-controller`,
`examples/attachment-repair-controller`) had their decision logic moved to
go4t while the original Go test files stayed byte-identical as the oracle.
All 6 + 22 original tests pass unchanged. The approval port surfaced a decision
branch the original tests never exercised; the exhaustiveness checker refused
to compile until it was handled. The attachment port moved a multi-step
workflow (observe → authorize → compare-and-swap delete → publish) into a
pure `Decide(Observed) Action` that Go calls up to three times per reconcile.

Each readability fix in those ports was the same correction — a Go
`(value, ok)`, flag or sentinel became a constructor:

| Go habit in the input | Became | Effect |
| --- | --- | --- |
| `gatesValid: Bool` + `gates: List[Gate]` | `gates: List[GateEvidence]`, `GateEvidence = Gate \| InvalidGate` | first-failure order identical to the Go loop |
| `Option[Int]` with "≤ 0 means invalid" | `Ttl = NoTtl \| InvalidTtl \| TtlNanos(Int)` | the invalid case cannot be forgotten |
| `present: Bool` + `Option[Int]` | `Timestamp = NoTimestamp \| InvalidTimestamp \| AtNanos(Int)` | one match instead of nested if/match, 20 → 11 lines |
| `fromOption(gen, -1)` | `gen != Some(plan.generation)` | no sentinel |

**A 110 k-line port of `client-go`** was done as a scale experiment, not as a
product. It compiled (after three compiler fixes — a bounded test harness, and
direct lowering of result-position `match` and `if`/blocks that otherwise became
closure-heavy codegen — that are now part of the language), but it showed the
boundary clearly: an infrastructure layer is mostly state, caches and I/O, where
pure functions and exhaustiveness buy little and the Go-binding layer grows
larger than the code it serves. The conclusion is the design rule above —
**keep go4t for the decision core; bind, do not port, the infrastructure.**
The port lives on the `experiment/client-go-gofx` branch (`ports/client-go`) and
as the standalone repository
[reoring/client-go-go4t](https://github.com/reoring/client-go-go4t), whose
`RESULTS.md` and `CODE_STATS.md` record what was verified and the before/after
code statistics.

## Language at a glance

- Algebraic data types, records with immutable update, transparent aliases,
  tuples, persistent `List`/`Map`/`Set`/`Array`.
- Hindley–Milner inference with rank-1 polymorphism; type classes with
  superclasses and higher kinds; `deriving Eq, Ord, Show, Debug, Functor,
  Foldable, Traversable`, JSON `Encode`/`Decode`.
- Exhaustive `match` with guards, tuple and list patterns, pattern `let`.
- Explicit effects: `IO[A]`, `do`-style blocks, typed `IoError` recovery,
  fibers (`fork`/`await`/race/timeout) with cancellation.
- `std`: strings, numbers, collections, JSON, HTTP client/server, regex,
  lazy values, `State`, concurrency, file/process I/O.
- Go interop both ways: `go4t bind` generates typed externs for any Go package
  (including methods on opaque handles); `go4t build --emit-go` exports go4t
  functions, records and sums as natural Go types.
- Tooling: `check`, `build`, `run`, `test`, `eval`, `fmt`, LSP server,
  VS Code extension, source-located diagnostics with `^~~~` spans.
- Go-flavoured syntax: `func`, `package`, `import`, braces, `|>` pipelines,
  `_` call placeholders.

Full reference: [SPEC.md](SPEC.md). Guided introduction for Go developers:
[docs/tutorial.md](docs/tutorial.md). Every std declaration:
[docs/stdlib.md](docs/stdlib.md).

## Install and quick start

Requires **Go 1.22+** on PATH (go4t generates Go and runs `go build`).

```sh
go install ./cmd/go4t
go4t eval '1 + 2'        # 3
```

The binary embeds the runtime, so `run`, `test` and `eval` work from any
directory without a `go.mod`. Without installing, use `go run ./cmd/go4t`.

Create `hello/main.gt`:

```go4t
package main
import "std/string"
func main() IO[Unit] {
  let names = [" Ada ", " Bob "] |> map(string.trim)
  println("Hello, " + string.join(" and ", names) + "!")
}
```

```sh
go4t check hello/main.gt
go4t run hello/main.gt        # Hello, Ada and Bob!
go4t build hello/main.gt      # emits Go source into hello/main_gen/
```

## Embedding in a Go program

This is the intended workflow.

1. Write the decision in `decide/decide.gt`: a projection record `Observed`,
   an action sum, and `func decide(o Observed) Action`.
2. Generate: `go4t build --emit-go decide --package decide decide/decide.gt`
   (put it in a `//go:generate` line). The package exports `Decide`, Go mirror
   types for every record and sum (`ObservedGo`, `ActionRollout{}`,
   `TtlTtlNanos{F0: …}`), and `Main() rt.IO`.
3. In Go: read the world, fill `Observed`, call `Decide`, switch on the result,
   perform the effects. Nothing in `decide` can perform I/O; the compiler
   enforces it.

Rules that keep the boundary clean (SPEC §8.7, tutorial §12):

- Project only what the decision needs; every call copies the record.
- Never cross a sentinel. `Option[Int]` crosses as a pointer-free mirror;
  compare with `Some(x)`, do not unwrap with `-1`.
- Cross durations and timestamps as `Int` nanoseconds — the representation of
  `time.Duration` and `UnixNano` — so no rounding happens at the boundary.
- A validity flag beside a list becomes a sum per element; "first failure in
  order" then matches the original Go loop exactly.

Runnable template: [examples/go-shell](examples/go-shell). Real ports:
[examples/approval-controller](examples/approval-controller),
[examples/attachment-repair-controller](examples/attachment-repair-controller).

## Calling Go from go4t

```sh
go4t bind strings --prefix str          # prints extern declarations
go4t bind modernc.org/sqlite -o bindings.gt
```

Generated externs cover functions, methods on opaque handles, slices, options
and errors; signatures that cannot be mirrored get a skip comment instead of a
wrong binding. [examples/sql-service](examples/sql-service) is an HTTP service
over SQLite with Prometheus metrics built this way; [examples/typed-extern](examples/typed-extern)
shows the ABI adapters. `std/http` needs no bindings.

## Projects and dependencies

A go4t library is a Go module with `go.mod`, `go4t.mod` and `.gt` files.

```sh
go4t init example.com/myapp            # creates go.mod + go4t.mod
go4t get example.com/lib@v1.0.0        # go get, then record in go4t.mod
```

Go selects versions and keeps checksums; `go4t.mod` lists language imports.
Imports are file-local and qualified (`import "std/io"` → `io.readFile`); only
the small core prelude is implicit. Resolution order: the importing module's
source root, `go4t.mod` requires, `GO4T_PATH`, embedded std. See
[examples/deps](examples/deps) for an offline local-replacement setup.

## CLI

| Command | Purpose |
| --- | --- |
| `go4t check [--full-types] FILE.gt\|DIR...` | Load and type-check; warnings do not fail; foreign ABI is not checked here. |
| `go4t build [--full-types] [-o DIR] FILE.gt\|DIR...` | Emit one Go file per go4t module into one Go package. |
| `go4t build --emit-go DIR --package NAME FILE.gt\|DIR...` | Emit an importable Go package with typed exports (§8.7). |
| `go4t run [--full-types] FILE.gt\|DIR... [-- args...]` | Build and execute; inherits stdin; preserves exit status. |
| `go4t test FILE.gt\|DIR...` | Run zero-arg `testName` functions returning `Bool` or `IO[Bool]`. |
| `go4t eval 'EXPR' [FILE.gt\|DIR...]` | Evaluate a pure value or IO result; imports come from the first file. |
| `go4t bind IMPORT [-o FILE] [--prefix STR]` | Generate Go extern declarations. |
| `go4t fmt [-w\|-d\|-l] FILE...\|DIR` | Canonical formatting; `-l` exits 1 on differences. |
| `go4t init <module-path>` / `go4t get <path>@<version>` | Project and dependency management. |
| `go4t lsp` | Stdio language server (diagnostics, hover, definition, symbols). |

Exit codes: ordinary IO errors 1, uncaught action panic 2. Recover inside the
program with `io.tryIO(action) : IO[Result[io.IoError, A]]`.

## Examples

Commands run from the checkout root. Network-using examples are marked.

| Example | Shows | Run |
| --- | --- | --- |
| [go-shell](examples/go-shell) | Minimal Go shell + go4t core template. | `go4t run examples/go-shell/core.gt` |
| [approval-controller](examples/approval-controller) | Real controller; 6 original tests unchanged. | `cd examples/approval-controller && go generate ./... && go test ./...` |
| [attachment-repair-controller](examples/attachment-repair-controller) | Multi-step workflow decided in go4t; 22 original tests unchanged. | `cd examples/attachment-repair-controller && go generate ./... && go test ./...` |
| [sql-service](examples/sql-service) | SQLite + Prometheus HTTP service via `bind`. | see its README |
| [typed-extern](examples/typed-extern) | Generated bindings, ABI adapters, opaque method handles. | `go4t run examples/typed-extern` |
| [deps](examples/deps) | Offline Go-module dependency with local replacement. | `go4t run examples/deps/app` |
| [calc](examples/calc) | Expression interpreter with `Result` errors. | `go4t run examples/calc/main.gt` |
| [cards](examples/cards) | Poker classification, derived ordering, seeded `State`. | `go4t run examples/cards/main.gt` |
| [scheduler](examples/scheduler) | Custom ordering, guards, bounded plan. | `go4t run examples/scheduler/main.gt` |
| [routes](examples/routes) | Shortest paths and record updates. | `go4t run examples/routes/main.gt` |
| [parser](examples/parser) | User-defined parser monad. | `go4t run examples/parser/main.gt` |
| [monoid](examples/monoid) | User `Monoid` instances with generic folds. | `go4t run examples/monoid/main.gt` |
| [multi-constraint](examples/multi-constraint) | Multiple classes per parameter, superclasses. | `go4t run examples/multi-constraint/main.gt` |
| [deriving](examples/deriving) | Derived `Functor`/`Foldable`/`Traversable`/`Debug`. | `go4t run examples/deriving/main.gt` |
| [json-deriving](examples/json-deriving) | JSON schema derivation and nested error paths. | `go4t run examples/json-deriving/main.gt` |
| [http](examples/http) | `std/http` server/client, JSON routes, cancellation (loopback). | `go4t run examples/http/main.gt -- 8080` |
| [sales-report](examples/sales-report) | Parallel fetches, typed IO recovery, fallbacks. | `go4t run examples/sales-report` |
| [concurrency](examples/concurrency) | Fork/await, ordered parallel work, race, timeout. | `go4t run examples/concurrency/main.gt` |
| [state](examples/state) | Pure `State` transitions. | `go4t run examples/state/main.gt` |
| [collections](examples/collections) | Persistent Map/Set/Array. | `go4t run examples/collections/main.gt` |
| [lists](examples/lists) | List Functor/Monad, cartesian `do`. | `go4t run examples/lists/main.gt` |
| [tree](examples/tree) | Recursive Functor and constrained rendering. | `go4t run examples/tree/main.gt` |
| [records](examples/records) | Records, interpolation, let polymorphism. | `go4t run examples/records/main.gt` |
| [alias-fields](examples/alias-fields) | Aliases and common-field updates. | `go4t run examples/alias-fields/main.gt` |
| [eqord](examples/eqord) | Custom equality/ordering. | `go4t run examples/eqord/main.gt` |
| [float-num](examples/float-num) | Float parsing/formatting, IEEE edges. | `go4t run examples/float-num/main.gt` |
| [strings](examples/strings) | Unicode strings, parsing, padding. | `go4t run examples/strings/main.gt` |
| [stdlib-extras](examples/stdlib-extras) | Regex, stable grouping, tables, `Lazy`. | `go4t run examples/stdlib-extras/main.gt` |
| [pipe-partial](examples/pipe-partial) | Pipelines, `_` placeholders, sections. | `go4t run examples/pipe-partial/main.gt` |
| [local-recursion](examples/local-recursion) | Recursive lambdas, mutual local recursion. | `go4t run examples/local-recursion/main.gt` |
| [io-effects](examples/io-effects) | File read/validate/write/append. | `out=$(mktemp -d); go4t run examples/io-effects/main.gt -- examples/io-effects/config.txt "$out/report"` |
| [wc](examples/wc) | Stdin word frequencies. | `go4t run examples/wc/main.gt -- 3 < examples/wc/input.txt` |
| [cli-tools](examples/cli-tools) | Pure/IO tests and eval; also `go4t test examples/cli-tools/main.gt`. | `go4t run examples/cli-tools/main.gt` |
| [modules](examples/modules) | Directory loading, aliases, shadow warning. | `go4t run examples/modules` |
| [lsp](examples/lsp) | Cross-module hover/definition playground. | `go4t run examples/lsp/main.gt` |
| [diagnostics](examples/diagnostics) | Exhaustiveness and alias-mismatch diagnostics (`bad_*.gt` fail on purpose). | `go4t run examples/diagnostics/main.gt` |
| [fmt](examples/fmt) | Formatter input; `go4t fmt -d examples/fmt/before.gt`. | `go4t run examples/fmt/before.gt` |
| [codegen-quality](examples/codegen-quality) | Million-step tail recursion, source locations. | `go4t run examples/codegen-quality/main.gt` |
| [perf](examples/perf) | Native scalars, bounded specialization, tail loops. | `go4t run examples/perf/main.gt` |
| [bank](examples/bank), [inventory](examples/inventory), [orders](examples/orders) | Domain validation with HTTP notification (**network**). | `go4t run examples/<name>/main.gt` |

Negative fixtures: `go4t test examples/cli-tools/failing.gt` exits 1,
`go4t check examples/diagnostics/bad_hint.gt` exits 1,
`go4t check examples/diagnostics/bad_unused.gt` warns and exits 0.

## What go4t is not

- **Not a Go replacement.** Services, caches, clients and anything that is
  mostly I/O stay in Go. Bind them; do not port them.
- **Not an effect-system research language.** There is one effect type, `IO`,
  and no mutable references in the language. If a design needs shared mutable
  state, that state belongs on the Go side of the boundary.
- **Not 1.0.** The grammar, std and ABI are stable enough for the ports above
  and are documented in SPEC.md, but they can still change. Known limits are
  listed in [SPEC §9](SPEC.md#9-known-limitations): rank-1 polymorphism only,
  no multi-parameter classes, no character type, selective unboxing,
  `check` cannot validate a foreign Go signature (`build`/`run` can).

## Documentation and editors

- [Tutorial for Go developers](docs/tutorial.md) — twelve short lessons ending
  with the Go-shell/go4t-core pattern; every snippet is type-checked in CI.
- [Language specification](SPEC.md) — grammar, type and effect rules, CLI,
  deriving, exported Go API, boundary tables.
- [Standard library reference](docs/stdlib.md) — generated from the source
  inventory.
- [Editor setup](editors/README.md) — stdio LSP and a VS Code client
  (`go4t.executablePath`).
- [Design history](DESIGN.md) — slice contracts; SPEC wins where they differ.

Documentation regression: `go test ./docs ./std` extracts every `go4t` fence
from SPEC, tutorial, stdlib and this README and runs the same loader and type
checker as `go4t check`; `go4t error` fences must be rejected.

`selfhost/` is a separately owned in-progress self-hosting compiler and is not
part of the release surface.

## License

Apache License 2.0. See [LICENSE](LICENSE).
