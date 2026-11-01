# Examples

Run these commands from the repository root.

Match arms written one per line omit the leading `|`. One-line matches also
omit it before the first arm, keeping `|` only between arms, as in
`match r { Err(e) => "Err(${e})" | Ok(a) => "Ok(${a})" }`.
Matches whose arms span several lines retain leading `|` separators.

Record patterns name the fields they match: `Ctor{f: p, g: q}` accepts fields
in any order. Without `..`, every field must appear exactly once; a trailing
`..` ignores unlisted fields, as in `User{name: n, ..}`. Unknown or duplicate
fields are errors. Positional constructor patterns remain valid alongside
record patterns. Tuple positions are zero-based: `p.0` reads the first element
and `p.1` the second, with the tuple type known at the access and the index in
range. The prelude's `fst` and `snd` helpers remain available.

## HTTP

`go run ./cmd/gofx run examples/http/main.gfx` requires network access. It prints
the HTTP status and an `Option` result using the `Show` class and string
concatenation:

```text
status: 200
twice: 23
```

## Tree

`go run ./cmd/gofx run examples/tree/main.gfx` needs no network access. It defines
a recursive `Tree`, a `Functor[Tree]` instance, and a polymorphic
`render[A Show]` function with one match arm per line and no leading `|`.
Mapping over the tree multiplies each value by ten:

```text
((. 10 .) 20 .)
```

## Orders

`go run ./cmd/gofx run examples/orders/main.gfx` requires network access for
the warehouse notification. It combines an `Order` data type and its `Show`
instance with the prelude's `List`, `Functor`, and constrained `Show`
instances. `Option` validation and `IO` sequencing keep failure and effects
explicit. With a successful HTTP 200 response, it prints:

```text
orders:    [Pending#1(300), Pending#2(0), Cancelled#3]
cancelled: [Cancelled#1, Cancelled#2, Cancelled#3]
twice:     [Cancelled#1, Cancelled#2, Cancelled#3]
shipped:   [Some(Shipped#1(300)), None, None]
notified warehouse: total=800
cannot ship both (second has zero amount)
IO twice:  22
```

## Lists

`go run ./cmd/gofx run examples/lists/main.gfx` needs no network access. It
demonstrates empty and nested list literals, recursively resolved
`Show[List[A]]` dictionaries, list helpers, and a `List`-based `do` block.
List bind visits each left-hand element in order, producing the cartesian
product of `[1, 2]` and `[10, 20]`:

```text
empty:    []
nested:   [[1, 2], [3]]
filter:   [3, 4]
reverse:  [4, 3, 2, 1]
append:   [1, 2, 3, 4]
length:   4
product:  [11, 21, 12, 22]
```

## Inventory

`go run ./cmd/gofx run examples/inventory/main.gfx` requires network access for
the order notification. Constructors share their natural names with the `Sku`,
`Stock`, and `Line` types. `Stock` and `Line` are records: named construction
uses `Stock{sku: ..., qty: ...}`, field reads use `s.sku` / `l.qty`, and
`s{qty: s.qty - l.qty}` copies a stock with an updated quantity. Tuple
expressions and nested tuple patterns compare SKUs and combine stocks; an
as-pattern (`whole @ Stock(sku, qty)`) retains an unchanged stock without
rebuilding it. Record constructors still support positional calls and patterns.
The `available` fold uses `Stock{sku: s, ..}` to match only the SKU, reading
the quantity from the original stock.
Partially applied instance heads `Functor[Result[E]]` and `Monad[Result[E]]`
sequence validation with a generic error type, while `Show[Result[E, A]]` uses
both constrained dictionaries. String interpolation (`"stocks:   ${stocks}"`)
uses each expression's `Show` instance, including the prelude's tuple `Show`
instance through the generic `tag` helper.
With a successful HTTP 200 response, its stdout is unchanged:

```text
stocks:   [apple=8, pear=2, fig=9]
tagged:   [stock:apple=8, stock:pear=2, stock:fig=9]
tagged?:  Some(n=42)
order1: Ok([7xapple, 1xfig])
order2: Err(short of pear: need 3, have 2)
order3: Err(invalid quantity for apple)
  notified 2 lines
  remaining [apple=1, pear=2, fig=8]
io:(1, true)
```

## Records, interpolation, and let-polymorphism

`go run ./cmd/gofx run examples/records/main.gfx` needs no network access.
`User{name: String, age: Int, email: Option[String]}` declares a record
constructor. Named construction must provide every field exactly once, in any
order. Field access and copy updates require a single-constructor record type;
`u{age: u.age + 1}` preserves the name and email without modifying `u`.
`Show[User]` matches `User{name: n, ..}` to bind the name while ignoring the
other fields in the pattern.
The example maps this update over three users with `fmap`, and sequences an
`Option`-based `do` that reads `u.email`. Missing email short-circuits to `None`.

