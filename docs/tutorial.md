# go4t for Go developers

With Go 1.22 or newer on PATH, `go install github.com/reoring/go4t/cmd/go4t@latest`
installs the CLI; add Go's bin directory to PATH. The installed binary embeds its
runtime, so saved programs can live anywhere: `run`, `test`, and `eval` create a
temporary Go module when needed and clean it afterward. Keep the Go toolchain on
PATH. From a repository checkout, `go install ./cmd/go4t` or `go run ./cmd/go4t`
work the same way. Externs outside `github.com/reoring/go4t/rt` and its
subpackages need an application Go module providing those packages and
`github.com/reoring/go4t`. `go4t init` creates that project; `go4t get` records
language dependencies in `go4t.mod` while Go manages their versions and checksums.

Every `go4t` block below is a complete checkable example, alone or with the files
in its named group. For ordinary blocks, save as `lesson.gt` and run
`go4t check lesson.gt` then `go4t run lesson.gt`. Use separate directories or
explicit files for independent examples so their main functions do not collide.

The `// file: ...` / `// dir: ...` comments are documentation-test directives and
ordinary language comments: create those files relative to the named directory.
Blocks with the same `dir` form **one** module example. The test checks all their
files together, including imported subdirectories; it does not invent a main or
imports. Full details are in [SPEC](../SPEC.md) and [stdlib](stdlib.md).

## 1. Immutable values, not mutable Go variables

`let` binds once; a block returns its last expression. Records are data
constructors, not mutable structs. Updates produce a new value. `alias` is a
transparent synonym, not a distinct type. Both constructors below expose the
same `id` field with the same type, so access/update works without a match.
When an annotation uses an alias, diagnostics and hover keep its name. Unrelated
inferred types are not renamed: `Identifier` remains semantically `Int`, but a
mismatch against an `Identifier` annotation says `expected Identifier`.

```go4t
package main
alias Identifier = Int
type Shape = Circle{id: Identifier, radius: Float} | Rectangle{width: Float, id: Int} deriving Debug
func rename(shape Shape, id Int) Shape { shape{id: id} }
func main() IO[Unit] {
  let original = Circle{id: 7, radius: 2.5}
  let changed = rename(original, 9)
  println("${original.id} -> ${changed.id}: ${debug(changed)}")
}
```

Int and Float do not coerce: write `toFloat(2) + 0.5`, not `2 + 0.5`.
All fields must exist in every constructor before common-field selection is legal.
There is no `nil`; represent absence as `Option[A] = None | Some(A)`.

## 2. Type classes versus Go interfaces

A Go interface is a type describing methods on a value. A go4t class describes
operations parameterised by a type; an **explicit instance** supplies its
implementation. A constrained generic function receives that implementation as
a compiler-managed dictionary. You can define instances for existing types;
loaded modules must agree on one instance per class/type constructor.

```go4t
package main
class Label[A] { label(a A) String }
type User = User{name: String} deriving Eq, Debug
instance Label[User] { label(user) { user.name } }
instance Show[User] { show(user) { "user:" + user.name } }
func describe[A Label + Show](value A) String { label(value) + " / " + show(value) }
func main() IO[Unit] { println(describe(User{name: "Ada"})) }
```

Unlike Go's implicit method-set satisfaction, a record does not acquire Label by
having a `name` field. `A Label + Show` requires both classes. Ord inherits Eq,
so `A Ord` is enough for equality and ordering. Show is intentional display;
Debug is structural. **`deriving Show` supports enums and single-field newtypes**:
enums render their source constructor names, and newtypes delegate to the field's
Show without requiring Debug. Write an explicit Show instance for other data
types or a different display form.

Higher-kinded classes operate on type constructors. `Functor[F[_]]` means that
F takes a type argument (Option, List, Array, or a partially applied Result).
Derivation maps the last parameter; Traversable needs Functor as its superclass.

```go4t
package main
import p "std/prelude"
type Tree[A] = Leaf(A) | Branch(Tree[A], Tree[A]) deriving Debug, Functor, Foldable, Traversable
func main() IO[Unit] {
  let tree = Branch(Leaf(2), Leaf(3))
  let larger = map(tree, (_ + 10))
  let total = p.foldr(larger, 0, func(value, rest) => value + rest)
  println("${debug(larger)}; total=${total}")
}
```

`map` is implicit; `p.foldr`, `p.fmap`, `p.pure`, `p.bind` and `p.traverse` need
an explicit prelude import. A class being implicit does not make every method
implicit.

## 3. IO descriptions versus immediate side effects

In Go, calling `fmt.Println` writes immediately. In go4t, `println` returns an
`IO[Unit]` description. Merely creating or discarding the description does not
run it. Main returns the action the runtime executes.

```go4t
package main
func greeting(name String) IO[Unit] { println("Hello, ${name}!") }
func main() IO[Unit] {
  let _notRun = greeting("discarded")
  greeting("Ada")
}
```

Only `Hello, Ada!` is printed. Values are still strict: `ioPure(1 / 0)` divides
before it can return IO. To defer a pure computation until an action executes,
put it after a bind in `do`. A trusted Go extern must honor its declared IO/purity
contract; the checker cannot infer side effects from Go function bodies.

## 4. Do notation is explicit sequencing

A `<-` binding extracts the result of an action. A bare action runs and discards
its result. A `let` computes a pure local value. The last expression must check as
an action of the **same Monad**; a local function is a statement, not an action.

