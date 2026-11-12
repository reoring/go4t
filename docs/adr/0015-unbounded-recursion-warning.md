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
   corresponding parameter, or a chain of record-field/tuple-index projections
   rooted at that parameter or a pattern-bound subterm of it; preceding
   arguments need not remain unchanged.
   Unlike the law termination check's lexicographic condition, this warning
   concerns recursion depth bounded by data, not a termination proof:

   ```text
   file:line:col: warning: sumTo calls itself in a non-tail position and no argument is a structural part of its parameter; go4t cannot bound the recursion depth, and stack exhaustion cannot be recovered. Use an accumulator so the call is a tail call
   ```

   The position is the recursive call. One warning per function. Mutual
   recursion, local recursive closures and recursion through higher-order
   arguments are not analysed and never warn.
   In non-IO `do` blocks (including polymorphic monads), scan through the first
   bind or monadic statement's expression, but exclude everything after it as
   a continuation closure; IO `do` retains direct-style scanning.
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
  Non-IO `do` scanning follows Bind desugaring: leading lets are evaluated
  directly, as is the first bound action, but subsequent statements and the
  result belong to the continuation closure. The checked monad dictionary
  selects direct IO; polymorphic monads keep the closure boundary.
- Pattern provenance is shared with law termination analysis through
  `types.BindStructuralPattern`. Tuple reconstruction retains each component's
  provenance; it does not invent descent. The warning accepts descent in any
  corresponding argument; law termination retains its stricter lexicographic
  rule, recursive-SCC checks and dictionary analysis.
  Warning provenance additionally follows record-field and tuple-index
  projections; helper-produced and reconstructed arguments are not certified.
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
| `examples/parser` | 0 |
| Other type-checkable `examples/**` sources | 0 |
| `selfhost` | 5 |
| `selfhost/dev` | 0 |
| **Examples total / selfhost total** | **0 / 5** |

The custom Parser continuation in `examples/parser.chainRest` is excluded,
as are the non-IO continuations in the self-hosted parser and inference code.
`selfhost/dev/parser_main.gt`'s `parserShape(block.result)` is now recognized as
structural projection. No example requires a rewrite.

Remaining selfhost hits, deliberately left unchanged:

- `base.gt:157`, `range`: `Cons(from, range(from + 1, to))` consumes stack
  according to a numeric range, not a previously allocated data structure.
- `cg_expr.gt:84`, `cgInstanceDict`: the recursive call is evaluated inside the
  first bound action, before a continuation exists; `superInst` comes from a
  map lookup and `superArgs` from a helper, so superclass traversal is not
  structurally certified.
- `modules.gt:198`, `moduleExpr`: `EIdent(ep, n)` reconstructs a child before
  the recursive call; this case is bounded but reconstruction is not certified.
- `modules.gt:234`, `moduleBlock`: the recursive call receives a rebuilt
  `block{stmts: rest}` using a helper-produced suffix; the shrinking statement
  traversal is not certified through reconstruction or the helper.
- `modules.gt:254`, `moduleDo`: `rest` comes from `localDoGroup(stmts)`, not
  from pattern descent; this helper-mediated shrinking traversal is not certified.

The diagnostic intentionally says that go4t cannot bound the depth: unlike
numeric `range`, the bounded-but-not-certifiable cases above are not evidence
of actually unbounded recursion.

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
