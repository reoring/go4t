# Laws: scope and limits

[日本語](laws.ja.md)

## What a law is

State an equation once and have `go4t check` verify it for every input, rather
than only selected test cases:

```go4t
package main
law distribute(a Int, b Int, c Int) { (a + b) * c == a * c + b * c }
proof distribute(a, b, c) { refl }
func main() IO[Unit] { ioPure(unit()) }
```

Laws use definitional equality, independent of `Eq` instances. Proofs combine
`refl`, constructor `match`, guard `if` and `rewrite` (including structural
induction). Only eligible pure, terminating dependencies are admitted; untrusted
externs are excluded. Float expressions are excluded, but data types that merely
contain Float (such as `dyn.Value`) are allowed as quantified values. `Map` is modelled symbolically, so
`mapLookup(mapInsert(m, k, v), k2)` reduces to a key-equality case split.
Nested updates over a symbolic single-constructor record merge into one update
with later writes winning and fields ordered by declaration index, so
`r{f: a}{f: b}` and `r{f: b}`, and `r{g: c, f: b}` and `r{f: b, g: c}`, share
normal forms; multi-constructor record updates are not merged.
Laws generate no runtime code. See
[tutorial §13](tutorial.md#13-laws) for cancellation, the `freshness`
`staleIff` guard law and list induction, and `laws/testdata/placement.gt`,
`refactor_v2.gt` for a Map-based core and a refactoring-equivalence proof.

What laws cover: first-order data (ADTs, records, `List`, `Option`, `Result`,
`Map`), "every other field is arbitrary" invariants via `o{field: v}`, gate
order, early return, idempotence, branch coverage, structural induction
(including std `filter`/`head`/`append`), and equivalence of two `decide`s.
What they do not: `Set`/`Array`, reasoning about string contents, order
reasoning across comparisons (`a < b && b < c ⇒ a < c`), preconditions
(`requires`), induction through fold-based std functions, and equality of two
symbolic maps. Proof length grows with the number of branches; that is the
specification's complexity, not the prover's.