```go4t
package main
import "std/io"
func main() IO[Unit] {
  do {
    millis <- io.nowMillis()
    let stamp = io.formatTime(millis, "2006-01-02")
    func message(day String) String { "UTC date: " + day }
    println(message(stamp))
    ioPure(unit())
  }
}
```

There is no reassignment here. The compiler lowers `do` to nested binds. The same
notation also works for Option, Result, List and `state.State[S]`: it is not a
special imperative sublanguage for IO. Pattern binds must be irrefutable; match
an Option/Result explicitly or use its Monad rather than writing `Some(x) <- ...`.

### Returning a pure value from IO

When an expected type is already `IO[T]`, a pure `T` in a function tail,
`if`/`match` arm, `do` final expression or annotated let is treated as
`ioPure(value)`. This includes `unit()`; IO branches can mix actions and values:

```go4t
package main
func label(found Bool) IO[String] {
  if found { "ready" } else { do { println("waiting"); "pending" } }
}
func main() IO[Unit] {
  let answer: IO[Int] = 42
  do { n <- answer; text <- label(n == 42); println(text); unit() }
}
```

Arguments, operands and list elements do not lift. Keep explicit `ioPure` when
the value's type is still a type variable. Existing IO values are never wrapped
again or flattened, and other Monads keep their existing rules. This introduces
no `async` syntax and does not permit `<-` outside `do`; lifting remains strict,
just like calling `ioPure` yourself.

## 5. Result replaces repeated error-tuple branching

`Result[E, A]` distinguishes a typed error from a success. Compared with Go's
`(T, error)` convention, the constructor records which case exists; the Monad
passes on the first Err without calling later continuations.

```go4t
package main
import "std/string"
import "std/result"
func positive(text String) Result[String, Int] {
  do {
    number <- result.okOr(string.parseInt(text), "expected an integer")
    if number > 0 { Ok(number) } else { Err("expected positive") }
  }
}
func addInputs(a String, b String) Result[String, Int] {
  do {
    x <- positive(a)
    y <- positive(b)
    Ok(x + y)
  }
}
func main() IO[Unit] {
  println("${addInputs("7", "11")} / ${addInputs("bad", "also bad")}")
}
```

This displays `Ok(18) / Err(expected an integer)`. `result.mapErr` changes only
errors, `result.toOption` deliberately discards them, and `result.liftResult`
turns a displayed Err into a failed IO. Defaults supplied to `fromResult` and
`fromOption` are ordinary strict arguments, not lazy fallbacks.

### Recovering an IO error or action panic with tryIO

`io.tryIO` returns `IO[Result[io.IoError, A]]`, even if the wrapped action fails
or panics. Match typed constructors such as `io.NotFound(message)`,
`io.PermissionDenied(message)` and `io.Panic(message)` rather than inspecting
error strings. `io.ioErrorMessage(e)` or `show(e)` gives the message; `debug(e)`
includes the constructor. It does not catch an error that happened before the
action was constructed, nor explicit process exit.

```go4t
package main
import "std/io"
func divideLater() IO[Int] {
  do {
    zero <- ioPure(0)
    ioPure(10 / zero)
  }
}
func main() IO[Unit] {
  do {
    failed <- io.tryIO(ioFail("service unavailable"))
    let typed: Result[io.IoError, Int] = failed
    panicked <- io.tryIO(divideLater())
    println("${typed}; ${panicked}")
  }
}
```

The output remains `Err(service unavailable); Err(runtime error: integer divide by zero)`.
The errors themselves are `io.Failure("service unavailable")` and
`io.Panic("runtime error: integer divide by zero")`. `io.ioRaise(e)` preserves an
IoError constructor when rethrowing; `ioFail` and `io.ioFailWith` raise Failure.
Timeout and Cancelled have no payload and display `context deadline exceeded`
and `context canceled`, respectively.

Unhandled ordinary IO failure prints `error: ...`, exit 1; a direct panic
**during action execution** prints `error: panic: ...`, exit 2. Panics during
action construction are outside RunMain recovery. A worker panic from concurrency
is instead a typed Panic IO failure, with no added `panic:` message prefix.
Installed `go4t run` preserves child exit status; the outer Go launcher does not
preserve arbitrary nonzero statuses.

For example, missing-file recovery can distinguish a missing configuration from
other failures:

```go4t
package main
import "std/io"
func main() IO[Unit] {
  do {
    loaded <- io.tryIO(io.readFile("/nonexistent"))
    match loaded {
      Err(io.NotFound(message)) => println("missing configuration: " + message)
      | Err(e) => io.ioRaise(e)
      | Ok(text) => println(text)
    }
  }
}
```

## 6. Modules are directories; imports are file-local

Create `module-demo/main.gt` and `module-demo/greeting/greeting.gt` from the
following two blocks, then run `go4t run module-demo`. Without `go4t.mod`, the
entry directory is the import root. The greeting module loads every immediate
`.gt` file in its directory, with one shared package spelling.

```go4t
// dir: module-demo
// file: main.gt
package main
import hello "greeting"
func main() IO[Unit] { println(hello.message(hello.Person{name: "Ada"})) }
```

```go4t
// dir: module-demo
// file: greeting/greeting.gt
package greeting
type Person = Person{name: String}
func _prefix() String { "Hello, " }
func message(person Person) String { _prefix() + person.name + "!" }
```

Every top-level name is public unless prefixed `_`; `hello._prefix()` is private,
although sibling files inside greeting can use it. Unlike Go export casing,
ordinary lower-case function names **are public**. Imports are qualified and
apply only to their file. Declarations across files in the same module are
mutually visible, and cyclic module graphs are supported.

