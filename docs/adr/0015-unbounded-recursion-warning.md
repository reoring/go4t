# ADR 0015: Warn on non-tail self-recursion that no data structure bounds

Status: Accepted (design); implementation notes are appended below.

## Context

Tail self-calls compile to loops. Any other recursion uses the Go stack, and
exhausting it is a Go *fatal error*: it is not a panic, `io.tryIO` cannot
turn it into `io.Panic`, and in a controller it kills the process.

Not all non-tail recursion is equally dangerous. A function that recurses on
a structural subterm of its argument (`map` over a list, a tree fold) has a
depth bounded by data that already had to be allocated. A function that
recurses on arithmetic (`n + sumTo(n - 1)`) has a depth bounded by nothing:
`sumTo(100000000)` dies with `fatal error: stack overflow`.

A warning on every non-tail recursive function would fire on the ordinary
functional idiom and be ignored.

## Decision

1. `go4t check` (and every command that type-checks) emits a warning for a
   function that calls **itself** in a **non-tail** position when **no
   parameter decreases structurally** at that call. A call is bounded when
   **any argument** is a pattern-bound strict structural subterm of its
   corresponding parameter; preceding arguments need not remain unchanged.
   Unlike the law termination check's lexicographic condition, this warning
   concerns recursion depth bounded by data, not a termination proof:

   ```text
   file:line:col: warning: sumTo recurses in a non-tail position on a non-structural argument; depth is unbounded and stack exhaustion cannot be recovered. Use an accumulator so the call is a tail call
   ```

   The position is the recursive call. One warning per function. Mutual
   recursion, local recursive closures and recursion through higher-order
   arguments are not analysed and never warn.
2. Warnings do not fail `check`; they follow the existing warning channel
   (`types.Program.Warnings`, `file:line:col: warning: …`).
3. The standard library must not trigger it: `std/**` has zero hits, enforced
   by a test.
4. Every shipped `std/list` function runs on a 1,000,000-element list without
   stack exhaustion; this is a test, so a future structural rewrite of a
   library loop is caught.

## Consequences

- SPEC §5.2 lists the warning; §9 keeps "mutual or non-tail recursion can
  still consume stack" but names the warning.
- Examples and the self-hosted compiler that trigger the warning are either
  rewritten with an accumulator or left as is; the implementation notes
  record the count per directory.

## Implementation notes

- The pass runs at the end of `types.Check` on checked top-level function
  bodies and appends positioned, source-named diagnostics to `Program.Warnings`.
  Existing CLI warning rendering and LSP severity-2 publication need no new
  channel. Local functions, lambda bodies and indirect calls are not scanned.
- `types.TailCalls` is shared with code generation, including dictionary
  passthrough checks and direct IO `do` result positions. Calls in initializers,
  arguments, guards and non-final IO statements remain calls. Dictionary-changing
  recursion is not treated as a loop merely because it is a syntactic result.
- Pattern provenance is shared with law termination analysis through
  `types.BindStructuralPattern`. Tuple reconstruction retains each component's
  provenance; it does not invent descent. The warning accepts descent in any
  corresponding argument; law termination retains its stricter lexicographic
  rule, recursive-SCC checks and dictionary analysis.
- Standard-library sources and inference snapshots are unchanged. A test imports
  every embedded standard module and checks for zero recursion warnings.
  A single generated program exercises all 57 exported list-taking functions
  (including public accumulator helpers and Foldable functions instantiated
  with List) on one-million-element inputs.
  The measured compile-and-run time was 5.63 seconds, so the test also runs under
  `testing.Short()`.

### Corpus measurement

The analysis was run on all example module directories and the self-hosted
compiler, including each of its seven development drivers combined with the
compiler declarations. Diagnostics are deduplicated by source call position.
Modules without an entry point received an in-memory `main`; directories
containing independent programs or negative fixtures were checked file by file.
No example or self-hosted source was changed.

| Directory | Warning count |
| --- | ---: |
| `examples/calc` | 0 |
| `examples/parser` | 1 |
| Other type-checkable `examples/**` sources | 0 |
| `selfhost` | 35 |
| `selfhost/dev` | 1 |
| **Examples total / selfhost total** | **1 / 36** |

The remaining example hit is `parser.chainRest` (line 75). It already carries
an accumulator but recurses through a custom Parser monad, not a direct IO loop.
There is no trivial accumulator-only rewrite; removing the call would require
changing the parser-combinator presentation. The calculator's environment-changing
calls are exempt because their expression arguments descend structurally.

Selfhost hits, deliberately left unchanged:

- `base.gt`: `range`.
- `cg_expr.gt`: `cgInstanceDict`.
- `decls.gt`: `declsSuperclassChain`.
- `derive_higher.gt`: `deriveHigherValue`.
- `infer.gt`: `infCovers`.
- `modules.gt`: `moduleExpr`, `moduleBlock`, `moduleDo`.
- `parser.gt`: `prSkipSemis`, `prSeparated1`, `prDelimited`, `prMembers`,
  `prImports`, `prDecls`, `prHeadParams`, `prNamedFields`, `prType`,
  `prBlockRest`, `prIf`, `prDoRest`, `prArmSeparators`, `prArms`,
  `prPattern`, `prRecordPatternFields`, `prBinaryRest`, `prUnary`, `prPostfix`,
  `prListPattern`, `prTacticArms`.
- `types.gt`: `zonk`, `typesPrune`, `typesNormalize`, `typesOccurs`,
  `typesUnify`, `resolveSTypeVisited`.
- `dev/parser_main.gt`: `parserShape`.

Seven example fixtures cannot reach the post-typechecking analysis:
`diagnostics/bad_alias_mismatch.gt`, `bad_hint.gt`, `bad_missing_ctor.gt`,
`bad_missing_literal.gt`, `bad_missing_nested.gt`, and `bad_missing_tuple.gt`
are rejected for their intended type/coverage errors; `fmt/before.gt` is
rejected for `unknown identifier pure`. These are not counted as zero-warning
successful checks.

The decision's initial lexicographic wording was corrected during implementation:
the warning accepts any corresponding strict subterm, while proof termination
does not. The initial `take` accumulator experiment was discarded; neither
standard-library sources nor inference snapshots changed. There are no deviations
from the revised decision.
