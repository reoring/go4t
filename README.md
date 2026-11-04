# go4t — Go for types

[日本語](README.ja.md)

go4t is a small, statically typed pure functional language that compiles to
ordinary Go. Its distinguishing feature is **checked equational laws over
pure code**: state invariants as equations and have `go4t check` verify them
for every input. Algebraic data types and exhaustive pattern matching make
states explicit; effects stay in `IO`.

go4t binds Go packages at import time (`import "go:net/http"`) and ships
IO-only mutable references (`Ref`), channels, fibers, and file streaming.
It has been used for an end-to-end Kubernetes controller against a real
cluster, ordinary CLI and HTTP programs, and a 5.4k-line self-hosting compiler.
You can write a whole program in go4t or embed a pure component in Go, as in
the decision function below.

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
`missing: (TtlNanos(_), InvalidTimestamp)`. Add a law —
`isStale(freshness(TtlNanos(t), AtNanos(at), now)) == (now >= at + t)` — and
moving the guard behind another check fails it for all `t`, `at`, `now`, not
for the handful of samples a table test happens to list.

## Why not just Go?

In a reconciler, a workflow engine, a policy evaluator or a pricing rule, the
hard part is often a decision over many partially-known facts. Go expresses
those facts as `(value, ok)` pairs, sentinel values, nullable pointers and
boolean flags; the decision becomes a ladder of early returns whose coverage
only tests can establish. go4t makes those states and cases explicit and lets
you check equations over the pure code.

| Need in a decision core | go4t | Go |
| --- | --- | --- |
| A state that is one of N named things | sum types (`A \| B(Int) \| C`) | consts + comments, or `*T` + `bool` |
| "Did I handle every case?" | exhaustive `match`, compile error listing the missing patterns | `default:` and hope |
| Errors as data | `Result[E, A]`, `Option[A]` with `do` sequencing | `(T, error)` and `if err != nil` |
| Guarantee the core performs no I/O | effects are `IO[A]` values; a pure function cannot run one | discipline |
| Equality, ordering, debug printing of nested data | `deriving Eq, Ord, Debug, Show` | hand-written or `reflect.DeepEqual` |
| Generic operations over containers | type classes, higher-kinded (`Functor`, `Monad`, `Monoid`) | per-type copies |
| "This invariant holds for every input" | `law` + `proof`, checked at compile time for all values | table tests over chosen samples, or property-based testing (`rapid`) over random ones |

Everything compiles to readable Go and uses `go.mod` for dependencies;
Go packages are called through typed bindings generated at import time.
Runtime cost is measured, not guessed ([bench/PERF.md](bench/PERF.md)):
kernels whose values stay unboxed run at 1.6–4x of Go; updating a persistent
`Map` while folding costs 19–24x (the price of persistence, as with other
persistent collections); the IO-only `std/mutmap` alternative is at 5x after
direct-style IO lowering. Decision cores and controllers read their maps and
pay none of the update cost. Further work waits for a workload that demands it.

The last row deserves a caveat: on the same decision core, property-based
testing with `rapid` caught every mutation we tried; what laws add is a proof
instead of a probability. For a `decide` with two `if`s, stay in Go. The
side-by-side comparison, the two production controller ports and the five
law rounds are recorded in [docs/evaluation.md](docs/evaluation.md).

## Use cases

go4t supports whole CLI, HTTP and controller programs, with effects in `IO`
and laws over their pure parts. **Decisions with many input combinations
where the order of checks matters** are a particularly useful place for laws.
The examples below show that pattern, including embedding a go4t `decide` in Go.

| Use case | What the decision core does | What laws can pin | Example |
| --- | --- | --- | --- |
| Kubernetes controller reconcile | Pick the one API operation to perform next from observed resource state | Order ("cordon before evict"), idempotence, exact timeout boundaries | [approval-controller](examples/approval-controller), [attachment-repair-controller](examples/attachment-repair-controller), [node-drain-controller](examples/node-drain-controller) |
| Whole controller in go4t | Same as above, with `observe`/`apply` written in go4t against `go:k8s.io/client-go` | The above plus projection laws: "a pod owned by a DaemonSet projects to `DaemonSet`", "the mirror annotation wins" | [node-drain-typed](examples/node-drain-typed) |
| Go-owned client, go4t controller | Pass a native `*kubernetes.Clientset` or controller-runtime `client.Client` from Go to exported go4t `observe`/`apply`/`reconcile` IO functions | The same 15 decision and projection laws, with a Go-owned reconcile loop; a platform controller's observe step moved into go4t with its six tests unchanged | [node-drain-shell](examples/node-drain-shell), [approval-controller](examples/approval-controller) |
| Admission / authorization gates | Run checks in order and return the first failure reason | Gate order, rejection at boundary values such as a negative quota | the admission round in ADR 0002 |
| Placement and scheduling | Choose a target from `Map` capacity and usage, with fallback | A full zone is never chosen; empty capacity rejects for any fallback list | `laws/testdata/placement.gt`, the zone-placement round in ADR 0002 |
| Multi-step workflows | Decide which stage (observe → authorize → compare-and-swap delete → publish) applies and return the next step | No stage left unhandled (exhaustiveness), step order | [attachment-repair-controller](examples/attachment-repair-controller) |
| Policy evaluation and pricing rules | Decide from partially known facts (deadlines, TTLs, flags) | The `>=` / `>` boundary on a deadline; no invalid-input case dropped | `freshness` at the top of this README |
| Safe refactoring | Rewrite an existing `decide` structurally | `decide(o) == decideV2(o)` proved for every input | `laws/testdata/refactor_v2.gt` |