For a stable project root, put the line `module demo` in `module-demo/go4t.mod`;
then `"demo/greeting"` also resolves to the greeting directory. Resolution tries
the nearest module root, matching `go4t.mod` requires, GO4T_PATH entries, then
embedded std. Local declarations may shadow implicit prelude names with a
warning; explicit module qualification removes ambiguity.

### Adding a library dependency

In a new directory, `go4t init example.com/myapp` creates `go.mod` and
`go4t.mod`, refusing to overwrite either. The Go module uses `go 1.22`,
`require github.com/reoring/go4t v0.0.0` and a local runtime replacement. It first tries
`go list -m -json go4t` from the current directory and uses its resolved `Dir`.
If that fails, it extracts the embedded runtime to
`$XDG_CACHE_HOME/go4t/runtime/<sha256>` on Linux (default
`$HOME/.cache/go4t/runtime/<sha256>`). This content-addressed cache is reused;
no project-local `.go4t` directory is created. The replacement is machine-local,
so keep the referenced checkout/cache or adjust the replacement when relocating.

For a published library, run `go4t get example.com/lib@v1.0.0`.
For a local library at `../lib`, first put
`replace example.com/lib => ../lib` in **go.mod** and run the same command;
Go accepts that replacement offline, without a published version.
The command runs `go get` at the nearest go4t project root and adds or updates
one `require example.com/lib v1.0.0` line in `go4t.mod`. `@latest` and other
Go version queries are recorded as the selected version. Go owns `go.sum`
checksums and all replacements; `go4t.mod` supports module/require lines,
blank lines and `//` comments, not `replace` directives.

Import `"example.com/lib/shapes"` to load the dependency's `shapes/` sources.
`go list -m -json example.com/lib` supplies its directory using the entry
project's Go graph, so its replacements also apply to transitive dependencies.
A dependency's own imports use its own go4t root and requires. Missing Go
dependencies are errors, not silent fallbacks to GO4T_PATH.

Try the complete offline [dependency example](../examples/deps):

```sh
go4t run examples/deps/app
# area: 42
```

## 7. Pipelines, sections and recursive helpers

`x |> f(a)` means `f(x, a)`. A call with `_` instead becomes a lambda at those
argument positions, so a pipeline can select a non-first argument. Local function
groups can recurse mutually; a bare-name lambda let can recurse into itself.

```go4t
package main
import "std/list"
import "std/string"
func main() IO[Unit] {
  let sumTo = func(n Int, acc Int) => if n <= 0 { acc } else { sumTo(n - 1, acc + n) }
  let values = list.range(1, 5) |> map((_ * 2)) |> map(show)
  println(string.join(", ", values) + "; sum=" + show(sumTo(5, 0)))
}
```

A section is not automatic currying: `(_ < _)` takes two arguments. Holes must
be whole arguments or parenthesised whole operator operands. Use `map((_ + 1))`
in a pipe, not `map(_ + 1)`. Direct self-tail recursion is loop-lowered, but mutual
recursion, non-tail calls and do-continuation calls do not get that guarantee.

An expected function type also supplies unannotated lambda parameters and
projection receivers. `Some(_.product)` can use a declared return type; a bare
`let key = Some(_.product)` cannot infer the record and reports
`cannot infer record type of _.product; annotate`. The expectation passes through
`if` branches, `match` arms, record fields and the final expression of `do`, not
through arbitrary expressions or later uses.

```go4t
package main
type Sale = Sale{product: String, qty: Int}
func productKey() Option[func(Sale) String] { Some(_.product) }
func quantityKey() func(Sale) Int {
  match true { true => _.qty; false => func(s) => s.qty }
}
func main() IO[Unit] {
  let sale = Sale{product: "tea", qty: 2}
  let key: func(Sale) String = func(s) => s.product
  let project: func(Sale) String = _.product
  println(key(sale) + ": " + project(sale) + " x " + show(quantityKey()(sale)))
}
```

## 8. Testing with go4t test

Save this as `lesson-tests.gt` and run `go4t test lesson-tests.gt`. It also
passes `go4t check` because it includes a normal main; **test does not require
main**, and replaces/removes it before checking. Test names must start `test`
with a nonempty suffix, have no parameters/type parameters, and explicitly return
Bool or IO[Bool]. There is no assertion DSL: return the Boolean contract.

```go4t
package main
import "std/io"
func testArithmetic() Bool { 2 + 3 == 5 }
func testRecovery() IO[Bool] {
  do {
    recovered <- io.tryIO(ioFail("expected"))
    let typed: Result[io.IoError, Int] = recovered
    ioPure(typed == Err(io.Failure("expected")))
  }
}
func main() IO[Unit] { println("Run this file with go4t test.") }
```

The harness reports `PASS testArithmetic`, `PASS testRecovery`, then
`2 passed, 0 failed`. False returns and failed IO report FAIL and later tests
still run; any failed test gives exit 1. An empty suite succeeds. A panic is not
a failed-assertion mechanism: it follows ordinary runtime panic policy.

For one-off exploration, run `go4t eval '1 + 2'` or
`go4t eval 'testArithmetic()' lesson-tests.gt`. Eval uses Show on the result;
Unit has no Show, so an `IO[Unit]` expression is not a displayable eval result.

## 9. FFI: import "go:…"

Save this as `ffi-demo/main.gt`, then run `go4t check ffi-demo` and
`go4t run ffi-demo`. Import a Go package directly; no generated source needs
to be copied into your project:

