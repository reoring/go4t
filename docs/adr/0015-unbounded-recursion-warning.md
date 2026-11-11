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
   parameter decreases structurally** at that call, using the same structural
   descent rule the law termination check uses (a pattern-bound subterm of
   the parameter, preceding arguments unchanged):

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
