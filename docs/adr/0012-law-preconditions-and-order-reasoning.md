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

Implemented in `ast/ast.go`, `ast/walk.go`, `parser/parser.go`,
`format/format.go`, `types/laws.go`, and the `laws` evaluator. `LawDecl.Requires`
is an optional expression; existing expression spans and `ast.Walk` supply
diagnostic/editor coverage without new span accessors. `requires` stays an
identifier. The formatter locates the equation body after the precondition,
including preconditions containing blocks or conditionals.

`laws/requires.go` provides
`func (e *evaluator) preconditionHolds(law lawGoal, env *environment) (bool, error)`
for closed-binding filtering inside the package. It temporarily clears proof
assumptions, so a law's own precondition cannot establish itself. Per integration
direction this is an internal helper, not an unused exported wrapper.
The proof entry point assumes the precondition; rewrite checks instantiate it
without assuming it. Explicit and synthesized induction rewrites share that
check. Dependency/taint scanning includes both the current and rewritten law's
preconditions. Sampling is rejected in type checking.

`laws/order.go` implements the final decision fragment:

- Only trusted prelude Int/String comparisons enter total-order closure.
  The existing runtime equality/less-than term forms are reused; `a <= b`
  is represented as `!(b < a)`. User dictionaries retain their original
  semantics. Graph paths retain strictness; equality is mutual reachability,
  and disequalities strengthen known non-strict edges.
- Int literal bounds are tightened over mathematical integers, including
  strict endpoints without overflowing at minInt/maxInt. Polynomial
  coefficients and differences use `math/big`; the existing modular ring
  normalizer is unchanged. Both operands, their difference, and intermediate
  monomial products must fit int64 before canonicalization. Failed checks
  leave the comparison unchanged.
- Linear decisions match exact sums of zero, one or two assumed nonnegative
  polynomials plus a nonnegative constant. A summand may be reused. Strict
  conclusions require a strict summand or positive constant; reversing the
  difference supplies refutation. Nonlinear polynomials can be range-checked
  and canonicalized but are not inputs to the combination decision.

Soundness for wrapping int64: Layer A uses only the runtime total order on
complete values, never an arithmetic relationship between atoms. Ring normal
forms already denote the runtime value modulo 2^64. A range-checked integer
polynomial is that same residue's unique int64 representative; checking the
exact difference as well makes comparison with zero equivalent to the
original signed comparison. Exact nonnegative addition is then an ordinary
integer implication, with strictness preserved as stated. No modular
coefficient equality is used to justify an ordered implication.

`laws/property_test.go` and `laws/property_proofs_test.go` retain preconditions
and branch assumptions for open sides and partial goals, close those assumptions
without an assumption stack, count rejected substitutions, and fail a target
with no satisfying sample. The existing forced Int prefix includes minInt,
maxInt, -1, 0 and 1 for every generated atom. After that mandatory prefix,
literals from the assumptions are tried as extra seeds; otherwise full-width
randomness cannot exercise branches such as `k == 2`. The `requires_*.gt`
fixtures cover transitivity, bounded successors, wrap-free versus wrapping deadlines, rejected
rewrites, two-summand decisions and elapsed-time TTLs. Focused tests also cover
contextual syntax, walk/spans, formatting, type/taint restrictions, induction
preconditions, negations, antisymmetry and editor diagnostics.

The authorized compatibility-only change in `selfhost/parser.gt` consumes the
optional precondition in its erased-law path. It adds no law AST, checking or
emission there. Bootstrap has an examples-directory comparison loop, not a
dedicated law fixture registry; no selfhost feature extension was added.
`SPEC.md` §6.5/§9 and the English/Japanese law guides describe the implemented
fragment. README and trust-document updates remain with the integrator, outside
this slice's documentation ownership.

Verification: lexer/parser/format/ast/types tests passed, followed by the full
laws suite (`go test -v ./laws -timeout 1200s`, 253.586 s). Its open-term oracle
ran for 252.057 s with 87,436 runtime comparisons and 6,564 rejected substitutions.
`go test ./docs ./lsp`, `go build ./...`, vet on all touched Go packages and
`gofmt -l` passed. The built CLI accepted the three positive `requires_*.gt`
fixtures and rejected the wrapping and missing-precondition fixtures with the
expected normal-form/precondition diagnostics. Selfhost bootstrap is reserved
for the integrator and was not run in this slice.
