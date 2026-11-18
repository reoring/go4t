# ADR 0018: Tactics: `absurd`, case analysis on expressions, Bool cases, bare `match`

Status: Accepted (design); implementation notes are appended below.

## Context

ADR 0001 noted that adding `requires` would need a tactic that closes a case
the assumptions rule out. ADR 0012 added `requires` without it. Observed:

```go4t
type S = Pending(Int) | Done deriving Eq, Debug
func isPending(s S) Bool { match s { Pending(_) => true; Done => false } }
func payload(s S) Int { match s { Pending(n) => n; Done => 0 } }
func bump(s S) S { match s { Pending(n) => Pending(n + 1); Done => Done } }
law bumpAdds(s S) requires isPending(s) { payload(bump(s)) == payload(s) + 1 }
proof bumpAdds(s) { match s { Pending(n) => refl; Done => refl } }
```

fails in the `Done` arm (`differs at: 0 vs 1`) and then advises
`this proof needs a case split, rewrite or induction` — the case split is
already there; the case is impossible. A precondition that is not a single
constructor shape cannot be worked around by restating the law.

Smaller gaps in the same code: a tactic `match` accepts only a proof variable
(`match flip(s) { … }` is a parse error), rejects `true`/`false` patterns
although a Bool has exactly those two cases, and has no bare form although
`induction x` has one. 35 of the 41 `if` tactics in the fixtures are
`if c { refl } else { refl }`.

## Decision

```ebnf
Tactic := 'refl'
        | ? absurd identifier ?
        | 'match' Expr ['{' TacticArms '}']
        | ? induction identifier ? Ident ['{' TacticArms '}']
        | 'if' Expr TacticBlock 'else' TacticBlock
        | 'rewrite' Name '(' [Expr {',' Expr} [',']] ')' TacticBlock
        | ? sample identifier ? '(' Int ')'
```

`absurd` is a contextual identifier like `sample` and `induction`. Tactic
patterns additionally accept `true`, `false` and alternatives `p | q`
(ADR 0020).

### 1. `absurd`

`absurd` proves the current case when the assumptions in scope cannot all
hold. Assumptions are the law's `requires` conjuncts, the conditions of
enclosing `if` tactics, and the equations introduced by enclosing `match`
tactics on expressions (§3). The check is sound and incomplete; it succeeds
when either holds:

1. Some assumption, normalized again under the current case-split solutions
   and under the assumptions made *before* it (never under itself or later
   ones), is a closed value different from the one assumed: `false` where
   `true` was assumed or the reverse, or a different head constructor.
2. The order closure of the assumed Int/String comparisons is inconsistent
   (the `impossibleOrder` test of ADR 0016).

A case that no argument values reach makes the quantified statement vacuous
there, so closing it is sound. If neither test succeeds:

```text
absurd: the assumptions in this case are not contradictory
```

followed by one line per assumption, `assumed: <term> is <true|false|pattern>`.

### 2. A failed tactic in an impossible case says so

When `refl` fails and the case is contradictory by the test above, the
diagnostic ends with

```text
this case cannot happen: <term> is <value> here; close it with absurd
```

