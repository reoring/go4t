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

Proved laws use definitional equality, independent of `Eq` instances. Proofs
combine `refl`, constructor `match`, guard `if`, `rewrite` and `induction`.
Only eligible pure, terminating dependencies are admitted to proofs; untrusted
externs are excluded. Float expressions are excluded from proofs, but data types that merely
contain Float (such as `dyn.Value`) are allowed as quantified values. Quantities you
want laws about (money, counts, totals) therefore belong in `Int` units such as
cents; `Float` is for measurements and formatting, and Float sums are genuinely
order-sensitive, so no law could hold for them anyway. `Map` is modelled symbolically, so
`mapLookup(mapInsert(m, k, v), k2)` reduces to a key-equality case split.
Nested updates over a symbolic single-constructor record merge into one update
with later writes winning and fields ordered by declaration index, so
`r{f: a}{f: b}` and `r{f: b}`, and `r{g: c, f: b}` and `r{f: b, g: c}`, share
normal forms; multi-constructor record updates are not merged.
Proved laws generate no runtime code. See
[tutorial §13](tutorial.md#13-laws) for cancellation, the `freshness`
`staleIff` guard law and list induction, and `laws/testdata/placement.gt`,
`refactor_v2.gt` for a Map-based core and a refactoring-equivalence proof.

Parenthesize a conditional used as either equation operand:
`(if c { a } else { b }) == x` or `x == (if c { a } else { b })`.
Without parentheses, the left-hand form is rejected with
`law body must be an equality (lhs == rhs)`; the right-hand form fails with
`expected expression, found "if"`.
A failed `refl` shows the two stuck normal forms and the failing proof branch,
then searches for a counterexample using the original law parameter names.
The normalizer tries 64 edge-case candidates and 64 fixed-seed random candidates,
each bounded by its step limit; no Go toolchain is needed. For example:

```text
counterexample: a = 9223372036854775807
  LHS: false
  RHS: true
```

This witness refutes `(a + 1 > a) == true`: Int addition wraps. If no candidate
refutes the law, the diagnostic says
`no counterexample in 128 candidates: the law may hold; this proof needs a case split, rewrite or induction`.
That is not proof that the law holds. If a parameter has no generator, it says
`counterexample search skipped: no candidates for <type>`. Float-bearing data,
handles, unsupported map keys and types with no bounded inhabitant are skipped.
Only unequal closed data normal forms produce a witness; residual neutral terms
and fuel exhaustion are discarded. Passing proofs never run the search.

Diagnostic terms use source variable names, adding `#n` only for a real name
collision within the diagnostic. Polynomials print as `now - at` or
`2 * a - b + 1`. Comparisons keep their canonical orientation but omit redundant
outer parentheses; `stuck on:` uses the same spelling as `LHS:` and `RHS:`.

What laws cover: first-order data (ADTs, records, `List`, `Option`, `Result`,
`Map`), "every other field is arbitrary" invariants via `o{field: v}`, gate
order, early return, idempotence, branch coverage, structural induction
(including std `filter`/`head`/`append` and the owner-checked structural models
for `length`, `sum`, `product`, `count`, `reverse`, `zipWith`, `unzip`,
`partition`, `flatten`, `intercalate`, `init`, `span`, `findIndex`, `indexed`),
equivalence of two `decide`s, and conditional equations with `requires`.
What they do not: `Set`/`Array`, reasoning about string contents, unrestricted
ordered arithmetic, automatic accumulator generalization for other folds, and
equality of two symbolic maps. Proof length grows with the number of branches; that is the
specification's complexity, not the prover's.

## Preconditions and order

```go4t
package main
law successor(a Int) requires a < 9223372036854775807 {
  (a + 1 > a) == true
}
proof successor(a) { refl }
func main() IO[Unit] { ioPure(unit()) }
```

`requires` is contextual, not a reserved word. Its Bool expression obeys the
same purity, termination, Float and first-order restrictions as the law sides.
The proof assumes it true, splitting top-level `&&` conjuncts. Rewriting a
conditional law (including an induction hypothesis) requires its instantiated
precondition to normalize to `true`. Preconditions generate no runtime checks,
and `sample` on a law with `requires` is rejected.

Total-order closure treats normalized Int/String terms as opaque atoms:
`a < b && b < c` establishes `a < c`, even if those terms contain wrapping
arithmetic. It does not relate `a + 1` to `a`.

For Int arithmetic, literal bounds derived from those facts feed
arbitrary-precision interval analysis. Only if both operands, their exact
polynomial difference and intermediate monomials fit int64 does a comparison
become `0 ⋈ (q - p)`. Constant differences decide; a linear goal may also be
one or two assumed nonnegative polynomials plus a nonnegative constant.
Strictness requires a strict summand or a positive constant; the symmetric
rule refutes comparisons. This is a bounded fragment, not a general solver:
no nonlinear decision, `/`/`%` reasoning, or String arithmetic.
Without a wrap-free range, `a + 1 > a` stays unproved and
`now >= at + t` need not equal `now - at >= t`. See
[`requires_ttl.gt`](../laws/testdata/requires_ttl.gt) for an elapsed-time TTL law.

## Structural induction

`induction xs` splits a quantified data value into every constructor case.
Each field of the same instantiated data type supplies a self-rewrite on that
structurally smaller argument; the remaining law arguments stay unchanged.
The bare form finishes each arm with `refl`. Explicit arms let a proof continue:

```text
proof appNil(xs) {
  induction xs {
    Nil => refl
    Cons(head, tail) => refl
  }
}
```

This is sugar for constructor `match` plus the existing structural `rewrite`,
not stronger recursion reasoning. The same descent, argument-order and
left-to-right rewrite checks apply. Library `length` now normalizes structurally,
so `length(append(xs, ys)) == length(xs) + length(ys)` proves directly by
`induction xs`. The models replace runtime unfolding only for declarations
owned by `std/list`; runtime implementations and callback eligibility are
unchanged. Other accumulator lemmas still need explicit generalization and
rewrites. [ADR 0014](adr/0014-structural-models-for-std-list.md) records the
equations, wraparound reasoning and exclusions.

## Sampled laws: tested, not proved

An equation using a pure extern outside the prover's trusted fragment can keep
its law declaration and use `proof name(args) { sample(N) }`. `N` is an Int
literal in 1..100000; `sample` must be the whole proof body. `go4t check`
type-checks it and validates its generators without normalization, then prints:

```text
file.gt:line:column: law name: tested by go4t test (sample N), not proved
```

The notice does not fail the check. `go4t test` runs the sampled laws, including
imported ones, alongside ordinary test functions. It generates N reproducible
random assignments, with a fixed edge-case prefix, and reports the first
counterexample using each binding's custom `Debug` when available, otherwise a
structural display. Set `GO4T_SAMPLE_SEED` to an Int to change the random seed;
a counterexample includes its seed and sample index.

Generators support Int extremes, Float NaN/signed-zero/infinities and random
bits, Bool, Unit, empty and short strings, tuples and constructor-recursive data
including lists, and maps built through `std/map.mapFromList`. Constructor depth
is bounded at four. Every parameter requires a usable generator;
map keys require `Ord`. Functions, IO values, handles and types with no
bounded inhabitant are rejected. The result requires `Eq`; a direct Float
result compares bits instead, so equal NaN payloads agree and signed zeros differ.

Sampling an equation whose dependencies are all law-eligible is rejected:
prove it instead. A sampled law cannot be used by `rewrite`. Sampling neither
widens the trusted fragment nor proves termination; divergent or failing
externs have ordinary test execution risks. `build`, `run` and `eval` do not run
samples. See [ADR 0007](adr/0007-sampled-laws.md).