```go4t
// dir: ffi-demo
// file: main.gt
package main
import "go:strings"
func main() IO[Unit] { println(strings.toUpper(strings.repeat("go", 3))) }
```

This prints `GOGOGO`. The generator adapts Go `string` directly and Go `int`
to go4t's canonical int64. Other supported natural types include float64, bool,
rt.Unit, rt.IO, []string, []int64 and primitive `(T, bool)` Option results.
A Go `(T, error)` return maps to `IO[T]`; the adapter defers the Go call and its
conversions until execution. No Go wrapper is needed for these signatures.

Names are qualified by the imported module: Go `ToUpper` becomes
`strings.toUpper`. Methods use `<type><Method>` with the receiver first,
such as `strings.builderWriteString(builder, text)`. Exported fields on handles
get getters such as `url.urlHost(value)` for `net/url.URL.Host`; zero-value
constructors use `new<Type>()`, such as `strings.newBuilder()`. Existing Go
constructors keep their arguments (`strings.newReader(text)`).

Bindings and any required Go shims are generated at build time (also when
loading for `check`) and cached under `os.UserCacheDir()/go4t/bind/<key>/`.
See the [CLI cache reference](cli.md) for invalidation and `GO4T_CACHE` controls.
`go4t bind strings` prints the same declarations for inspection or legacy use;
`go:` is the normal FFI path.

For a `go:` import, the compiler records unsupported signatures and name
collisions as `// skipped:` lines in the cached `bindings.gt`. Inspect those
reasons rather than assuming that every Go function is available; the set also
varies with toolchain version. `check` alone does not validate the Go ABI—
`build`/`run` do. A pure extern should really be pure; signatures are a trust
boundary, not a side-effect analyzer.

## 10. Concurrency without mutable shared state

Go goroutines are exposed as IO actions and Fiber values. `fork` starts one task
when executed; `await` joins it. The result is cached, so later awaits do not
repeat its effects. Parallel collection results preserve input order.

```go4t
package main
import "std/conc"
func square(n Int) IO[Int] { ioPure(n * n) }
func main() IO[Unit] {
  do {
    fiber <- conc.fork(square(7))
    first <- conc.await(fiber)
    again <- conc.await(fiber)
    squares <- conc.parMapIO([3, 1, 2], square)
    pair <- conc.parBoth(ioPure(4), ioPure("ready"))
    skipped <- conc.timeout(0, ioPure(99))
    println("${first}/${again} ${squares} ${pair} ${skipped}")
  }
}
```

`parMapIO` cancels siblings on the first observed failure and joins them;
`parBoth` joins both without cancelling a sibling merely because the other failed.
An extern that ignores cancellation can delay either join. `race` is
first-completion, not first-success: it cancels and joins the loser.
Timeout returns None on its own expiry, not on timely failure;
`conc.withTimeout(millis, action)` instead fails on expiry. Both cancel expired
actions and wait until the deadline plus a 100 ms cleanup grace. Race gives its
loser the same 100 ms grace. After that, uncooperative foreign code is abandoned
and may still perform effects; a cancelled sleep never proceeds to its next bind.

`conc.cancel(fiber)` requests cancellation immediately. Awaiting an unfinished,
cancelled fiber fails, while completed outcomes remain cached. Parent cancellation
reaches child fibers, and SIGINT/SIGTERM cancels main with exit status 1 and
`error: cancelled`. Unawaited fibers do not keep the process alive after main
finishes; use explicit awaits when effects must finish.

### Serving and calling HTTP

`std/http` uses ordinary records and IO rather than application-specific externs.
This complete server routes by method/path and encodes a record using `std/json`:

```go4t
package main
import "std/http"
import "std/json"
type Health = Health{ok: Bool} deriving json.Encode
func handle(r http.Request) IO[http.Response] {
  ioPure(if r.method == "GET" && r.path == "/health" {
    http.jsonResponse(200, Health(true))
  } else {
    http.response(404, "not found")
  })
}
func main() IO[Unit] { http.serve(8080, handle) }
```

Run it and request `http://127.0.0.1:8080/health`; stop it with SIGINT.
`http.get(url)` and `http.post(url, contentType, body)` return `IO[http.Response]`,
including non-2xx statuses. They have a 30 s overall timeout;
`http.requestWithTimeout(request, millis)` sets another budget. `io.tryIO`
distinguishes `io.Network(message)`, `io.Timeout`, and `io.Cancelled`.
Headers are `List[(String, String)]`; `http.requestHeader`/`http.responseHeader`
look up the first matching name case-insensitively.

Each handler gets its request's cancellation context in its own goroutine.
Failure or panic returns 500 with the message body and logs one stderr line.
Cancellation stops the server with up to 5 s of graceful shutdown. When running
`serve` on a fiber, `conc.cancel` requests that shutdown without waiting.
The [self-contained HTTP example](../examples/http) probes readiness after
forking, calls its own `/sum?a=1&b=2` endpoint, decodes the JSON, prints `sum: 3`,
and cancels the server; it requires only loopback networking, not Internet access.

## 11. JSON: derive a schema, handle decoding explicitly

JSON lives in `std/json`, not the implicit prelude. Derive qualified Encode and
Decode on a record and use a typed Result to determine the decoded target.

