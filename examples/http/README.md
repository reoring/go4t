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
`s{qty: s.qty - l.qty}` copies a stock with an updated quantity. `Eq[Sku]`
defines equality once; stock merging, lookup, and consumption use `==` on
SKUs instead of a separate equality helper. Nested tuple patterns combine
stocks, while an as-pattern (`whole @ Stock(sku, qty)`) retains an unchanged
stock without rebuilding it. Record constructors still support positional
calls and patterns.
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
`Eq[AccountId]` supplies `==` for account lookup and replacement, so equality
is shared with generic helpers such as `elem` rather than a `sameId` function.

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

## Calc

`examples/calc/main.gfx` is a small expression-language interpreter: an `Expr` ADT with
record constructors, a `Result`-monad evaluator (partially applied `Monad[Result[E]]`),
free-variable analysis with `State`, record patterns (`Bin{op: op, lhs: l, rhs: r}`), tuple
projections (`p.0`, `p.1`), `forEach` over `IO` and `mapM` over `Result`.
Free-variable analysis uses `elem` for membership in the bound-name list;
environment lookup still compares the name alone, not the whole binding tuple.

```text
arith: (1 + (2 * 3))
  => Ok(7)
  free: []
let: let x = 10 in (x * (x + 1))
  => Ok(110)
  free: []
if: if (1 < 2) then 100 else 0
  => Ok(100)
  free: []
free: (y + 1)
  => Err(unbound variable y)
  free: [y]
div0: (1 / 0)
  => Err(division by zero)
  free: []
type: ((1 < 2) + 1)
  => Err(expected Int, got true)
  free: []
nested: let a = 2 in let b = (a * a) in if (a < b) then b else a
  => Ok(4)
  free: []
all: Err(unbound variable y)
ok:  Ok([7, 110, 100, 4])
```

## Equality, ordering, and list helpers

`go run ./cmd/gofx run examples/eqord/main.gfx` needs no network access.
`Eq[AccountId]` makes `==` usable inside `filter` and supplies the dictionary
for `elem`. `Priority` declares its constructors in a different order from
its semantic order: its `Eq` and `Ord` instances use `Low < Normal < High`.
`sort`, `maximum`, and `minimum` use that instance rather than declaration
order. `sortBy` accepts a comparator and uses stable insertion sort.

The prelude derives equality and lexicographic ordering for pairs and lists
from their elements. For `Option`, `None` precedes `Some`, and `Some` values
compare their contents. `Bool` orders `false < true`; `Ordering` orders
`LT < EQ < GT`. `Unit` has equality but no ordering instance.
The zipped-name filter compares whole `(String, Int)` tuples, so both fields
must match. `forEach` then destructures each pair directly in
`func((n, s)) => println("${n}: ${s}")`.

`let xs: List[Int] = []` fixes the empty list's element type without a
defaulting warning; a typed `let` inside `do` works the same way.
`Some(x) if x > 10 =>` is a guarded arm: when the guard is false, the next
`Some(x)` arm handles the value. Guarded arms alone do not make a match
exhaustive.

`head` returns `None` for an empty list, while `tail` returns an empty list.
`take` with a nonpositive count returns an empty list; `drop` with that count
returns the original list. `maximum` and `minimum` return `None` for an empty
list. `sum` totals integers and `concatMap` concatenates mapped lists.
The list-prefix checks exercise equality on both `List[Int]` and
`Option[Int]`. The separate `false && ...` / `true || ...` expressions prove
short-circuiting by leaving a division by zero unevaluated on each right
side. `minInt()` and `maxInt()` provide the signed 64-bit bounds.

Expected stdout (no stderr warnings):

```text
accounts: [A-1, A-1]
priorities: [Low, Low, Normal, High]
contains: true / false
maximum: Some(High)
minimum: Some(Low)
empty: [] / None / [] / None
head/tail: Some(1) / [2, 3]
take/drop: [1, 2] / [3]
sum: 6
concatMap: [1, 10, 2, 20, 3, 30]
descending: [3, 2, 1]
tuples: [(Ada, 42)]
Ada: 42
Bob: 7
Cara: 99
guard: large 42 / small 7 / none
starts with 1: false / true
short-circuit: false / true
bounds: -9223372036854775808 / 9223372036854775807
```

## Scheduler

```sh
go run ./cmd/gofx run examples/scheduler/main.gfx
```

The scheduler combines task-specific ordering and display, guarded buckets, and
a six-hour plan. Expected output:

`Priority` derives `Eq` and `Ord` in declaration order while keeping its custom
display; `Status` derives `Eq`. The plan is unpacked with
`let (today, later) = plan(tasks, 6)`. The `top:` statistic uses
`maybe(head(sort(tasks)), "none", func(t) => t.title)` to display a title or its
fallback. Other Option helpers are `fromOption(o, defaultValue)`, which returns
the contained value or the default, and the predicates `isSome(o)` / `isNone(o)`.

