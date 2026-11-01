# Multiple type-class constraints

Run from the repository root:

```sh
go run ./cmd/gofx run examples/multi-constraint/main.gfx
```

Observed output:

```text
[1, 2, 3]
[apple, banana, pear]
match: 7
Some(different: pear)
same: true
method: 9 / EQ
let: 4EQ / trueEQ
```

`showSorted[A Show + Ord]` needs both display and ordering dictionaries.
`showMatching[F[_] Monad, A Eq + Show]` combines a higher-kinded constraint
with two constraints on its element type, and runs with both `IO` and `Option`.
`same[A Eq + Ord]` uses the `Ord` dictionary for its inherited `Eq` operation.
The class method `describe[A Show + Ord]` has its own constraint list, while
`f` infers `Show` and `Ord` on one generalised type parameter and is used at
both `Int` and `Bool`.