```go4t
package main
import codec "std/json"
type Person = Person{name: String, age: Int, note: Option[String]} deriving Eq, Debug, codec.Encode, codec.Decode
func loadPerson(text String) Result[String, Person] {
  do {
    document <- codec.parseJson(text)
    codec.decode(document)
  }
}
func main() IO[Unit] {
  let person = Person{name: "Ada", age: 36, note: None}
  let text = codec.toJson(codec.encode(person))
  let roundTrip = loadPerson(text)
  let bad = loadPerson("{\"name\":\"Ada\",\"age\":\"old\",\"note\":null}")
  do {
    println(text)
    println(debug(roundTrip))
    println(debug(bad))
  }
}
```

The record encodes in declaration order, without a tag because it has one record
constructor. The invalid age reports `expected Int at $.age`. All record fields
are required, even Option fields (write null); extra fields are ignored. Sums use
`$tag`; positional constructors also use an exact-length `$values` array.

Json numbers are exact `JInt(Int)` or `JNum(Float)`. Int values round-trip exactly
across the full int64 range, including maxInt. Int decoding also accepts integral
JNum values within inclusive [-2^53, 2^53]; Float decoding accepts JInt through
conversion, which may round large integers. NaN/infinity cannot be printed.
None encodes as null, and Some of a null-like value is indistinguishable from
None on decode. Use an explicit tagged ADT when that distinction matters.

## 12. Go shell, go4t core: projecting observed state

`go4t build --emit-go DIR --package NAME` exports every public, monomorphic,
mirrorable top-level function as a typed Go function (SPEC §8.7). The intended
shape is: Go reads the world, fills one **projection** record, calls a pure
go4t `decide`, and acts on the returned sum; see [go-shell](../examples/go-shell).
The client and reconcile loop can also stay in Go while `observe` and `apply`
move to go4t, as in [node-drain-shell](../examples/node-drain-shell) and
[approval-controller](../examples/approval-controller), whose original tests
pass unchanged.

Three rules decide whether the projection is readable. Each one is the same
correction: a state Go expresses with `(value, ok)`, a sentinel or a flag
becomes a constructor with a name.

| Go habit in the projection | Replace with | Why |
| --- | --- | --- |
| `valid: Bool` beside a list | a sum per element: `GateEvidence = Gate{…} \| InvalidGate` | keeps the position of the invalid element, so "first failure in order" matches a Go loop |
| `Option[Int]` plus "≤ 0 means invalid" | three constructors: `Ttl = NoTtl \| InvalidTtl \| TtlNanos(Int)` | the invalid case cannot be forgotten by a match |
| `present: Bool` plus `Option[Int]` | three constructors: `Timestamp = NoTimestamp \| InvalidTimestamp \| AtNanos(Int)` | one match instead of nested if/match |

With both sums, a four-step chain of `if !ok { return … }` becomes one match
whose arms read in the original order and are checked for exhaustiveness:

```go4t
package main
type Ttl = NoTtl | InvalidTtl | TtlNanos(Int) deriving Eq, Debug
type Timestamp = NoTimestamp | InvalidTimestamp | AtNanos(Int) deriving Eq, Debug
type Freshness = Fresh(Int) | TtlInvalid | TimestampMissing | TimestampInvalid | Stale deriving Eq, Debug
func freshness(ttl Ttl, observedAt Timestamp, nowNanos Int) Freshness {
  match (ttl, observedAt) {
    (NoTtl, _) => Fresh(0)
    (InvalidTtl, _) => TtlInvalid
    (TtlNanos(_), NoTimestamp) => TimestampMissing
    (TtlNanos(_), InvalidTimestamp) => TimestampInvalid
    (TtlNanos(t), AtNanos(at)) if nowNanos >= at + t => Stale
    (TtlNanos(t), AtNanos(at)) => Fresh(at + t - nowNanos)
  }
}
func main() IO[Unit] { println(debug(freshness(TtlNanos(10), AtNanos(5), 12))) }
```

The Go side then reads as successive narrowing rather than flag bookkeeping:

```go
plan.EvidenceTtl = decide.TtlNoTtl{}
if text := ttlField(obj); text != "" {
    plan.EvidenceTtl = decide.TtlInvalidTtl{}
    if ttl, err := time.ParseDuration(text); err == nil && ttl > 0 {
        plan.EvidenceTtl = decide.TtlTtlNanos{F0: int64(ttl)}
    }
}
```

Two further conventions from the same example: compare `Option[Int]` with
`Some(x)` rather than unwrapping with a sentinel, and cross durations and
timestamps as `Int` nanoseconds (the representation of `time.Duration` and
`UnixNano`) so no rounding happens at the boundary. Keep the projection to
what the decision needs; every call copies it (SPEC §8.7).

## 13. The Go boundary never converts

The boundary never converts **lossily**: all-exported structs copy as records;
other values cross by identity as opaque handles. Only named conversions (such
as `…FromJson` or `…Parse`) and SPEC §8 rows explicitly marked lossy may discard
information. See [ADR 0011](adr/0011-lossless-boundary.md).

In particular, a Go `error` is a `GoError` handle, not a message string.
`io.tryIOCause` retains it alongside the classified `IoError`, and `io.cause`
returns `Option[GoError]` for a native predicate:

```go4t
// dir: boundary-error
// file: main.gt
package main
import "std/io"
import "go:os"
func main() IO[Unit] {
  do {
    loaded <- io.tryIOCause(io.readFile("/nonexistent"))
    match loaded {
      Ok(text) => println(text)
      Err(failure) => match io.cause(failure) {
        Some(original) => println("Go says missing: ${os.isNotExist(original)}")
        None => println(io.ioErrorMessage(failure.error))
      }
    }
  }
}
```

Local failures and recovered panics have no Go cause. Use `io.goErrorMessage`
only when you explicitly want the original message, or `io.ioRaiseCause` to
rethrow without losing the handle.