```text
== all (sorted) ==
  [ ] #1 fix login bug (URGENT, 2h)
  [x] #5 update deps (HIGH, 1h)
  [ ] #6 add metrics (HIGH, 3h)
  [ ] #3 migrate database (HIGH, 12h)
  [~] #4 refactor parser (normal, 5h)
  [ ] #2 write release notes (low, 1h)
== buckets ==
  quick-win: [1]
  big-rock: [3, 6]
  backlog: []
  normal: [2, 4]
  done: [5]
== today (6h) ==
  [~] #1 fix login bug (URGENT, 2h)
  [~] #6 add metrics (HIGH, 3h)
  [~] #2 write release notes (low, 1h)
  total 6h, 2 left
== stats ==
  top: fix login bug
  longest: Some(12)
  Low < Urgent = true, minInt = -9223372036854775808
  next after today: Some([~] #4 refactor parser (normal, 5h))
```

## Cards

`examples/cards/main.gfx` (contributed by review): poker hand classification with
`deriving Eq, Ord, Show` on enums and the `Hand` ADT (declaration order = hand strength), an
LCG shuffle in `State[Int]`, `let (hands, seed) = runState(...)`, record/tuple patterns in
lambda parameters, guards mixed with structural patterns, and `fromOption`/`maybe`/`isSome`.

```text
== fixed hands ==
  royal: StraightFlush(Ace)
  fullhouse: FullHouse(Nine, Two)
  twopair: TwoPair(King, Five)
  straight: Straight(Ten)
== deck ==
  52 cards, min Some(Card{rank: Two, suit: Clubs})
== dealt (seed -> 109007879884728) ==
  Jack/Clubs Seven/Clubs Eight/Hearts Nine/Hearts King/Hearts => HighCard(King)
  Four/Hearts Queen/Diamonds Six/Spades Three/Diamonds Five/Spades => HighCard(Queen)
  Three/Hearts Ten/Diamonds Eight/Clubs Nine/Spades Ace/Spades => HighCard(Ace)
  winner: HighCard(Ace) with Three/Hearts Ten/Diamonds Eight/Clubs Nine/Spades Ace/Spades 
  Pair(Ace) < Trips(Two) = true, isSome(head(hands)) = true
```

## Wc

`examples/wc/main.gfx` reads stdin, reports line and word counts, and folds the
words into a `List[(String, Int)]` frequency table. It sorts by descending count,
then ascending word for ties. The first argument selects the top N entries;
without an argument it defaults to 10. Words are case-sensitive whitespace
tokens, with no punctuation removal.

Run from the repository root:

```sh
go run ./cmd/gofx run examples/wc/main.gfx -- 3 < examples/wc/input.txt
```

Output:

```text
lines: 12, words: 60
   24 the
    5 garden
    5 leaves
```

The invalid-number path prints usage to stdout and exits with status 2:

```sh
go run ./cmd/gofx run examples/wc/main.gfx -- x < examples/wc/input.txt
```

Output:

```text
usage: wc [N]
```

The generated program exits 2; the outer `go run` wrapper may additionally
report `exit status 2` on stderr and return its own nonzero status.

## Strings

`go run ./cmd/gofx run examples/strings/main.gfx` needs no network or stdin.
Fixed data demonstrates splitting, trimming and joining, Unicode rune indexes
and counts, replacement of all occurrences, repetition, integer parsing, and
decimal padding.

Output:

```text
split: [  red,  green , blue  ]
join/trim: red / green / blue
upper: RED / GREEN / BLUE
replace: baNANA
substring: é界
chars: [A, é, 界]
repeat: hahaha
parse: Some(42) / None
padded: [   42]
length: 3
```

## Parser combinators

`examples/parser/main.gfx` (contributed by review): a user-defined `Parser[A]` with
`Functor`/`Monad` instances, so the prelude's `map` and `do` drive a left-associative
arithmetic parser over a token list. No language support for parsing exists; the HKT
machinery alone makes this a 120-line library.

```text
2 + 3 * 4 => Some(14)
(2 + 3) * 4 => Some(20)
20 - 5 - 3 => Some(12)
2 * (3 + (4 - 1)) => Some(12)
7 => Some(7)
empty => None
2 + => None
(2 + 3 => None
2 3 => None
() => None
```

## Routes

`examples/routes/main.gfx` (contributed by review): shortest paths over a small station
network with records (`Road{from, to, minutes}`), record update for delays, guards in the
priority-queue insertion, and `Option` for unreachable destinations.

```text
fastest: 11 min, [Home, Park, Market, Museum, Harbor]
return trip: 11 min, [Harbor, Museum, Market, Park, Home]
Market closed: 15 min, [Home, Park, Museum, Harbor]
Park delayed: 12 min, [Home, Market, Museum, Harbor]
via Museum: 11 min, [Home, Park, Market, Museum, Harbor]
already there: 0 min, [Home]
isolated island: unreachable
via island: unreachable
empty network: unreachable
```

## Generated Go

`go run ./cmd/gofx build examples/http/main.gfx` writes
`examples/http/main_gen/main.go` with `package main`; the tree, orders, lists,
inventory, records, bank, state, calc, and eqord examples similarly use their
own `main_gen/main.go`.
Each source file gets its own `<base>_gen/` directory, so multiple `.gfx`
programs can share a source directory.
`run` builds this directory and executes it with `go run` from the module root.
Generated `*_gen/` directories are ignored by Git. Legacy `<base>_gen.go` files
are neither written nor removed.
