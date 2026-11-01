# gofx for Go developers

Start from the repository checkout with Go 1.22 or newer. `go install ./cmd/gofx`
installs the CLI; add Go's bin directory to PATH. Alternatively replace `gofx` in
commands below with `go run ./cmd/gofx`. Keep saved programs **beneath this checkout**
so generated Go can resolve `gofx/rt`; an independent app needs its own Go module
requiring/replacing this checkout. `gofx.mod` controls gofx imports, not Go dependencies.

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
Debug is structural. **`deriving Show` supports only enums** (all constructors
nullary), rendering their source constructor names. Write an explicit Show
instance for data types with payloads or a different display form.

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

`io.tryIO` returns an action that succeeds with a Result, even if the wrapped
action fails or panics. It does not catch an error that happened before the
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
    let typed: Result[String, Int] = failed
    panicked <- io.tryIO(divideLater())
    println("${typed}; ${panicked}")
  }
}
```

The recovered panic payload has no extra `panic:` prefix. Unhandled ordinary IO
failure prints `error: ...`, exit 1; a direct panic **during action execution**
prints `error: panic: ...`, exit 2. Panics during action construction are outside
RunMain recovery. A worker panic from concurrency is instead an ordinary IO
error with `panic: ...` text. Installed `gofx run` preserves child exit status;
the outer Go launcher does not preserve arbitrary nonzero statuses.

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
the nearest module root, then GOFX_PATH entries, then embedded std. No download
manager is involved. Local declarations may shadow implicit prelude names with a
warning; explicit module qualification removes ambiguity.

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
    let typed: Result[String, Int] = recovered
    ioPure(typed == Err("expected"))
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

`parMapIO` and `parBoth` join **all** work before reporting the first observed
failure; an indefinitely blocked sibling still blocks. `race` is first-completion,
not first-success. Timeout returns None on expiry, not on timely failure. Race
losers and timed-out actions are **not cancelled** and may continue effects.
Unawaited fibers do not keep the process alive after main finishes. Use explicit
joins when effects must finish; this is not structured concurrency.

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