A Go `*T` argument used in-out binds as `Ref[T]`: allocate a cell, pass it to
the Go function, then read the updated value. Pointer results and fields instead
use `Option[T]`. Here Go's `FlagSet.IntVar` writes its default into a `*int`;
the bound call writes that value back into the cell:

```go4t
// dir: boundary-ref
// file: main.gt
package main
import "std/ref"
import "go:flag"
func main() IO[Unit] {
  let flags = flag.newFlagSet("lesson", flag.continueOnError())
  do {
    cell <- ref.new(0)
    flag.flagSetIntVar(flags, cell, "count", 7, "item count")
    updated <- ref.get(cell)
    println("default=${updated}")
  }
}
```

This illustrates the in-out writeback during one call, not a live alias for
later flag parsing. The same cell/call/read pattern updates a list in
[approval-controller](../examples/approval-controller/decide/apply.gt):
`meta.setStatusCondition(cell, next)` takes `Ref[List[metav1.Condition]]`.

JSON-capable Go types also expose explicit `<t>ToJson` / `<t>FromJson` views.
These use `std/dyn.Value`, not the `std/json.Json` tree from §11, and do not
silently replace a native value with JSON. For example, `go:time` provides
`time.timeToJson` and `time.timeFromJson`; its `Time` value uses the explicitly
lossy `Int` nanoseconds mapping in SPEC §8, not the general handle rule:

```go4t
// dir: boundary-json
// file: main.gt
package main
import "go:time"
func main() IO[Unit] {
  let instant = time.unix(0, 0)
  let view = time.timeToJson(instant)
  do {
    restored <- time.timeFromJson(view)
    println(time.timeFormat(restored, time.rFC3339()))
  }
}
```

`…FromJson` is an `IO` action because decoding can fail; this explicit view is
not a promise that JSON preserves every detail of the original Go value.

## 14. Laws

Tests check chosen inputs. A `law` states an equation for every value of its
typed parameters; its same-named `proof` explains how to reduce both sides to
the same term. Save each example separately and run `go4t check FILE.gt`.
`build`, `run`, `test` and `eval` do not verify proofs. Proved laws generate no
Go code; `test` does execute laws explicitly marked `sample(N)`.

Parenthesize conditional equation operands: write
`(if c { a } else { b }) == x` or `x == (if c { a } else { b })`.
The unparenthesized left-hand form fails with
`law body must be an equality (lhs == rhs)`; the unparenthesized right-hand
form fails with `expected expression, found "if"`.

### Constructor cases: cancellation is idempotent

```go4t
package main
type Order = Pending(Int) | Cancelled(Int) | Shipped(Int)
func cancel(order Order) Order {
  match order {
    Pending(n) => Cancelled(n)
    Cancelled(n) => Cancelled(n)
    Shipped(n) => Shipped(n)
  }
}
law cancelIdempotent(order Order) {
  cancel(cancel(order)) == cancel(order)
}
proof cancelIdempotent(order) {
  match order {
    Pending(_) => refl
    Cancelled(_) => refl
    Shipped(_) => refl
  }
}
func main() IO[Unit] { ioPure(unit()) }
```

`match` opens each constructor case. In each branch `refl` unfolds `cancel`
and checks that the normal forms agree. No `Eq` instance is needed: a law's
outer `==` means definitional equality, not a call to the runtime equality method.
Proof parameter names can differ from law names; binding is positional.
A failed `refl` shows the two stuck normal forms and the failing proof branch,
then tries 128 bounded, deterministic candidates for the original law parameters.
A witness prints `counterexample: ...` and the closed `LHS:`/`RHS:` values.
Otherwise it prints
`no counterexample in 128 candidates: the law may hold; this proof needs a case split, rewrite or induction`,
or `counterexample search skipped: no candidates for <type>` for unsupported
parameter types. The failed proof stays failed even when no witness is found.
Source names are preserved (`#n` only on collisions), and subtraction prints
as `now - at`. See [laws: scope and limits](laws.md) for the search boundaries.

### Guards: exactly when a finite TTL is stale

This is the README's `freshness` decision. Splitting on its guard proves both
outcomes without assuming an ordering theorem about wrapping Int arithmetic:

```go4t
package main
type Ttl = NoTtl | InvalidTtl | TtlNanos(Int) deriving Eq, Debug
type Timestamp = NoTimestamp | InvalidTimestamp | AtNanos(Int) deriving Eq, Debug
type Freshness = Fresh(Int) | TtlInvalid | TimestampMissing | TimestampInvalid | Stale deriving Eq, Debug
func freshness(ttl Ttl, observedAt Timestamp, nowNanos Int) Freshness {
  match (ttl, observedAt) {
    (NoTtl, _) => Fresh(0)
    (InvalidTtl, _) => TtlInvalid
    (TtlNanos(_), NoTimestamp) => TimestampMissing
    (TtlNanos(_), InvalidTimestamp) => TimestampInvalid
    (TtlNanos(t), AtNanos(at)) if nowNanos >= at + t => Stale
    (TtlNanos(t), AtNanos(at)) => Fresh(at + t - nowNanos)
  }
}
func isStale(value Freshness) Bool {
  match value { Stale => true; _ => false }
}
law staleIff(t Int, at Int, now Int) {
  isStale(freshness(TtlNanos(t), AtNanos(at), now)) == (now >= at + t)
}
proof staleIff(t, at, now) {
  if now >= at + t { refl } else { refl }
}
func main() IO[Unit] { ioPure(unit()) }
```