instead of the counterexample lines (the counterexample search admits only
arguments that satisfy the precondition, so it can only report "no
counterexample" for such a case).

### 3. `match` on an expression

The scrutinee may be any expression that a law side may contain, of a data
type, tuple type or Bool. `induction` still requires a quantified variable.

- A scrutinee that normalizes to a bare proof variable is the existing
  variable split.
- A scrutinee that normalizes to a constructor, tuple or Bool literal is
  rejected: `match scrutinee is already decided: <normal form>`.
- Otherwise each arm binds its pattern variables to fresh symbols and assumes
  `normalized scrutinee = pattern term` for the arm, using the mechanism that
  already serves a stuck `match` inside a term. Arms must be exhaustive.
- The scrutinee is checked for termination and taint like an `if` condition.

### 4. Bool cases

`true` and `false` are tactic patterns for a Bool scrutinee and are jointly
exhaustive. `match c { true => t; false => u }` is `if c { t } else { u }`.
Other literal patterns stay rejected:
`tactic match supports only the literal patterns true and false`.

### 5. Bare `match`

`match e` with no arm block splits `e` into every case of its type and closes
each case with `absurd` when the case is contradictory, otherwise with `refl`.
Bare `induction x` closes its cases the same way after its hypothesis
rewrites. So `if c { refl } else { refl }` can be written `match c`.

### 6. Alternatives in arms

`A | B => tactic` proves the same tactic once per alternative
(`ast.Alternatives`). Alternatives bind the same variable names.

## Soundness evidence required of the implementation

- Fixtures on both sides of every rule: `absurd` rejected where the
  assumptions are satisfiable (including a case that is merely unproved);
  accepted for data preconditions, contradictory `if` branches, order
  contradictions and expression-match assumptions.
- The open-term property test covers the partial goals of the new forms: for
  an expression-`match` arm, only substitutions that satisfy the arm's
  assumption are compared with the runtime, and a fixture with no satisfying
  substitution fails the test (the rule of ADR 0012).
- A false law must not become provable through any new tactic; a negative
  fixture set states one false law per tactic.

## Consequences

- `SPEC.md` §6.5: grammar, tactic table, diagnostics.
- `ast.TMatch` gains `Scrut` for a non-variable scrutinee; `ast.TAbsurd` is new.
- The self-hosted compiler skips tactics without parsing them (ADR 0023).

## Implementation notes

The tactic implementation uses the shared `TAbsurd` and `TMatch.Scrut` nodes;
it adds no AST fields. A syntactically bare lower-case identifier uses `Var`,
including its existing proof-variable resolution rule. Parenthesized identifiers,
qualified names and other expressions use `Scrut`. Formatting preserves that
distinction, the bare flag, alternatives and tactic spans.

### Contradiction certificates

`contradiction` visits assumptions oldest first. While normalizing each
condition and its assumed value, the evaluator's assumption stack is exactly
that assumption's predecessor stack. Current case-split solutions remain
available. The incoming stack is restored on success and error. Opposite Bool
values and disjoint constructor heads are certificates; disjoint fields inside
tuples or a shared constructor are checked recursively. Neutral terms, unequal
symbols and failure to prove an equation are not certificates.

For order facts, `impossibleOrder` adds the proposed fact to the closure of its
predecessors and rejects strict cycles in Int or String order. Int also rejects
empty finite intervals. No new arithmetic normalization rule is introduced:
wraparound expressions remain opaque unless the existing normalizer proves a
wrap-free reduction. An inconsistent total order or an empty Int interval has
no runtime valuation, so closing that case cannot prove a false instance.

The failed-`refl` diagnostic invokes the same check and replaces counterexample
advice only when a certificate exists. Failed `absurd` lists the actual scoped
assumptions, oldest first, without using them to certify themselves.

### Case analysis and induction

The type checker permits datatypes, tuples and Bool and checks exhaustiveness
using its existing coverage checker. Only Bool literal patterns are allowed.
Eligibility scans expression scrutinees with the same dependency checker as
conditions and rewrite arguments, so divergence, tainted operations and forbidden
Float computations cannot manufacture branch equations.

A normal form with a constructor, tuple or Bool head is rejected as already
decided. A normal form that is a proof variable retains that variable's existing
structural ancestry. Other scrutinees use `refineMatch`, the same branch-local
equation mechanism as residual expression matches, with fresh pattern symbols
and no ancestry back to quantified variables. Thus a field of `f(xs)` cannot
be passed off as a structurally smaller `xs`. Solutions and assumptions are
restored between alternatives and on failure; every alternative is proved.

Bare case analysis synthesizes exhaustive constructor, tuple or Bool arms only
inside the evaluator. Each arm selects `absurd` if contradictory and `refl`
otherwise. Bare induction uses the existing self-rewrite elaboration for
reachable arms, retaining its structural-descent and precondition checks.
Deviation from the literal \"after its hypothesis rewrites\" wording: a
certified contradictory bare-induction arm closes immediately, without trying
to establish a recursive hypothesis's precondition in an impossible case.
This is sound because the contradiction alone proves vacuity; the hypothesis
is not used.

### Parallel pattern integration

As agreed with the integrator, this slice parses, resolves, checks and formats
top-level tactic-arm alternatives itself. Its recursive tactic-pattern validator
and evaluator (`ast.Alternatives`) support nested alternatives, but the general
pattern parser, resolver, type inference, formatter and nested-pattern editor
index remain owned by ADR 0020's patterns slice. Nested surface-pattern tests
belong to their integration; the executable grammar here describes the syntax
available before that merge. Tactic record refinements resolve checked field
names against constructor metadata rather than relying on pointer identity of
newly expanded record-pattern nodes.

### Runtime-oracle evidence

`tactics_forms.gt` exercises expression and projection matches, fresh fields and
wildcards, Bool cases, bare Bool/expression/tuple matches, alternatives,
normalized-variable splits, induction elaboration and a contradictory if arm.
The partial-goal traversal also visits generated induction-hypothesis rewrites.
Both sides of an arm equation are instantiated and normalized with hypotheses
cleared before accepting a runtime substitution; a mismatched field or head
is rejected. Every reachable partial goal must have a satisfying sample.

Certified contradictory partial goals have no runtime instance and are omitted
only after the production contradiction check, not counted as successful
comparisons. A separate `tactics_no_samples.gt` fixture has an unreachable arm
outside that incomplete check; the test verifies that its empty satisfying
sample set fails the oracle. Data-precondition and rejection fixtures live in
`testdata/tactics/`, outside the unconditional top-level fixture sweep. They
cover satisfiable and merely unproved `absurd`, data/if/order/expression
contradictions, and false laws for each new tactic form. Tests also reject
fabricated structural descent, divergent and tainted scrutinees, and decided
scrutinees. Parser, type/index, formatter round-trip/fuzz and editor diagnostic
tests cover the corresponding surface contracts.

The first full runtime-oracle run exposed a pre-existing synthetic-name collision:
bare induction's `$induction1_0` field binder was overwritten by the generated
self-rewrite argument with that name. Production `prove` does execute both
operations, but the old bare arm ends in `refl`, which reads the law environment,
not that synthetic field name; generated rewrite arguments intentionally read
the newly installed argument bindings. The observed defect was missing free
variables in newly covered partial-goal sampling, not an accepted false law.
Case binders now use the disjoint `$case...` namespace, and a focused regression
test checks the production elaborator preserves every existing case binding.


### Review evidence (2026-11-11)

After the seven surface-syntax slices were merged, an independent read-only
review attacked `absurd`, expression/Bool/bare `match`, bare `induction`,
alternatives, or-patterns, predicate bodies, implicit and inline proofs,
several `requires`, law type parameters, constants and the new literals with
465 hand-written laws (false laws and invalid proofs). No false law was
accepted. A one-time result: the scratch laws are not in the repository.

Open notes from that review, none turned into an accepted false law:

- A body whose root is `==`, even fully parenthesised, is an equation and is
  decided definitionally; with a hand-written `Eq` that returns `false`, both
  `{ (x == x) }` and `{ !(x == x) }` are accepted. By SPEC §6.5, but it is
  the one place where the two readings of a body disagree.
- Opaque Go handle types are unit-like to the checker (`type DB = DBHandle`),
  so a case split could prove two handles equal. Run later against
  `go:strings`: `law (a Reader, b Reader) { a == b }` was accepted with
  `match a { strings.ReaderHandle => match b }`. It predated this work. Fixed:
  a proof pattern naming a handle constructor, written or elaborated by bare
  `match`/`induction`, is rejected (`cannot case-split opaque handle`).
  Constructing a handle constructor in source remains undefined (SPEC §8.3).
- Bare `induction` selects hypothesis fields by printed type, which elides
  deep types; terms are untyped and descent is still checked.
- A law whose dependency is invalid is skipped without its own diagnostic;
  safe only while every invalid dependency reports an error.
