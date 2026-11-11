# go4t — Go for types

[日本語](README.ja.md)

go4t is a small programming language that compiles to ordinary Go. You write
the parts of a program where *getting every case right* matters — a
reconciler's decision, an authorization gate, a pricing rule — and go4t gives
you three things Go does not:

1. **States as data.** A value is one of several named cases (`NoTtl |
   InvalidTtl | TtlNanos(Int)`), and a `match` that forgets a case is a
   compile error that names the missing pattern.
2. **A line between pure code and effects.** Functions that do I/O return
   `IO[A]` values; a pure function cannot call one. The compiler enforces it.
3. **Laws.** You state an equation about your pure code — *stale exactly
   when the TTL has elapsed*,
   `isStale(freshness(TtlNanos(t), AtNanos(at), now)) == (now - at >= t)` —
   and `go4t check` proves it for **every** input in the range the law names,
   not for the samples a test happens to list.

Everything else is deliberately ordinary: the output is readable Go, builds
with `go build`, uses `go.mod` for dependencies, and calls any Go package
through bindings generated at build time (`import "go:net/http"`). You can
write a whole program in go4t, or keep your Go program and move one function
into it.

## Try it in two minutes

Requires **Go 1.22+** on PATH.

```sh
go install github.com/reoring/go4t/cmd/go4t@latest
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
go4t check hello/main.gt      # type-check (and prove laws, if any)
go4t run hello/main.gt        # Hello, Ada and Bob!
go4t build hello/main.gt      # builds ./main; keeps Go sources in hello/main_gen/
```

If you know Go, the [tutorial](docs/tutorial.md) is eighteen short lessons
that start from Go habits and show what changes.

## A small decision, end to end

Here is the kind of code go4t is for. A controller must decide whether an
observation is still fresh, given a TTL and a timestamp that may each be
missing or invalid:

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

Three things to notice:

- The inputs are **sums**, not `*time.Time` plus a `bool`. Every combination
  the function must handle is visible in its signature.
- Delete the `(TtlNanos(_), InvalidTimestamp)` arm and `go4t check` fails with
  `missing: (TtlNanos(_), InvalidTimestamp)`.
- Add a **law** and moving the `Stale` guard behind another check, or
  turning `>=` into `>`, fails it for all `t`, `at`, `now`. The proof is
  usually one word (`refl`) or a case split:

  ```text
  law staleOnceElapsed(t Int, at Int, now Int)
    requires 0 <= t && t <= 1000000000000 && 0 <= at && at <= now && now <= 4000000000000000000 {
    isStale(freshness(TtlNanos(t), AtNanos(at), now)) == (now - at >= t)
  }
  proof staleOnceElapsed(t, at, now) { if now - at >= t { refl } else { refl } }
  ```

  The `requires` clause is the honest part. `Int` wraps, so "elapsed" only
  means what it says while `at + t` cannot overflow. Delete the clause and
  `go4t check` rejects the law with the input that breaks it:
  `counterexample: t = 9223372036854775807, at = 1, now = 0`.

To use it from Go, run `go4t build --emit-go decide --package decide
decide/decide.gt` (typically from a `//go:generate` line). The generated
package exports each function and plain Go types for every record and sum, and
the Go side switches on the result. (`freshness` returns the type `Freshness`,
so the exported function is `FreshnessFn`; a function named `decide` would be
`decide.Decide`.)

```go
switch step := decide.FreshnessFn(ttl, observedAt, now.UnixNano()).(type) {
case decide.FreshnessStale:
    return requeue(ctx, obj)
case decide.FreshnessTtlInvalid:
    return reject(ctx, obj, "evidence TTL invalid")
// …
}
```

[examples/go-decision-template](examples/go-decision-template) is this exact
decision as a Go module you can copy: one `.gt` file, one `//go:generate`
line, a Go API in front of it, and a Go test.

### The same decision with its I/O in go4t

The shell around a decision does not have to be Go. Effects are values of
type `IO[A]`, and a `do` block sequences them; `x <- action` runs an action
and binds its result, while a bare expression is just a pure value. Reading
the world and turning what was read into a `Timestamp` are two different
jobs, so they are two different functions:

```go4t
package main
import "std/io"
import "std/string"

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

// Reading the file is IO; deciding what the text means is pure.
func observedAt(path String) IO[Timestamp] {
  do {
    read <- io.tryIO(io.readFile(path))
    match read {
      Err(io.NotFound(_)) => NoTimestamp
      | Err(e)            => io.ioRaise(e)
      | Ok(text)          => match string.parseInt(string.trim(text)) {
        Some(nanos) => AtNanos(nanos)
        None        => InvalidTimestamp
      }
    }
  }
}

func main() IO[Unit] {
  do {
    at     <- observedAt("/var/run/last-seen")
    millis <- io.nowMillis()
    match freshness(TtlNanos(30 * 1000000000), at, millis * 1000000) {
      Fresh(left)      => println("fresh for ${left / 1000000} ms more")
      Stale            => println("stale: re-observe")
      TimestampMissing => println("never observed")
      | other          => io.ioFail("invalid input: " + debug(other))
    }
  }
}
```

