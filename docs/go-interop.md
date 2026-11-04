# Go interop: embedding, bindings, projects

[日本語](go-interop.ja.md)

## Three ways to put go4t in a Go program

Choose how much to move; the same pure decision can serve all three shapes.

| Shape | Who owns the client and loop | What is go4t | Laws reach | Examples |
| --- | --- | --- | --- | --- |
| Go shell + go4t decision | Go | `decide` only | Pure decisions; not Go observation or effects | [go-shell](../examples/go-shell), [node-drain-controller](../examples/node-drain-controller), [attachment-repair-controller](../examples/attachment-repair-controller) |
| Go shell + go4t observe/decide/apply | Go; exported IO functions receive native `*kubernetes.Clientset` or controller-runtime `client.Client` handles | Observation, projection, decision and application | Pure projections and decisions; not API availability or effect success | [node-drain-shell](../examples/node-drain-shell) (kind-verified), [approval-controller](../examples/approval-controller) (real AppThrust controller; Go reconciler 271 → 88 lines, six original tests unchanged) |
| Whole controller in go4t | go4t, through `import "go:k8s.io/client-go/kubernetes"` | Client setup, observation, decision, application and entry | Pure projections and decisions; not API availability or effect success | [node-drain-typed](../examples/node-drain-typed) ([three-hour soak](../examples/node-drain-typed/soak): 839/839 cycles) |

## The boundary rule

The boundary never converts lossily: it copies all-exported, mirrorable structs
as records and preserves private or otherwise nonmirrorable state as handles.
Adding a view does not change that representation.
Lossy operations belong in named functions (`…ToJson`, `…FromJson`, `…Parse`)
or SPEC §8 rows explicitly marked lossy, such as `time.Time` → UnixNano `Int`.
This is [ADR 0011](adr/0011-lossless-boundary.md); the concrete rules follow.

