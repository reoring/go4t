# Go interop: embedding, bindings, projects

[日本語](go-interop.ja.md)

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
- `go:` handles cross as native Go values with identity (`*pkg.T` for opaque
  structs, `pkg.T` for interfaces), not copied records or enums. A Go shell can
  own the `*kubernetes.Clientset` and pass it to exported go4t `observe` / `apply`
  IO functions, or receive the same handle back.
  [node-drain-shell](../examples/node-drain-shell) demonstrates this with a
  Go-owned clientset and reconcile loop calling go4t IO against a kind cluster.

Runnable template: [examples/go-shell](../examples/go-shell). Real ports:
[examples/approval-controller](../examples/approval-controller),
[examples/attachment-repair-controller](../examples/attachment-repair-controller).

## Calling Go from go4t

Import a Go package directly; bindings are generated at build time and never
committed:

```go4t
import "go:strings"
import gohttp "go:net/http"

let (scheme, rest, found) = strings.cut(url, "://")
request <- gohttp.newRequest("GET", url, strings.newReader("") |> strings.readerAsReader)
headers <- gohttp.requestHeader(request)
gohttp.requestSetHeader(request, map.mapInsert(headers, "X-Who", ["Ada"]))
response <- gohttp.clientDo(gohttp.defaultClient(), request)
```

Rules (ADR 0003, ADR 0009 and ADR 0011 §2C/§3C, SPEC §8):

- Functions, methods, package variables and basic constants become externs;
  exported fields of opaque structs get `<type><Field>` getters and
  `<type>Set<Field>` setters; opaque structs get `new<Type>()`.
- Anything that receives a mutable handle (method receiver, parameter, field
  getter) is `IO`: a handle is shared Go state. Error handles are the exception
  below. By-value records, maps and slices cross as copies and stay pure unless
  they carry shared state.
- Direct `*T` parameters with scalar, record, collection or bytes payloads use
  `Ref[T]` and force `IO`, even for read-only callees. Import `std/ref`, allocate
  a cell with `ref.new(value)`, pass it to Go, then read back with `ref.get(cell)`.
  The shim copies the cell's value into Go and writes the changed value back
  before returning, including on Go errors. `...*T` uses `List[Ref[T]]`.
  Reads and writes each lock the cell; the entire foreign call is not atomic.
  Results and record fields remain `Option[T]`; pointer-to-handle identity and
  JSON-scalar/unresolved type-parameter pointer rules are unchanged. A Ref is
  non-null: native callbacks cannot pass nil for an in-out pointer parameter.
- Go `error` parameters become the builtin `GoError`, preserving identity;
  predicates such as `apierrors.isNotFound(cause)` stay pure. Non-trailing
  results, fields, package variables and container elements use
  `Option[GoError]` (nil ↔ None); trailing `error` still takes the IO failure
  path. Use `io.tryIOCause` and `io.cause` to recover a cause, pass it to Go
  predicates, or explicitly read its message with `io.goErrorMessage`.
  Direct/callback parameters and `...error` elements cannot represent nil;
  native callbacks passing nil panic with `go4t error: nil error parameter`.
  Error-bearing records, including nested ones, omit `deriving Eq, Debug`
  because `GoError` has no such instances.
- `(A, B)` results are tuples, `(A, B, error)` is `IO[(A, B)]`, `(A, bool)` is
  `Option[A]`; `iter.Seq[T]` results are `List[T]`; returned funcs are go4t
  functions. Still unsupported: iterator parameters, generic receiver methods,
  generic named structs/interfaces, unsupported channel elements, and more than
  three non-error results.
- Every Go type is declared once, in the module of the package that owns it;
  `net/http` bindings refer to `io.Reader` as `go_io.Reader` and the loader
  binds `go:io` transitively. There is no subtyping: convert with the generated
  upcasts (`strings.readerAsReader`, `goio.readCloserAsReader`).
  In a binding module P, the source type and interface must belong to P or its
  direct imports, with at least one declared in P: for example,
  `client.unstructuredAsObject`. Foreign source homonyms gain an owner qualifier
  (`qImplAsI`); P's own type keeps the unsuffixed name (`implAsI`).
  Named scalar, bytes, slice and map implementers accept their underlying go4t values in pure upcasts, such as `client.inNamespaceAsListOption(ns)` for a `String` namespace.
- The standard library binds without a `go.mod`; third-party packages need the
  build module to require them. Shims are written beneath the ignored build
  output (`*_gen/`); `go4t check` writes nothing.
- Each call copies records across the boundary; handles keep identity.
  Struct-handle package variables, fields and method receivers use addresses
  rather than copying, whether or not they contain locks. True by-value
  parameters, results, container elements and field setters containing locks
  are skipped with `copies lock value T`; `*T` remains a handle. Detection follows
  Go vet's pointer-only `sync.Locker` implementation rule recursively through
  fields, arrays and embedding; embedding the `sync.Locker` interface alone is
  not a lock.

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
shows the ABI adapters. `std/http` needs no bindings.

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
