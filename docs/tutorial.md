# gofx for Go developers

Start from the repository checkout with Go 1.22 or newer. `go install ./cmd/gofx`
installs the CLI; add Go's bin directory to PATH. The installed binary embeds its
runtime, so saved programs can live anywhere: `run`, `test`, and `eval` create a
temporary Go module when needed and clean it afterward. Keep the Go toolchain on
PATH. Alternatively use `go run ./cmd/gofx` from the checkout root. Externs outside
`gofx/rt` and its subpackages need an application Go module providing those packages
and `gofx`. `gofx init` creates that project; `gofx get` records language
dependencies in `gofx.mod` while Go manages their versions and checksums.

Every `gofx` block below is a complete checkable example, alone or with the files
in its named group. For ordinary blocks, save as `lesson.gfx` and run
`gofx check lesson.gfx` then `gofx run lesson.gfx`. Use separate directories or
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

```gofx
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

A Go interface is a type describing methods on a value. A gofx class describes
operations parameterised by a type; an **explicit instance** supplies its
implementation. A constrained generic function receives that implementation as
a compiler-managed dictionary. You can define instances for existing types;
loaded modules must agree on one instance per class/type constructor.

```gofx
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

```gofx
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

In Go, calling `fmt.Println` writes immediately. In gofx, `println` returns an
`IO[Unit]` description. Merely creating or discarding the description does not
run it. Main returns the action the runtime executes.

```gofx
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
its result. A `let` computes a pure local value. The last expression must still
be an action of the **same Monad**; a local function is a statement, not an action.

```gofx
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

## 5. Result replaces repeated error-tuple branching

`Result[E, A]` distinguishes a typed error from a success. Compared with Go's
`(T, error)` convention, the constructor records which case exists; the Monad
passes on the first Err without calling later continuations.

```gofx
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

```gofx
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
Installed `gofx run` preserves child exit status; the outer Go launcher does not
preserve arbitrary nonzero statuses.

For example, missing-file recovery can distinguish a missing configuration from
other failures:

```gofx
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

Create `module-demo/main.gfx` and `module-demo/greeting/greeting.gfx` from the
following two blocks, then run `gofx run module-demo`. Without `gofx.mod`, the
entry directory is the import root. The greeting module loads every immediate
`.gfx` file in its directory, with one shared package spelling.

```gofx
// dir: module-demo
// file: main.gfx
package main
import hello "greeting"
func main() IO[Unit] { println(hello.message(hello.Person{name: "Ada"})) }
```

```gofx
// dir: module-demo
// file: greeting/greeting.gfx
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

For a stable project root, put the line `module demo` in `module-demo/gofx.mod`;
then `"demo/greeting"` also resolves to the greeting directory. Resolution tries
the nearest module root, matching `gofx.mod` requires, GOFX_PATH entries, then
embedded std. Local declarations may shadow implicit prelude names with a
warning; explicit module qualification removes ambiguity.

### Adding a library dependency

In a new directory, `gofx init example.com/myapp` creates `go.mod` and
`gofx.mod`, refusing to overwrite either. The Go module uses `go 1.22`,
`require gofx v0.0.0` and a local runtime replacement. It first tries
`go list -m -json gofx` from the current directory and uses its resolved `Dir`.
If that fails, it extracts the embedded runtime to
`$XDG_CACHE_HOME/gofx/runtime/<sha256>` on Linux (default
`$HOME/.cache/gofx/runtime/<sha256>`). This content-addressed cache is reused;
no project-local `.gofx` directory is created. The replacement is machine-local,
so keep the referenced checkout/cache or adjust the replacement when relocating.

For a published library, run `gofx get example.com/lib@v1.0.0`.
For a local library at `../lib`, first put
`replace example.com/lib => ../lib` in **go.mod** and run the same command;
Go accepts that replacement offline, without a published version.
The command runs `go get` at the nearest gofx project root and adds or updates
one `require example.com/lib v1.0.0` line in `gofx.mod`. `@latest` and other
Go version queries are recorded as the selected version. Go owns `go.sum`
checksums and all replacements; `gofx.mod` supports module/require lines,
blank lines and `//` comments, not `replace` directives.

Import `"example.com/lib/shapes"` to load the dependency's `shapes/` sources.
`go list -m -json example.com/lib` supplies its directory using the entry
project's Go graph, so its replacements also apply to transitive dependencies.
A dependency's own imports use its own gofx root and requires. Missing Go
dependencies are errors, not silent fallbacks to GOFX_PATH.