For decisions that fit in two `if`s, plain Go may be simpler; I/O code can
be written in either language, but laws apply only to pure code.
See [What go4t is not](#what-go4t-is-not).

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
  lazy values, `State`, IO-only `Ref`/`MutMap`/channels, concurrency, file/process I/O.
- Go interop both ways: `import "go:net/http"` binds any Go package at build
  time (functions, methods, fields, tuples, upcasts; nothing committed);
  `go4t build --emit-go` exports go4t functions, records and sums as natural Go
  types.
- Generic Go functions use call-site monomorphisation; generic receiver methods and instantiated generic named types are still skipped.
- Extern-dependent laws can use `sample(N)`: `go4t test` tests them, without proving them or making them rewrite lemmas.
- Tooling: `check`, `build`, `run`, `test`, `eval`, `fmt`, LSP server,
  VS Code extension, source-located diagnostics with `^~~~` spans.
- Go-flavoured syntax: `func`, `package`, `import`, braces, `|>` pipelines,
  `_` call placeholders.

### Laws

State an equation once and have `go4t check` verify it for every input:

```go4t
package main
law distribute(a Int, b Int, c Int) { (a + b) * c == a * c + b * c }
proof distribute(a, b, c) { refl }
func main() IO[Unit] { ioPure(unit()) }
```

Laws use definitional equality over first-order data; proofs combine `refl`,
constructor `match`, guard `if`, `rewrite` and `induction`. Proved laws
generate no runtime code. What laws can and cannot express is listed in
[docs/laws.md](docs/laws.md); worked examples are in
[tutorial §13](docs/tutorial.md#13-laws).

## Install and quick start

Requires **Go 1.22+** on PATH (go4t generates Go and runs `go build`).

```sh
go install ./cmd/go4t
go4t eval '1 + 2'        # 3
```

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
go4t build hello/main.gt      # builds ./main; keeps Go sources in hello/main_gen/
go4t build --emit-go generated hello/main.gt  # emits Go sources only
```

## Embedding in a Go program

1. Write the decision in `decide/decide.gt`: a projection record `Observed`,
   an action sum, and `func decide(o Observed) Action`.
2. Generate: `go4t build --emit-go decide --package decide decide/decide.gt`
   (put it in a `//go:generate` line).
3. In Go: read the world, fill `Observed`, call `Decide`, switch on the result,
   perform the effects. Nothing in `decide` can perform I/O; the compiler
   enforces it.

Runnable template: [examples/go-shell](examples/go-shell). To write the
shell in go4t instead, import the Go packages directly
(`import "go:k8s.io/client-go/kubernetes"`); handles and anything that
observes shared state are `IO`, plain Go structs become records, and the
bindings are generated and cached at build time. Boundary rules, `go:`
imports and project/dependency layout are in
[docs/go-interop.md](docs/go-interop.md).

## What go4t is not

- **Not a requirement to replace Go.** Write a whole program in go4t or keep
  existing Go infrastructure and embed pure components; laws do not prove
  the behavior of I/O.
- **Not an effect-system research language.** There is one effect type, `IO`;
  mutable references, channels and other shared-state operations are IO-only.
- **Not a proof assistant.** Laws are an equational checker over first-order
  data with a fixed tactic set; see [docs/laws.md](docs/laws.md).
- **Not 1.0.** The grammar, std and ABI are stable enough for the ports above
  and are documented in SPEC.md, but they can still change. Known limits:
  [SPEC §9](SPEC.md#9-known-limitations).

## Documentation

| Document | Contents |
| --- | --- |
| [Tutorial for Go developers](docs/tutorial.md) | Seventeen short lessons; every go4t snippet type-checked in CI. |
| [Language specification](SPEC.md) | Grammar, type and effect rules, deriving, exported Go API, boundary tables. |
| [Laws](docs/laws.md) | What laws cover, what they do not, how proofs are written. |
| [Go interop](docs/go-interop.md) | Embedding rules, `go:` imports, projects and dependencies. |
| [CLI reference](docs/cli.md) | Every command, exit codes. |
| [Examples](examples/README.md) | All runnable examples with their commands. |
| [Evaluation](docs/evaluation.md) | Production ports, law rounds, the Go/`rapid` comparison, the `client-go` scale experiment. |
| [Standard library](docs/stdlib.md) | Generated from the source inventory. |
| [Editor setup](editors/README.md) | stdio LSP and a VS Code client. |
| [Design history](DESIGN.md) | Slice contracts; SPEC wins where they differ. |

## License

Apache License 2.0. See [LICENSE](LICENSE).
