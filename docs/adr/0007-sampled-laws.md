# ADR 0007: sampled laws are tested, not proved

Date: 2026-11-04
Status: Accepted

## Context

The logstat parser's concrete regex equation is rejected because `RegexCaptures`
is outside the normalizer's trusted fragment. Moving the equation to a test
function loses its law declaration. Admitting externs to the prover would weaken
the proof boundary.

## Decision

Use `sample(N)` as the whole body of an existing `proof`, not a new declaration.
`check` validates the equation and sampling requirements without normalization;
`test` executes generated property tests. Sampling is never proof evidence and
sampled laws cannot be rewrite lemmas.

## Rules

- `N` is an Int literal in 1..100000.
- At least one dependency must be outside the law-eligible fragment. Otherwise
  reject sampling and require a proof instead. The trusted fragment is unchanged.
- `check` reports each accepted sampled law as `tested by go4t test (sample N),
  not proved`; this notice does not change its exit status.
- Quantified values use bounded constructor-recursive generators, including
  primitive edge cases, empty and short lists and strings, and maps built through
  `mapFromList`. Function, IO and handle values have no generator.
- The result requires `Eq`; Float results compare bits, preserving NaN payloads
  and signed zero. Binding displays prefer custom `Debug` when available and
  otherwise use structural output; input types do not require `Debug`.
- `test` evaluates both sides for N reproducible random instantiations and reports
  the first counterexample with the bindings. A failed sample fails the test.
  `GO4T_SAMPLE_SEED` overrides the deterministic default Int seed; failures
  include the seed and sample index.
- IO observations remain IO; sampling does not execute an IO-valued equation or
  make a stateful operation pure.

## Consequences

Equations involving pure externs can stay beside proved laws, with an explicit
weaker guarantee. Bounded sampling can miss counterexamples and cannot replace
induction. There is no normalization or termination certificate for sampled laws;
execution has the same divergence and failure risks as ordinary tests.

## Follow-ups

Measure generator coverage on ordinary programs before adding shrinking or
user-defined generators. These are not part of this decision.
