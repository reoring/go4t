# What "proved" rests on

`go4t check` reports a law as proved when its two sides reach the same normal
form in every branch of its proof. That statement is only as good as the code
that computes normal forms. This page lists everything that has to be right
for a proved law to be true at run time, and what checks each piece. It is the
trusted base of the law checker; `SPEC.md` §6.5 states the rules, this page
states the assumptions.

A law is a claim about the **generated Go program**: for all values of its
parameters (that satisfy its `requires` clause, if it has one), evaluating the
left side and the right side at run time yields the same value.

## 1. What is trusted

| # | Component | What it must get right | Where |
| --- | --- | --- | --- |
| 1 | Normalizer | Every reduction step preserves the run-time value of an open term: unfolding, beta, `let`, `match` selection, record projection and update merging, resolved class methods, branch assumptions | `laws/eval.go`, `laws/normal.go`, `laws/resume.go`, `laws/patterns.go` |
| 2 | Int ring | `+`, `-`, `*`, negation as polynomials modulo 2^64 — the same ring as wrapping `int64` | `laws/ring.go` |
| 3 | Order decisions | A comparison is decided only by the total-order closure of assumed facts, or after it is shown wrap-free by interval analysis over the integers ([ADR 0012](adr/0012-law-preconditions-and-order-reasoning.md)) | `laws/order.go`, `laws/requires.go` |
| 4 | Termination check | Every function a law can reach, and every self-`rewrite`, decreases a pattern-bound structural subterm; law-to-law `rewrite` cycles are rejected | `laws/termination.go` |
| 5 | Taint analysis | No law reaches `IO`, an untrusted `extern`, `Float` arithmetic, or `/` `%` with a non-literal divisor | `laws/termination.go` |
| 6 | Extern whitelist | The 10 prelude primitives and 32 `std/string` primitives are pure, total, and are folded by calling the real `rt` implementation, only when every argument is a constant | `laws/primitives.go`, `laws/strings.go` |
| 7 | `std/map` model | The 13 map externs behave as the symbolic model says, for comparators that identify `compare(a, b) == EQ` with `a == b` (prelude Int/String/Bool, derived `Ord`, tuples of those) | `laws/mapterm.go`, [ADR 0002](adr/0002-laws-map-and-diagnostics.md) |
| 8 | `std/list` models | The structural equations used instead of unfolding 14 accumulator loops (`length`, `sum`, `product`, `count`, `reverse`, `zipWith`, `unzip`, `partition`, `flatten`, `intercalate`, `init`, `span`, `findIndex`, `indexed`) equal the run-time result for every input, Int wraparound included ([ADR 0014](adr/0014-structural-models-for-std-list.md)) | `laws/listterm.go` |
| 9 | Tactics | `match`/`induction` cases are exhaustive; `rewrite` instantiates a proved law, checks its precondition, and replaces only alpha-equal occurrences; `if` assumes exactly the condition and its negation | `laws/laws.go`, `laws/rewrite.go`, `types/laws.go` |
| 10 | Front end and code generator | The term the normalizer reasons about is the program that runs: parsing, type-class resolution, deriving expansion and Go emission agree | `parser/`, `types/`, `codegen/`, `rt/` |

Trust in rows 6–8 is tied to the **owning standard-library declaration**, not
to a name: a user function called `length`, or a user `extern` bound to the
same Go symbol, gets none of it.

## 2. What is not trusted

- **Proofs.** A proof script is checked, never believed; a wrong proof fails.
- **`sample(N)` laws.** They are tested by `go4t test`, reported as *tested,
  not proved*, and cannot be used by `rewrite`.
- **The counterexample search** ([ADR 0013](adr/0013-failed-proof-diagnostics.md)).
  It runs only after a proof has already failed and only adds text to the
  diagnostic; it cannot turn a failure into a success.
- **User `Eq` instances.** Law equality is definitional equality of normal
  forms, not a call to `==`.
- **Fuel.** Running out of normalization steps is an error, never a proof.

## 3. What a proved law does not say

- Nothing about `IO`, externs outside the whitelist, `Float`, `std/set`,
  `std/array`, or string contents beyond constant folding.
- Nothing about arguments outside its `requires` clause; the precondition is
  not checked at run time. A precondition that provably cannot hold is
  rejected, and one for which the checker finds no satisfying argument is
  flagged with a warning, but that search is bounded: a law can still be
  vacuous without either ([ADR 0016](adr/0016-requires-satisfiability.md)).
- Nothing about resources: allocation failure and stack exhaustion are outside
  the model.
- A trusted `extern` with a pure type can still perform effects in Go; the
  checker cannot inspect foreign code.

## 4. How the trusted base is checked

**The open-term property test** (`laws/property_test.go`,
`TestOpenTermProperty`) is the main defence for rows 1–3 and 6–8. For every
law in `laws/testdata/*.gt` — including deliberately false ones, whose two
sides must each still be preserved — it takes every side and every partial
proof goal, normalizes it **with free variables first**, then substitutes
random values into both the original term and its normal form and compares
the two by running the real compiled program. Substituting before normalizing
would miss exactly the open-term bugs it exists to find.

- It runs in CI on every push, as part of `go test ./...`, on the minimum
  and the stable Go version; it skips only when no Go toolchain is on `PATH`.
- It is deterministic: generator `rand.NewPCG(2119, 4)`, 50 substitutions per
  target, constructor depth 4, with a forced prefix of scalar edge values
  (`0`, `±1`, `minInt`, `maxInt`, empty strings).
- For goals that carry assumptions (`requires`, `if` branches), only
  substitutions that satisfy every assumption are compared; a fixture for
  which none does fails the test.
- Its last log line reports the totals for the run; see
  [§5](#5-current-numbers).

**Fixtures** pin each rule from both sides: a law that must prove, and a
neighbouring false law that must not (`laws/testdata/*_bad*.gt`,
`rejected.gt`). The taint boundary is pinned by a generated snapshot of every
standard-library declaration and its category
([docs/laws-std-reach.md](laws-std-reach.md), `laws/taint_reach_test.go`).

**Fuzzing** covers row 10's front half: the lexer and parser never panic, and
`format` is idempotent and preserves the parsed program (`FuzzLexer`,
`FuzzParser`, `FuzzFormat`; CI fuzzes each for a short fixed time).

**Ports** cover the rest of row 10: two production controllers were moved with
their original Go tests byte-identical as the oracle
([docs/evaluation.md](evaluation.md)).

## 5. Current numbers

From `go test -v ./laws` on the commit that introduced this page (Go 1.26.1):

| | |
| --- | ---: |
| Laws in `laws/testdata` | 298 |
| Law sides and partial proof goals normalized | 2,176 |
| Substitutions per target | 50 |
| Normal form vs. runtime comparisons | 102,116 |
| Substitutions skipped for violating an assumption | 6,684 |
| Float-bearing substitutions | 1,242 |
| Fixtures that are intentionally not normalizable | 9 |
| Mismatches | 0 |

The same run passes on Go 1.22. The test takes about six minutes on a
developer machine; it is the reason `go test ./laws` is slow, and that cost is
deliberate.

Before the order-reasoning rules were merged they were also attacked once by
an independent review with hand-written false laws and differential fuzzing
against wrapping int64 semantics; the result and its limits are recorded in
[ADR 0012](adr/0012-law-preconditions-and-order-reasoning.md#review-evidence-2026-11-11).