`io.tryIO` turns a failing action into a `Result` you can match on, so a
missing file becomes the `NoTimestamp` case instead of an exception; any
other failure is re-raised with `io.ioRaise`. A `match` arm may return a plain
value (`NoTimestamp`) or an action (`io.ioFail(...)`); both are fine where an
`IO` is expected. With no file the program prints `never observed`; with a
file containing a recent UnixNano it prints `fresh for 29170 ms more`; with
garbage it exits 1 with `invalid input: TimestampInvalid`. Nothing in
`freshness` changed, and its laws still hold.

## How it fits into a Go program

You choose how much to move. All three shapes have runnable examples and
share the same decision code:

| Shape | Go keeps | go4t owns | Example |
| --- | --- | --- | --- |
| Go shell, go4t decision | the client, the reconcile loop, observe and apply | `decide` | [go-shell](examples/go-shell), [node-drain-controller](examples/node-drain-controller) |
| Go shell, go4t controller | the client and the loop | `observe`, `decide`, `apply` as exported IO functions that receive the Go client as a native handle | [node-drain-shell](examples/node-drain-shell), [approval-controller](examples/approval-controller) (a real controller: Go went from 271 to 88 lines, its six tests unchanged) |
| Whole program in go4t | nothing; `main` is go4t | everything, calling Go packages through `import "go:…"` | [node-drain-typed](examples/node-drain-typed) (ran a 3-hour soak on a kind cluster) |

The rule at the boundary is simple: **it never converts lossily**. Go structs
whose fields are all exported are copied as records; everything else (a
`*kubernetes.Clientset`, an `error`, a `time.Time` inside a struct) crosses by
identity as an opaque handle. Anything that can lose information is a named
function you call on purpose. Details, recipes and the generated names are in
[docs/go-interop.md](docs/go-interop.md).

## Why not just Go?

In a reconciler, a workflow engine, a policy evaluator or a pricing rule, the
hard part is a decision over many partially-known facts. Go expresses those
facts as `(value, ok)` pairs, sentinel values, nullable pointers and boolean
flags; the decision becomes a ladder of early returns whose coverage only
tests can establish.

| Need in a decision core | go4t | Go |
| --- | --- | --- |
| A state that is one of N named things | sum types (`A \| B(Int) \| C`) | consts + comments, or `*T` + `bool` |
| "Did I handle every case?" | exhaustive `match`, compile error listing the missing patterns | `default:` and hope |
| Errors as data | `Result[E, A]`, `Option[A]` with `do` sequencing | `(T, error)` and `if err != nil` |
| Guarantee the core performs no I/O | effects are `IO[A]` values; a pure function cannot run one | discipline |
| Equality, ordering, debug printing of nested data | `deriving Eq, Ord, Debug, Show` | hand-written or `reflect.DeepEqual` |
| Generic operations over containers | type classes, higher-kinded (`Functor`, `Monad`, `Monoid`) | per-type copies |
| "This invariant holds for every input" | `law` + `proof`, checked at compile time for all values | table tests over chosen samples, or property-based testing (`rapid`) over random ones |

Be honest about the last row: on the same decision core, property-based
testing with `rapid` caught every mutation we tried; what a law adds is a
proof instead of a probability. For a `decide` with two `if`s, stay in Go.

Where a proof reaches and a generator does not is input you would have to
shape by hand. The node-drain controller must leave DaemonSet pods alone, and
a pod is a DaemonSet pod if *any* of its owner references says so:

```text
law daemonSetOwnerAnywhere(prefix List[meta.OwnerReference], owner meta.OwnerReference, rest List[meta.OwnerReference]) {
  ownedByDaemonSet(append(prefix, Cons(owner{kind: "DaemonSet"}, rest))) == true
}
```

`meta.OwnerReference` is the real `k8s.io/apimachinery` type, bound with
`import "go:…"`. The law quantifies over every prefix, every suffix and every
other field of the owner; its proof is an induction on `prefix`
([examples/node-drain-typed](examples/node-drain-typed/decide.gt)). A `rapid`
test of the same property starts by writing a generator for owner-reference
lists, and is then a statement about the lists that generator produces.

**Speed.** Runtime cost is measured, not guessed ([bench/PERF.md](bench/PERF.md)).
Code whose values stay unboxed runs at 1.6–4x of the equivalent Go. Updating
a persistent `Map` inside a hot loop costs 19–24x — that is the price of
persistence, as with any persistent collection; the IO-only `std/mutmap`
alternative runs at 5x. Decision cores and controllers *read* their maps and
pay none of the update cost.

## Laws, briefly

A law is an equation over pure code; a proof says how to check it. Most
proofs are short:

```go4t
package main
law distribute(a Int, b Int, c Int) { (a + b) * c == a * c + b * c }
proof distribute(a, b, c) { refl }
func main() IO[Unit] { ioPure(unit()) }
```

