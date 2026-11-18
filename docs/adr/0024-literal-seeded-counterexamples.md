# ADR 0024: Seed counterexample search from the law and its callees

Status: Accepted. Extends [ADR 0013](0013-failed-proof-diagnostics.md).

## Context

The controller law `writesOnlyWhenSettled` failed to prove after a callee
started treating `"pending"` specially. The diagnostic search could not reach
that branch: its short-string alphabet cannot spell `"pending"`, and the law
has no precondition from which to extract it. Increasing the attempt limit
would not fix that. Collecting literals from every module would let unrelated
declarations change which inputs are tried.

## Decision

Keep the standard candidates and the 128-attempt budget. Add Int and String
literals from the law operands, its precondition and their transitive callees,
using the dependency graph already resolved for eligibility, including instance
methods. Proof-only dependencies are not roots. Keep at most 64 distinct hints
per type in deterministic traversal order, starting with the normalized
precondition's literals and in-range integer neighbors. Bool and Unit already
have complete scalar enumerators. The same pools apply inside composite inputs.

## Consequences

The mutated controller produced `t = "pending", h = true` with a nonempty
`c` distinct from `t`: the evaluated sides were `Some(c)` and `None`. Its
unmutated law still checks. The search remains incomplete: not every literal
or combination is tried. A hint cannot prove or refute an equation. Only an
admitted, evaluated binding with unequal closed results is reported, and a
failed proof stays failed regardless of the search result. No normalization
rule, termination certificate or proof tactic changes.