`if` adds a true/false assumption to each branch. Int ring expressions in
operands are normalized, so `now >= t + at` names the same guard.

### Preconditions: stale once the TTL has elapsed

`staleIff` restates the guard. What a reader wants to know is different:
*is the result stale exactly when `now - at >= t`?* Under wrapping `Int` that
is false — `at + t` can overflow — and the checker says so with an input:

```text
counterexample: t = 9223372036854775807, at = 1, now = 0
  LHS: true
  RHS: false
```

A `requires` clause states the range in which the sentence is true. The proof
starts with that precondition assumed, and comparisons are decided only where
the assumptions rule out a wrap:

```go4t
package main
type Ttl = NoTtl | InvalidTtl | TtlNanos(Int) deriving Eq, Debug
type Timestamp = NoTimestamp | InvalidTimestamp | AtNanos(Int) deriving Eq, Debug
type Freshness = Fresh(Int) | TtlInvalid | TimestampMissing | TimestampInvalid | Stale deriving Eq, Debug
func freshness(ttl Ttl, observedAt Timestamp, nowNanos Int) Freshness {
  match (ttl, observedAt) {
    (NoTtl, _) => Fresh(0)
    (InvalidTtl, _) => TtlInvalid
    (TtlNanos(_), NoTimestamp) => TimestampMissing
    (TtlNanos(_), InvalidTimestamp) => TimestampInvalid
    (TtlNanos(t), AtNanos(at)) if nowNanos >= at + t => Stale
    (TtlNanos(t), AtNanos(at)) => Fresh(at + t - nowNanos)
  }
}
func isStale(value Freshness) Bool {
  match value { Stale => true; _ => false }
}
law staleOnceElapsed(t Int, at Int, now Int)
  requires 0 <= t && t <= 1000000000000 && 0 <= at && at <= now && now <= 4000000000000000000 {
  isStale(freshness(TtlNanos(t), AtNanos(at), now)) == (now - at >= t)
}
proof staleOnceElapsed(t, at, now) {
  if now - at >= t { refl } else { refl }
}
law successor(a Int) requires a < 9223372036854775807 { (a + 1 > a) == true }
proof successor(a) { refl }
law transitive(a Int, b Int, c Int) requires a < b && b < c { (a < c) == true }
proof transitive(a, b, c) { refl }
func main() IO[Unit] { ioPure(unit()) }
```

Two things decide comparisons. Assumed facts are closed under the total order
(`transitive` needs no range: `a`, `b`, `c` are just three values). And where
the assumptions bound every operand so that nothing can wrap, a comparison is
treated as integer arithmetic (`successor`, and `now >= at + t` becoming
`now - at >= t`). Without `requires a < 9223372036854775807`, `successor` is
rejected with `counterexample: a = 9223372036854775807`. A precondition is
not checked at run time, and `rewrite` with a law that has one must be able to
establish it from the current assumptions.

### Induction: the length of concatenation

Use direct structural definitions here rather than the standard library's
accumulator-style length. `rewrite` applies the induction hypothesis to the
tail, replacing left with right everywhere in the goal.

```go4t
package main
func len(xs List[Int]) Int {
  match xs { Nil => 0; Cons(_, tail) => 1 + len(tail) }
}
func app(xs List[Int], ys List[Int]) List[Int] {
  match xs { Nil => ys; Cons(head, tail) => Cons(head, app(tail, ys)) }
}
law lenApp(xs List[Int], ys List[Int]) {
  len(app(xs, ys)) == len(xs) + len(ys)
}
proof lenApp(xs, ys) {
  match xs {
    Nil => refl
    Cons(_, tail) => rewrite lenApp(tail, ys) { refl }
  }
}
func main() IO[Unit] { ioPure(unit()) }
```

The recursive functions and self-rewrite decrease the first argument to its
structural tail. Any arguments before the decreasing one must stay unchanged;
later arguments may change. Int ring normalization closes the final addition.
An accumulator implementation usually needs a stronger lemma quantifying over
the accumulator, not just the zero-accumulator result.

The common constructor-match/self-rewrite pattern also has a shorter spelling.
Replace the proof above with either of these:

```text
proof lenApp(xs, ys) { induction xs }
```

```text
proof lenApp(xs, ys) {
  induction xs {
    Nil => refl
    Cons(head, tail) => refl
  }
}
```

`induction` supplies a rewrite for each field whose type is the same data type
as the quantified variable, keeping all other law arguments unchanged. For a
binary tree it supplies both child hypotheses. Bare `induction xs` means every
constructor arm uses `refl` after those rewrites. Explicit arms can use further
tactics. The existing structural-descent and argument-order checks still apply.

