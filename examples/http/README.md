# Examples

Run these commands from the repository root.

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
`render[A Show]` function with `|`-separated match arms. Mapping over the tree
multiplies each value by ten:

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
`Stock`, and `Line` types. Tuple expressions and nested tuple patterns compare
SKUs and combine stocks; an as-pattern (`whole @ Stock(sku, qty)`) retains an
unchanged stock without rebuilding it. Partially applied instance heads
`Functor[Result[E]]` and `Monad[Result[E]]` sequence validation with a generic
error type, while `Show[Result[E, A]]` uses both constrained dictionaries.
The prelude's tuple `Show` instance also works through the generic `tag` helper.
With a successful HTTP 200 response, it prints:

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


## Generated Go

`go run ./cmd/gofx build examples/http/main.gfx` writes
`examples/http/main_gen/main.go` with `package main`; the tree, orders, lists,
and inventory examples similarly use their own `main_gen/main.go`. Each source file gets its own
`<base>_gen/` directory, so multiple `.gfx` programs can share a source directory.
`run` builds this directory and executes it with `go run` from the module root.
Generated `*_gen/` directories are ignored by Git. Legacy `<base>_gen.go` files
are neither written nor removed.