Proofs combine `refl`, constructor `match`, guard `if`, `rewrite` and
`induction`. Proved laws generate no runtime code. Laws cover first-order
data (sums, records, lists, maps, `Int`, `String`, …) and the pure functions
over them; they do not cover I/O, and an equation that depends on an opaque
Go function can only be *sampled* (`sample(N)`), which `go4t test` runs
without treating it as proved. What laws can and cannot express is in
[docs/laws.md](docs/laws.md); worked examples are in
[tutorial §14](docs/tutorial.md#14-laws).

## Where it has been used

- **Kubernetes controllers.** Two production controllers from the AppThrust
  platform were ported with their original Go tests kept as the oracle
  ([approval-controller](examples/approval-controller),
  [attachment-repair-controller](examples/attachment-repair-controller)). A
  node-drain controller exists in all three shapes above; the all-go4t one
  ran 839/839 drain/recover cycles in a three-hour soak.
- **Ordinary programs.** CLI tools, an HTTP service, and programs written
  from the docs alone by four coding agents that had never seen the compiler
  ([docs/evaluation.md](docs/evaluation.md) records what they tripped over
  and what was changed). Everything above was done by the project itself; no
  one outside it has used go4t yet.
- **Itself.** `selfhost/` is a 5.4k-line go4t→Go compiler written in go4t
  that reproduces its own output (`bash selfhost/bootstrap.sh`), checked in CI.

Laws have so far pinned: operation order ("cordon before evict"), idempotence,
exact `>=`/`>` boundaries on deadlines, "a full zone is never chosen",
"a pod owned by a DaemonSet projects to `DaemonSet`", and
`decide(o) == decideV2(o)` across a refactor.

## Language at a glance

- Algebraic data types, records with immutable update, tuples, persistent
  `List`/`Map`/`Set`/`Array`.
- Hindley–Milner inference; type classes with superclasses and higher kinds;
  `deriving Eq, Ord, Show, Debug, Functor, Foldable, Traversable`, JSON
  `Encode`/`Decode`.
- Exhaustive `match` with guards, tuple and list patterns, pattern `let`.
- Explicit effects: `IO[A]`, `do` blocks, typed `IoError` recovery, fibers
  (`fork`/`await`/race/timeout) with cancellation; IO-only `Ref`, `MutMap`,
  channels.
- `std`: strings, numbers, collections, JSON, HTTP client/server, regex,
  lazy values, `State`, file and process I/O.
- Go interop both ways: `import "go:…"` binds functions, methods, fields,
  generics and upcasts of any Go package at build time; `--emit-go` exports
  go4t functions, records and sums as plain Go.
- Go-flavoured syntax: `func`, `package`, `import`, braces, `|>` pipelines.
- Tooling: `check`, `build`, `run`, `test`, `eval`, `fmt`, an LSP server and a
  VS Code extension; diagnostics point at source spans.

## Status and what go4t is not

- **Not 1.0.** The current release is
  [v0.1.0](https://github.com/reoring/go4t/releases/tag/v0.1.0); `@latest`
  follows the newest tag, `@v0.1.0` pins it. The grammar, standard library
  and Go ABI are stable enough for the ports above and are written down in
  [SPEC.md](SPEC.md), but they can still change between releases. Known
  limits: [SPEC §9](SPEC.md#9-known-limitations).
- **Not a replacement for Go.** Keep your Go program and move the parts that
  benefit; laws say nothing about I/O.
- **Not a proof assistant.** Laws are an equational checker over first-order
  data with a fixed set of tactics.
- **Not an effect-system research language.** There is one effect type, `IO`.
- **Not fast everywhere.** See *Speed* above before putting a persistent `Map`
  in a hot loop.

## Documentation

| Document | Contents |
| --- | --- |
| [Tutorial for Go developers](docs/tutorial.md) | Eighteen short lessons; every go4t snippet type-checked in CI. |
| [Language specification](SPEC.md) | Grammar, type and effect rules, deriving, exported Go API, boundary tables. |
| [Laws](docs/laws.md) | What laws cover, what they do not, how proofs are written. |
| [Go interop](docs/go-interop.md) | Three migration shapes, the boundary rule, `--emit-go`, `go:` imports and dependencies. |
| [CLI reference](docs/cli.md) | Every command, exit codes. |
| [Examples](examples/README.md) | All runnable examples with their commands. |
| [Evaluation](docs/evaluation.md) | Production ports, law rounds, the Go/`rapid` comparison, the soak, the migration record. |
| [Architecture decisions](docs/adr/README.md) | Why each rule is the way it is. |
| [Standard library](docs/stdlib.md) | Generated from the source inventory. |
| [Editor setup](editors/README.md) | stdio LSP and a VS Code client. |
| [Design history](docs/history/DESIGN.md) | Slice contracts; SPEC wins where they differ. |

## License

Apache License 2.0. See [LICENSE](LICENSE).
