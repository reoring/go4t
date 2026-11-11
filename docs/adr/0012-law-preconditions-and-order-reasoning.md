# ADR 0012: Law preconditions (`requires`) and sound Int order reasoning

Status: Accepted (design); implementation notes are appended below.

## Context

ADR 0001 left "order reasoning across comparisons" open because linear
arithmetic is unsound under wrapping `Int` (`a + 1 > a` is false at
`maxInt`). The consequence is visible in the README's own headline law:
`isStale(freshness(TtlNanos(t), AtNanos(at), now)) == (now >= at + t)` holds
*including* the inputs where `at + t` wraps, so it does not say "stale once
the TTL has elapsed". The invariants a decision core most wants — deadlines,
TTLs, quotas, monotonicity, transitivity — are exactly the ones that need an
assumption ("these quantities are in range") and an order decision.

A separate `Nat`/bounded-Int type was considered and rejected for now: it
changes the Go ABI (`--emit-go` signatures), needs coercions the language
deliberately lacks, and still requires the same decision procedure. A
precondition on the law states the same fact without touching runtime types.

## Decision

### 1. `requires` on a law

```go4t
law staleAfterTtl(t Int, at Int, now Int)
  requires 0 <= t && t <= 1000000000000 && 0 <= at && at <= now && now <= 4000000000000000000 {
  isStale(freshness(TtlNanos(t), AtNanos(at), now)) == (now - at >= t)
}
```

- Grammar: `LawDecl := 'law' Ident '(' Params ')' ['requires' Expr] '{' Expr '==' Expr '}'`.
  `requires` is a contextual identifier in exactly that position, like `sample`
  and `induction`; it is not a reserved word.
- The expression must have type `Bool` and obeys every restriction a law side
  obeys (eligibility/taint, no Float, first-order).
- Meaning: the equation is claimed only for arguments that satisfy the
  precondition. The proof starts with the precondition assumed `true`. A
  top-level `&&` conjunction is split so each conjunct is its own assumption.
- `rewrite name(args)` with a law that has a precondition is accepted only if
  the instantiated precondition normalizes to `true` under the current
  assumptions; otherwise the rewrite fails with
  `rewrite %s: precondition not established: <normal form>`.
  The induction-hypothesis rewrite follows the same rule.
- `sample(N)` on a law with `requires` is rejected in this version
  (`sample does not support requires`); filtering generators is a separate
  decision.
- Proved laws still generate no runtime code; `requires` is never checked at
  run time.

### 2. Order reasoning that is sound for wrapping Int

Assumptions come from `requires` and from `if` tactic branches / stuck-`if`
arms (the existing mechanism). Two layers decide a stuck Int comparison
(`<`, `<=`, `==` after the existing canonical orientation). Neither changes
what a comparison means; each only *decides* one whose truth follows.

**Layer A — total order over opaque atoms (no range assumption).** Every
distinct normalized Int term is an atom of the int64 total order, and Int
literals are atoms with their known order. Assumed facts `a < b`, `a <= b`,
`a == b` and their negations form a graph; the decision uses reflexivity,
antisymmetry, transitivity (mixing strict and non-strict edges) and totality
(`!(a < b)` is `b <= a`). This is sound regardless of overflow because it
never relates an atom to its own arithmetic: `a + 1` and `a` are unrelated
atoms. It gives `a < b && b < c ⇒ a < c` and constant bounds by transitivity
(`at <= now`, `now <= H` ⇒ `at <= H`).

**Layer B — wrap-free linear canonical form (needs proven ranges).** Each
atom has an interval over the mathematical integers: `[minInt, maxInt]` by
default, tightened by Layer A's bounds against literals. Interval arithmetic
over ℤ (arbitrary precision) bounds any ring polynomial. For a comparison
`p ⋈ q`, if the ℤ-intervals of `p`, `q` *and* `q - p` (as polynomials, every
intermediate monomial included) lie inside `[minInt, maxInt]`, none of them
can wrap, the int64 values equal the ℤ values, and the comparison is rewritten
to the canonical form `0 ⋈ (q - p)` with `q - p` in ring normal form. Then:

- a constant polynomial decides the comparison (`a < a + 1` becomes `0 < 1`);
- two comparisons with the same ℤ meaning share a normal form
  (`now >= at + t` and `now - at >= t` both become `0 <= now - at - t`);
- a canonical goal `0 <= g` / `0 < g` is decided `true` when `g` equals an
  assumed canonical nonnegative, or the sum of at most two assumed canonical
  nonnegatives, plus a nonnegative constant (strict when any summand is
  strict or the constant is positive); symmetric for refutation. This is a
  bounded, exact fragment of Fourier–Motzkin over ℤ, not a complete decision
  procedure.

If any interval check fails the comparison is left exactly as it is today.
The interval of the difference is what keeps this sound: a term that could
wrap is never interpreted over ℤ.

### 3. What stays out

No `/`, `%`, or nonlinear decision; no automatic case split; no quantifier
instantiation; no String order reasoning beyond Layer A's equality/total-order
closure. Comparisons that involve Float remain excluded as before.

## Soundness evidence required of the implementation

- The open-term property test (`laws/property_test.go`) is extended: for laws
  with assumptions, random substitutions that satisfy every assumption are
  compared with runtime evaluation for both sides and every partial goal;
  substitutions that violate an assumption are skipped and counted, and a
  fixture with no satisfying substitution fails the test.
- Negative fixtures: `a + 1 > a` without a bound stays unprovable; with
  `requires a < 9223372036854775807` it proves; a `requires` whose range
  allows a wrap leaves `now >= at + t` and `now - at >= t` distinct.
- Edge constants `minInt`, `maxInt`, `-1`, `0`, `1` appear in the forced
  prefix of the property generator for every Int atom.

## Consequences

- `SPEC.md` §6.5 gains the `requires` grammar and the two decision layers;
  §9 drops "no `requires` clause" and narrows "ordered arithmetic" to the
  fragment above.
- The README headline law is replaced by one that states the elapsed-time
  property under an explicit range.
- The trusted base grows by the interval analysis and the canonical-form
  rewrite; `docs/laws-trust.md` lists them.

## Implementation notes
