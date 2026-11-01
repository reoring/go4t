# Local recursion

Run from the repository root:

```sh
go run ./cmd/gofx run examples/local-recursion/main.gfx
```

Observed output:

```text
sum 1..5: 15
even 10: true, even 7: false
do helper: 13
polymorphic: 42, true
dictionaries: 7, false
```

`sumTo` uses a recursive let-bound `go` loop. `parity` declares consecutive mutually recursive `isEven`/`isOdd` helpers. The `do` block's recursive `addBase` helper captures a preceding monadic binding. Its recursive let-bound helpers are reused at different types, including a generalized `Show` constraint.
