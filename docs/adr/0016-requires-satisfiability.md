# ADR 0016: A `requires` clause must be satisfiable, and the checker says when it cannot tell

Status: Accepted (design); implementation notes are appended below.

## Context

ADR 0012 added `requires`. A precondition nobody can satisfy makes its law say
nothing, and today the checker handles that badly in both directions:

- `law v(a Int, b Int) requires a < b && b < a { (a == b) == true }` is
  **accepted**: the contradictory assumptions decide the goal. The law is
  vacuously true and almost certainly a typo.
- `law v(a Int) requires a < 5 && a > 7 { (a < 0) == true }` is rejected with
  `no counterexample in 128 candidates: the law may hold; this proof needs a
  case split, rewrite or induction`. That advice is wrong: no candidate was
  ever admitted, because none satisfies the precondition.

Writing the range once and reusing it already works and needs no new syntax:
`requires inRange(t, at, now)` with `func inRange(...) Bool { 0 <= t && … }`
unfolds, and its conjuncts become assumptions like inline ones.

## Decision

### 1. A provably unsatisfiable precondition is an error

When the proof of a law with `requires` starts, the normalized precondition is
split into its top-level conjuncts (the split ADR 0012 already performs) and
they are assumed **in order**. Before a conjunct is assumed it is normalized
under the conjuncts already assumed. If that normal form is `false`, the law
is rejected:

```text
law <name>: requires can never hold: <conjunct> is false given the conditions before it
```

or, when the first conjunct (or the whole precondition) is `false` on its own:

```text
law <name>: requires can never hold: <conjunct> is false
```

The position is the `requires` expression. `<conjunct>` is printed in source
spelling (ADR 0013). This is sound: it uses only decisions the normalizer
already makes, and rejecting a law is always sound. It is not complete —
`0 <= a && a <= 10 && a * 2 == 21` is unsatisfiable and is not detected here.

The check runs once per law, before any tactic. It does not run for
assumptions introduced by `if`/`match` tactics: a contradictory *branch* is
ordinary and is how impossible cases are discharged.

### 2. A witness search reports what it could not find

The counterexample search (ADR 0013) already enumerates candidates and filters
them by the precondition. The same enumeration is used to look for one
**witness**: a candidate for which the precondition normalizes to `true`.

- The candidate prefix for a law with `requires` is extended with every Int
  and String literal that occurs in the normalized precondition, and for Int
  literals also the literal ± 1. A precondition such as `a == 123456` or
  `1000 <= a && a < 2000` then has a witness in the prefix.
- **Failed proof, no witness.** The last line of a failed `refl` becomes

  ```text
  no arguments satisfying requires were found in N candidates: no counterexample could be tried
  ```

  instead of the `no counterexample in N candidates: the law may hold…` line,
  which is kept for the case where at least one candidate was admitted.
- **Successful proof, no witness.** The law is accepted and a warning is
  reported at the `requires` expression:

  ```text
  warning: law <name>: no arguments satisfying requires were found in N candidates; the law may say nothing
  ```

  It is a warning, not an error, because the search is a bounded sample: a
  narrow but satisfiable precondition can be missed.
- The witness search runs only for laws that have `requires`, is deterministic
  (same fixed seed as ADR 0013), and never changes whether a law is accepted.

### 3. Warnings from the law checker

`laws.CheckProgram` gains a way to return non-fatal diagnostics. They use the
existing warning channel and format (`file:line:col: warning: …`), are printed
by every command that checks laws, do not change the exit status, and are
published by the language server as warnings.

## Consequences

- `SPEC.md` §6.5 lists the new error and warning templates and states the
  conjunct-order rule; §9 says the unsatisfiability check is incomplete.
- `docs/laws.md` documents reuse of a range through a `Bool` function and the
  two new diagnostics.
- The review note in ADR 0012 ("an accidentally unsatisfiable `requires` is
  not reported") is closed for the provable case and narrowed to a warning for
  the rest.

## Implementation notes