Try the complete offline [dependency example](../examples/deps):

```sh
gofx run examples/deps/app
# area: 42
```

## 7. Pipelines, sections and recursive helpers

`x |> f(a)` means `f(x, a)`. A call with `_` instead becomes a lambda at those
argument positions, so a pipeline can select a non-first argument. Local function
groups can recurse mutually; a bare-name lambda let can recurse into itself.

```gofx
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

```gofx
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

## 8. Testing with gofx test

Save this as `lesson-tests.gfx` and run `gofx test lesson-tests.gfx`. It also
passes `gofx check` because it includes a normal main; **test does not require
main**, and replaces/removes it before checking. Test names must start `test`
with a nonempty suffix, have no parameters/type parameters, and explicitly return
Bool or IO[Bool]. There is no assertion DSL: return the Boolean contract.

```gofx
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
func main() IO[Unit] { println("Run this file with gofx test.") }
```

The harness reports `PASS testArithmetic`, `PASS testRecovery`, then
`2 passed, 0 failed`. False returns and failed IO report FAIL and later tests
still run; any failed test gives exit 1. An empty suite succeeds. A panic is not
a failed-assertion mechanism: it follows ordinary runtime panic policy.

For one-off exploration, run `gofx eval '1 + 2'` or
`gofx eval 'testArithmetic()' lesson-tests.gfx`. Eval uses Show on the result;
Unit has no Show, so an `IO[Unit]` expression is not a displayable eval result.

## 9. FFI via gofx bind

Create `ffi-demo/main.gfx` and `ffi-demo/gostd.gfx` using the two blocks below.
The second shows the minimal declarations needed here; replace it with the
**complete generated output** by running:

```sh
gofx bind strings --prefix str -o ffi-demo/gostd.gfx
gofx check ffi-demo
gofx run ffi-demo
```

```gofx
// dir: ffi-demo
// file: main.gfx
package main
func main() IO[Unit] { println(strToUpper(strRepeat("go", 3))) }
```

```gofx
// dir: ffi-demo
// file: gostd.gfx
package main
extern func strRepeat(p0 String, p1 Int) String = "strings".Repeat
extern func strToUpper(p0 String) String = "strings".ToUpper
```

This prints `GOGOGO`. The generator adapts Go `string` directly and Go `int`
to gofx's canonical int64. Other supported natural types include float64, bool,
rt.Unit, rt.IO, []string, []int64 and primitive `(T, bool)` Option results.
A Go `(T, error)` return maps to `IO[T]`; the adapter defers the Go call and its
conversions until execution. No Go wrapper is needed for these signatures.

Bind prints skipped-function reasons for callbacks, pointers, generics, variadics,
unsupported named types/results and uninferable any/rt.IO APIs. Available Go
functions vary with toolchain version. `--prefix` avoids accidental collisions;
for another receiving package, pass its sources/directory before `-o`.
`check` alone does not validate the Go ABI—`build`/`run` do. A pure extern should
really be pure; signatures are a trust boundary, not a side-effect analyzer.

## 10. Concurrency without mutable shared state

Go goroutines are exposed as IO actions and Fiber values. `fork` starts one task
when executed; `await` joins it. The result is cached, so later awaits do not
repeat its effects. Parallel collection results preserve input order.

```gofx
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

```gofx
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

```gofx
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

Json numbers are Float. Integers beyond binary64's exact range may lose precision;
maxInt cannot round-trip through its rounded encoding. NaN/infinity cannot be
printed. None encodes as null, and Some of a null-like value is indistinguishable
from None on decode. Use an explicit tagged ADT when that distinction matters.

## Next steps

- [README examples](../README.md#examples) has a command for every example directory,
  including file IO, collections, State, parsing, HTTP, formatting and diagnostics.
- [stdlib](stdlib.md) gives the exact signature and boundary of every declaration.
- [Editor setup](../editors/README.md) configures `gofx lsp`; `gofx fmt -w FILE`
  formats source without type checking, and `gofx fmt -l DIR` is a formatting gate.
- `gofx build --emit-go DIR --package embedded FILE` emits a Go package with
  `Main() rt.IO` for a Go caller to run; it emits sources, not an executable.