String interpolation writes `${expression}` inside a string and resolves its
`Show` instance. Here, `Show[User]` interpolates the name, age, and optional
email, and interpolating lists and tuples uses their constrained instances.
Nested string literals are allowed inside interpolation expressions, as in
`"strings:  ${pair("hi")}"`. `$$` produces a literal `$`; a `$` not followed by
`{` is also literal. Braces outside `${...}` are ordinary characters, so `{`
and `}` need no escaping, and `{{` is two literal braces rather than an escape.

Local `let` bindings generalise lambdas and literals/constructor applications:
`let pair = func(x) => (x, x)` can be called at both `Int` and `String`.
Class constraints are generalised too, so a let-bound lambda using `show`
can receive the appropriate dictionary at each use. Function parameters are
not generalised, and other expressions such as calls, `do`, and `match`
remain monomorphic.

Expected stdout (only):

```text
users:    [Ada (36, Some(ada@example.com)), Bob (28, None), Cara (41, Some(cara@example.com))]
older:    [Ada (37, Some(ada@example.com)), Bob (29, None), Cara (42, Some(cara@example.com))]
ints:     (7, 7)
strings:  (hi, hi)
contacts: [Some(Ada: ada@example.com), None, Some(Cara: cara@example.com)]
```

The unused `let x = None` deliberately leaves its element type unconstrained.
Defaulting that type to `Unit` emits this expected warning on stderr, not stdout:

```text
examples/records/main.gfx:23:11: warning: type of None defaulted to Unit; add an annotation or use the value
```

Warnings are non-fatal: `gofx check` still exits successfully, and `gofx run`
continues to execute. Unconstrained empty lists and other unused values can
also trigger defaulting warnings. Add a type annotation or use the value in a
typed context to determine the type instead.

## Bank

`go run ./cmd/gofx run examples/bank/main.gfx` requires network access for
the audit notification. It combines named record construction and copy updates,
`Result` validation, tuple patterns, list operations, and a polymorphic local
`describe` helper. Interpolation uses `${expression}` without changing record
syntax such as `Account{id: ..., owner: ..., balance: ...}` or
`a{balance: a.balance - amt}`. Matches use one-line separators or one arm per
line without a leading `|`, retaining leading separators when arms span
several lines.
`Show[Tx]` uses `Transfer{from: f, to: t, amount: a}` alongside positional
`Deposit` and `Withdraw` patterns, and `Show[Failure]` names the fields of
`Insufficient`. The first `applyAll` result is read with `r1.0` and `r1.1`;
the `step` transition keeps its nested tuple patterns.

`applyAll` sequences each transaction with `forEach` and `modify` over
`State[(List[Account], Option[(Failure, Int)]), Unit]`, then uses `execState`
to return the final book and failure. The pure `step` transition preserves the
book after the first failure and counts subsequent skipped transactions.
The final `mapM` traverses account IDs in `Result[Failure]`, collecting the
accounts in order or returning the first lookup failure.

With a successful HTTP 200 response, expected stdout is:

```text
book:  alice<A-1>:100; bob<B-2>:20
txs:   deposit 30 -> B-2; transfer 50 A-1 -> B-2
good:  [alice<A-1>:50, bob<B-2>:100] / None
  audit good: sent=true total=150
bad:   [alice<A-1>:50, bob<B-2>:90] / Some((B-2: need 500, have 90, 1))
ugly:  Some((no such account Z-9, 0))
check: Err(bad amount 0)
sum:   120 -> 140
zip:   [a, b]
mapM:  Ok([alice<A-1>:100, bob<B-2>:20])
```

## State

`go run ./cmd/gofx run examples/state/main.gfx` needs no network access.
`counter` is a `do` block over `State[Int]`: `get` reads the counter, `put`
replaces it, and `modify` doubles it. Starting at 10, it returns the increase
of 12 and leaves the state at 22. `runState` returns both values, while
`evalState` and `execState` select the result and final state, respectively.
`fmap` changes only the result, leaving the final state untouched.

The stack uses `State[List[Int]]` and `forEach` to push 3 and then 5 before
popping; popping an empty stack returns `None` without changing the stack.
The same generic `forEach` sequences the final three `IO` prints in order.
Both `forEach` and `mapM` work with any `Monad` and recurse over the list
using `bind` and `pure`; `forEach` discards results, while `mapM` collects them.

Expected stdout:

```text
counter: (12, 22)
value:   12
state:   22
mapped:  (120, 22)
stack:   (Some(5), [3, 1])
empty:   (None, [])
IO: first
IO: second
IO: third
```

## Generated Go

`go run ./cmd/gofx build examples/http/main.gfx` writes
`examples/http/main_gen/main.go` with `package main`; the tree, orders, lists,
inventory, records, bank, and state examples similarly use their own `main_gen/main.go`.
Each source file gets its own `<base>_gen/` directory, so multiple `.gfx`
programs can share a source directory.
`run` builds this directory and executes it with `go run` from the module root.
Generated `*_gen/` directories are ignored by Git. Legacy `<base>_gen.go` files
are neither written nor removed.
