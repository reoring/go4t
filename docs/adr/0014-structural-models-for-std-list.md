# ADR 0014: Accumulator-based `std/list` functions get structural models in laws

Status: Accepted (design); implementation notes are appended below.

## Context

`std/list` implements `length`, `sum`, `product`, `count`, `reverse` and
others as tail-recursive accumulator loops, because the runtime must not grow
the stack with the list. The normalizer unfolds what is written: 
`length(Cons(x, r))` becomes `fold(r, 1, f)`, never `1 + length(r)`, so the
induction hypothesis `length(r)` does not apply. Authors work around it by
redefining a structural `count` in their own module (docs/evaluation.md), and
SPEC §9 lists "induction through fold-based std functions" as a limitation.

Options:

1. Rewrite the std functions structurally. Rejected: non-tail recursion costs
   stack proportional to the list, the opposite of what a runtime library
   should do.
2. Ship proved lemmas in std and `rewrite` with them. Rejected for now: laws
   quantify over monomorphic parameters, so a lemma about `length[A]` cannot
   be stated; polymorphic laws are a larger decision.
3. Model the functions in the normalizer, as ADR 0002 does for `std/map`.

## Decision

Option 3. For a fixed, owner-checked set of `std/list` declarations the
normalizer uses a *structural model* instead of unfolding the accumulator
loop, when the list argument is headed by a constructor or is neutral:

| Function | Model |
| --- | --- |
| `length(Nil)` | `0` |
| `length(Cons(x, r))` | `1 + length(r)` |

The same treatment applies to every other `std/list` function for which
(a) the shipped definition is an accumulator loop that blocks structural
induction and (b) a structural equation is *exactly* equal to the runtime
result for all inputs, including Int wraparound (the ring is commutative and
associative modulo 2^64, so `sum`, `length` and `count` qualify; anything
whose equation depends on evaluation order of effects or on a non-associative
callback does not). The implementation section lists the final set and, for
each function left out, the reason.

Rules:

- Trust is tied to the owning `std/list` declaration (as with the extern
  whitelist), never to a same-named user function.
- The model replaces unfolding for these functions; there is no second,
  competing normal form.
- Each modelled equation is covered by the open-term property test against
  the real runtime, with random lists including `Nil` and long lists, and with
  Int edge values where the function does arithmetic.
- `docs/laws-trust.md` lists the models as part of the trusted base.

## Consequences

- `length(append(xs, ys)) == length(xs) + length(ys)` and
  `count(markDone(xs, id)) == count(xs)`-style laws prove by `induction xs`
  over the library functions; user-side structural copies are unnecessary.
- SPEC §6.5 "Induction follows the recursion shape" and §9 are narrowed.
- The std reach snapshot changes category for the modelled functions.

## Implementation notes