Only a pure, terminating, untainted fragment is eligible. Float, IO, untrusted
externs and variable divisors are excluded; trusted prelude/string primitives
are permitted. Exhaustive cases, a type-correct proof and eligible dependencies
are necessary but do not guarantee `refl` succeeds. See [SPEC §6.5](../SPEC.md#65-laws-and-proofs)
for the full rules and diagnostics.

### Regex equations: sampled, not proved

An extern outside the trusted fragment cannot be proved by normalization.
Keep such an equation as a law with an explicit testing tactic:

```go4t
package main
import "std/regex"
law captures() {
  regex.regexCaptures("^(a)(b)$", "ab") == Some(["a", "b"])
}
proof captures() { sample(30) }
law replaceIdentity(text String) {
  regex.regexReplace("z", text, "z") == text
}
proof replaceIdentity(text) { sample(100) }
func main() IO[Unit] { ioPure(unit()) }
```

`go4t check FILE.gt` reports each law as
`tested by go4t test (sample N), not proved`, without executing or normalizing
it. Run `go4t test FILE.gt` to execute the samples. A failed sample prints the
first counterexample's `debug` bindings, seed and sample index, and fails the
test command. `GO4T_SAMPLE_SEED` selects another reproducible Int seed.

`sample(N)` must be the whole proof body, with an Int literal from 1 to 100000.
Inputs need a bounded generator: primitives, tuples, algebraic data, lists and
maps are supported; functions, IO and handles are not. Binding displays use
custom `Debug` when available, otherwise structural output. Constructors
are generated to depth four, with scalar edge cases including NaN, signed zero,
infinities and Int extremes. The result needs `Eq`; direct Float results compare
bits. Sampling can miss counterexamples, does not certify termination, and
cannot be used as a rewrite lemma. An entirely law-eligible equation must be
proved instead; `sample` cannot replace a difficult proof.

## 15. Mutable state in IO: Ref

Keep a counter in an opaque `Ref[Int]`. Allocation, reading and modification are
deferred IO; `modify` runs its pure callback atomically under the cell's lock.

```go4t
package main
import "std/ref"
func main() IO[Unit] {
  do {
    counter <- ref.new(0)
    ref.modify(counter, func(n) => n + 1)
    ref.modify(counter, func(n) => n + 1)
    count <- ref.get(counter)
    println("count=${count}")
  }
}
```

## 16. Throughput maps: mutmap and freeze

For update-heavy work, count in an IO-only `MutMap`, then `freeze` an independent
persistent snapshot. Use `std/map` to read that snapshot in pure code. Here the
top word is the first entry after sorting by descending count; ties keep key order.

```go4t
package main
import "std/mutmap"
import maps "std/map"
import "std/list"
import "std/string"
func countWord(counts MutMap[String, Int], word String) IO[Unit] {
  do {
    previous <- mutmap.lookup(counts, word)
    let n = match previous { None => 0; Some(value) => value }
    mutmap.insert(counts, word, n + 1)
  }
}
func main() IO[Unit] {
  do {
    counts <- mutmap.new()
    list.forEach(string.words("go types go laws go types"), func(word) => countWord(counts, word))
    snapshot <- mutmap.freeze(counts)
    let ranked = list.sortBy(maps.mapToList(snapshot),
      func((_, a), (_, b)) => compare(b, a))
    println("top=${debug(head(ranked))}")
  }
}
```

Later mutations of `counts` cannot change `snapshot`. The lookup/insert pair is
not a transaction; this example updates sequentially.

## 17. Conditional effects: when and unless

`if` always needs an `else`. Import the prelude explicitly for `when` and
`unless`: they select a `Monad[Unit]` action, using a no-op action otherwise.

```go4t
package main
import "std/prelude"
func main() IO[Unit] {
  let verbose = true
  let quiet = false
  do {
    prelude.when(verbose, println("details enabled"))
    prelude.unless(quiet, println("not quiet"))
    prelude.when(false, println("not printed"))
    unit()
  }
}
```

Only the selected IO effects run. Arguments are still strict: these combinators
do not defer pure work performed while constructing the action.

## 18. Laws you cannot prove: sample(N)

A regex-based parser depends on untrusted externs, so keep its equation as a
sampled law rather than claiming a proof. Save this example as `parser.gt`.
The second law repeats §14's structural example with `induction xs` replacing
the manual `Nil` / `Cons` arms and self-rewrite.

```go4t
package main
import "std/regex"
func parseWord(text String) Option[String] {
  match regex.regexCaptures("^word:([a-z]+)$", text) {
    Some([word]) => Some(word)
    _ => None
  }
}
law parsePrefixedWord(text String) {
  parseWord("word:" + text) ==
    (if regex.regexMatch("^[a-z]+$", text) { Some(text) } else { None })
}
proof parsePrefixedWord(text) { sample(500) }
func len(xs List[Int]) Int {
  match xs { Nil => 0; Cons(_, tail) => 1 + len(tail) }
}
func app(xs List[Int], ys List[Int]) List[Int] {
  match xs { Nil => ys; Cons(x, tail) => Cons(x, app(tail, ys)) }
}
law lenApp(xs List[Int], ys List[Int]) {
  len(app(xs, ys)) == len(xs) + len(ys)
}
proof lenApp(xs, ys) { induction xs }
func main() IO[Unit] { println(debug(parseWord("word:go"))) }
```

`go4t check parser.gt` checks the structural proof and emits the notice
`tested by go4t test (sample 500), not proved` for `parsePrefixedWord`.
`go4t test parser.gt` runs its 500 samples; `check` does not run them.
Sampled laws can miss counterexamples and cannot be rewrite lemmas.
In contrast, `induction xs` supplies all constructor arms and tail hypotheses,
then closes each arm with `refl`; `lenApp` is proved, not sampled.

## Next steps

- [README examples](../README.md#examples) has a command for every example directory,
  including file IO, collections, State, parsing, HTTP, formatting and diagnostics.
- [stdlib](stdlib.md) gives the exact signature and boundary of every declaration.
- [Editor setup](../editors/README.md) configures `go4t lsp`; `go4t fmt -w FILE`
  formats source without type checking, and `go4t fmt -l DIR` is a formatting gate.
- `go4t build --emit-go DIR --package embedded FILE` emits a Go package with
  `Main() rt.IO` for a Go caller to run; it emits sources, not an executable.
