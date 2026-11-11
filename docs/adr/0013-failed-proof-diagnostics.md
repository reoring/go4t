# ADR 0013: A failed `refl` says whether the law is false, in source spelling

Status: Accepted (design); implementation notes are appended below.

## Context

A failed `refl` prints two stuck normal forms. It cannot tell the author the
one thing they need first: is the *law* false, or is the *proof* too weak?
Only `sample` failures print bindings. The printed terms also use internal
spellings: `a#1` for the proof variable `a`, `(-1 * at#2) + now#3` for
`now - at`, and canonically oriented comparisons that no longer look like the
source.

## Decision

### 1. Counterexample search inside `go4t check`

When `refl` fails, the checker searches for a closed instantiation of the
law's quantified parameters under which the two sides normalize to different
closed normal forms.

- The search runs **in the normalizer**, not by compiling a program: every
  law dependency is already eligible (pure, terminating, whitelisted), so a
  closed term normalizes to a value. `go4t check` stays free of the Go
  toolchain.
- Candidates: a fixed edge-case prefix per type (Int `0, 1, -1, minInt,
  maxInt` and small values; Bool; `""` and short Strings; Unit; every
  constructor to a bounded depth; tuples; lists up to a small length; literal
  maps with constant keys), then pseudo-random candidates from a fixed seed.
  The budget is a constant number of candidates and the existing step fuel
  per candidate; the search is deterministic, so diagnostics are reproducible.
- The candidate must satisfy the law's precondition (ADR 0012) when it has
  one. The search exposes one hook, `admissible(bindings) (bool, error)`,
  that defaults to `true`; the `requires` implementation supplies it.
- A candidate is reported only if both sides reach closed normal forms and
  those differ. Fuel exhaustion or a residual neutral discards the candidate.
- Output, appended after the existing `LHS:`/`RHS:` lines:

  ```text
  counterexample: a = 9223372036854775807
    LHS: false
    RHS: true
  ```

  or, when none is found:

  ```text
  no counterexample in N candidates: the law may hold; this proof needs a case split, rewrite or induction
  ```

  or, when a parameter type has no enumerator (functions cannot occur; opaque
  Float-bearing or handle-bearing data can):

  ```text
  counterexample search skipped: no candidates for <type>
  ```

- A counterexample is a fact about the law, so it is printed for the root law
  parameters, not for the proof branch's local pattern variables.
- The search never turns a failure into a success and never runs for a proof
  that checks.

### 2. Source-faithful term display

`display` is for people; `alphaEqual` and the normal forms are unchanged.

- A variable prints as its source name. The `#n` suffix appears only when two
  distinct variables in the *same printed diagnostic* share a name.
- Int polynomials print positive terms first and use subtraction:
  `now - at`, `2 * a - b + 1`, never `(-1 * at) + now`.
- Comparisons keep their canonical orientation (it is what the normalizer
  decided on) but drop redundant outer parentheses; `stuck on:` lines name the
  neutral exactly as the `LHS:`/`RHS:` lines spell it, so a reader can find
  one inside the other.

## Consequences

- Diagnostic text changes; pinned fixtures and the quoted diagnostics in
  `SPEC.md` §6.5, `docs/laws.md`, `docs/tutorial.md` and `docs/evaluation.md`
  are updated in the same change.
- `docs/laws.md` no longer says a failed `refl` has no counterexample.

## Implementation notes