| Consequence | Contract | Reference |
| --- | --- | --- |
| Handles by identity | `go:` handles cross `--emit-go` as native `*pkg.T` (struct handles) or `pkg.T` (including interfaces), also inside records and containers. | [SPEC §8.7](../SPEC.md#87-exported-go-api); [ADR 0011](adr/0011-lossless-boundary.md) (boundary invariant) |
| Cross-package upcasts | Generated conversions such as `client.unstructuredAsObject` preserve handle identity; there is no implicit subtyping. | [SPEC §8.3](../SPEC.md#83-externs-and-bind); [ADR 0009](adr/0009-cross-package-upcasts-and-lock-values.md) |
| Scalar implementer upcasts | `client.inNamespaceAsListOption(ns)` accepts a `String`; named scalar, bytes, slice and map implementers use their underlying values. | [SPEC §8.3](../SPEC.md#83-externs-and-bind); [ADR 0010](adr/0010-scalar-implementer-upcasts.md) |
| In-out pointers | Direct `*T` arguments with value mirrors use non-null `Ref[T]` and IO, with writeback even on error; results and fields use `Option[T]`. Pointer-to-handle identity is unchanged. | [SPEC §8.3](../SPEC.md#83-externs-and-bind); [ADR 0011](adr/0011-lossless-boundary.md) |
| Go errors | Parameters use `GoError`; nullable positions use `Option[GoError]`. Trailing errors fail IO, retaining the cause through `io.tryIOCause` / `io.cause` for pure predicates such as `apierrors.isNotFound(cause)`. | [SPEC §8.3](../SPEC.md#83-externs-and-bind); [ADR 0011](adr/0011-lossless-boundary.md) |
| Explicit JSON views | JSON-capable types gain `<t>ToJson` / `<t>FromJson` without changing their ordinary mirror; `metav1.Time` remains a handle, not `dyn.Value`. | [SPEC §8.3](../SPEC.md#83-externs-and-bind); [ADR 0011](adr/0011-lossless-boundary.md) |
| Records containing handles | No derived `Eq` or `Debug`, including handles or `GoError` nested through records, containers or `Option`. | [SPEC §8.3](../SPEC.md#83-externs-and-bind); [ADR 0011](adr/0011-lossless-boundary.md) |
| Export-name collisions | A function colliding with a type or constructor mirror gets `Fn` (`nodeState` → `NodeStateFn`); the type keeps its name. | [SPEC §8.7](../SPEC.md#87-exported-go-api) |

## Embedding in a Go program

1. Write the entry module in `decide/decide.gt`: for a pure core, define a
   projection record `Observed`, an action sum and `func decide(o Observed) Action`.
2. Generate: `go4t build --emit-go decide_gen --package decide decide/decide.gt`
   (put it in a `//go:generate` line). The package exports `Decide`, reachable
   Go mirror types (`Observed`, action constructors), and `Main() rt.IO`.
3. Import the generated package from Go, fill `Observed`, call `Decide` and
   perform the selected effects. For the second shape, call exported
   observe/apply functions with the Go-owned handles; an `IO[T]` result exports
   as `func(context.Context) (T, error)`. The compiler keeps `decide` pure.

Rules that keep the boundary clean (SPEC §8.7, tutorial §12):

- Project only what the decision needs.
- Never cross a sentinel. `Option[Int]` crosses as a pointer-free mirror;
  compare with `Some(x)`, do not unwrap with `-1`.
- Project durations and timestamps explicitly as `Int` nanoseconds when that
  is what the decision needs; a timestamp projection discards location and
  monotonic metadata, unlike transporting a native handle.
- A validity flag beside a list becomes a sum per element; "first failure in
  order" then matches the original Go loop exactly.

## Calling Go from go4t

Import a Go package directly; bindings are generated at build time and never
committed:

```go4t
package main

import "std/io"
import "std/option"
import "go:net/url"

func main() IO[Unit] {
  do {
    parsed <- url.parse("https://go4t.example/guide")
    match parsed {
      None => io.println("no URL")
      Some(value) => io.println(url.urlHostname(value))
    }
  }
}
```

Other binding rules ([ADR 0003](adr/0003-import-time-go-bindings.md),
[SPEC §8.3](../SPEC.md#83-externs-and-bind)):

- Functions, methods, package variables and basic constants become externs;
  exported fields of opaque structs get `<type><Field>` getters and
  `<type>Set<Field>` setters; opaque structs get `new<Type>()`.
- Anything that receives a mutable handle (method receiver, parameter, field
  getter) is `IO`: a handle is shared Go state. `GoError` is the exception.
  By-value records, maps and slices stay pure unless they carry shared state.
- For in-out cells, import `std/ref`, allocate with `ref.new(value)`, pass the
  cell to Go and read back with `ref.get(cell)`. `...*T` uses `List[Ref[T]]`.
  Reads and writes each lock the cell; the entire foreign call is not atomic.
  Native callbacks cannot pass nil for an in-out pointer parameter.
- Use `io.goErrorMessage` for an explicit error message projection.
  Direct/callback error parameters and `...error` elements cannot represent nil;
  native callbacks passing nil panic with `go4t error: nil error parameter`.
- `(A, B)` results are tuples, `(A, B, error)` is `IO[(A, B)]`, `(A, bool)` is
  `Option[A]`; `iter.Seq[T]` results are `List[T]`; returned funcs are go4t
  functions. Still unsupported: iterator parameters, generic receiver methods,
  generic named structs/interfaces, unsupported channel elements, and more than
  three non-error results.
- Every Go type is declared once, in the module of the package that owns it;
  `net/http` bindings refer to `io.Reader` as `go_io.Reader` and the loader
  binds `go:io` transitively. Upcasts include `strings.readerAsReader` and
  `goio.readCloserAsReader`. In a binding module P, the source type and interface
  must belong to P or its direct imports, with at least one declared in P.
  Foreign source homonyms gain an owner qualifier (`qImplAsI`); P's own type
  keeps the unsuffixed name (`implAsI`).
- The standard library binds without a `go.mod`; third-party packages need the
  build module to require them. Shims are written beneath the ignored build
  output (`*_gen/`); `go4t check` writes nothing.
- Struct-handle package variables, fields and method receivers use addresses
  rather than copying, whether or not they contain locks. True by-value
  parameters, results, container elements and field setters containing locks
  are skipped with `copies lock value T`; `*T` remains a handle. Detection follows
  Go vet's pointer-only `sync.Locker` implementation rule recursively through
  fields, arrays and embedding; embedding the `sync.Locker` interface alone is
  not a lock.
- `<t>ToJson(recv T) dyn.Value` is pure; a failed Marshal panics with
  `go4t json view: <type>: <err>`. `<t>FromJson(v dyn.Value) IO[T]` checks
  cancellation and preserves conversion errors as Go causes.
  Existing functions/methods, accessors, constructors/value getters and upcasts
  win name collisions; a skipped view receives the normal collision comment.
  JSON round trips may normalize locations or otherwise lose information;
  ordinary record transport never takes that route.

The runnable example is [examples/go-import](../examples/go-import).

Go channels bind as `Chan[T]`, `RecvChan[T]`, or `SendChan[T]`. Import `std/chan`
for cancellation-aware send and receive operations and deferred close; receive
returns `None` after closure. Use `asRecv` and `asSend` to narrow a bidirectional
handle. `make` creates channels for go4t fibers; use a generated constructor such
as `signal.newSignalChan` when passing a channel to a Go API. Constructors
allocate inside IO, and channel elements use the same conversions as ordinary
arguments and results. See [examples/go-chan](../examples/go-chan) for `time.after`
and producer/consumer fibers. Unsupported channel elements remain skipped.

Generic Go functions bind as templates and instantiate from concrete call-site
types after checking; no explicit Go type arguments are written. Int, Float,
Bool, and String select `int64`, `float64`, `bool`, and `string`; List and Map
compose those mappings, and known foreign records/handles retain their Go
identity. Go constraints are checked, not replaced by go4t Eq/Ord. Every generic
call and its callbacks use IO, including returned closures such as
`sync.onceValue`. Unresolved arguments, unsupported mirrors, and failed
constraints are errors at the use site. Top-level polymorphic helpers receive
required concrete copies; generalized local lambdas need concrete annotations
when a foreign argument otherwise remains rigid. Iterator results still become
Lists (`maps.keys`); iterator inputs (`slices.sorted`) remain unsupported.
In-place operations mutate boundary copies: `slices.sort` returns `IO[Unit]`
and does not update the caller's List. Package templates are cached separately
from per-tuple shim companions, so changing callers does not invalidate the
package cache. See [examples/go-generics](../examples/go-generics) and
[ADR 0006](adr/0006-generic-go-functions.md).
Context, error, and optional-result conventions follow the original Go
declaration, not the substituted type arguments.


`go4t bind` still prints or writes the same declarations for inspection:

```sh
go4t bind strings --prefix str          # prints extern declarations
go4t bind modernc.org/sqlite -o bindings.gt --shim shimdir
```

For example, inspect `path/filepath` with `go4t bind path/filepath` before
calling an unfamiliar generated operation. Without `--prefix`, function names
lower-case the first Go letter (`Base` becomes `base`), so
`import "go:path/filepath"` exposes `filepath.base(path)`.
[SPEC §8.3](../SPEC.md#83-externs-and-bind) defines the naming rules, including
receiver-method names; [CLI reference](cli.md) lists the inspection flags.

Signatures that cannot be mirrored get a skip comment instead of a wrong
binding. [examples/sql-service](../examples/sql-service) uses `go:` imports for
SQLite and Prometheus integration; [examples/typed-extern](../examples/typed-extern)
executes through `go:net/url` and keeps regenerated bindings as inspection output.
`std/http` needs no bindings.

## Projects and dependencies

A go4t library is a Go module with `go.mod`, `go4t.mod` and `.gt` files.

```sh
go4t init example.com/myapp            # creates go.mod + go4t.mod
go4t get example.com/lib@v1.0.0        # go get, then record in go4t.mod
```

Go selects versions and keeps checksums; `go4t.mod` lists language imports.
`go4t get` only updates dependencies; `go:` imports generate bindings on demand,
without committed binding artifacts.
Imports are file-local and qualified (`import "std/io"` → `io.readFile`); only
the small core prelude is implicit. Resolution order: the importing module's
source root, `go4t.mod` requires, `GO4T_PATH`, embedded std. See
[examples/deps](../examples/deps) for an offline local-replacement setup.
