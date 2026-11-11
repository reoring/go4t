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
