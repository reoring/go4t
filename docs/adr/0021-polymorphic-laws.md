# ADR 0021: Laws may quantify over types

Status: Accepted (design); implementation notes are appended below.

## Context

Law parameters are monomorphic (ADR 0001). A fact about `append` or `length`
must be stated at one element type and can be rewritten with only at that
type: 80 of the 326 fixture laws fix a generic type at `Int`, `String` or
`Bool` for no reason of their own. ADR 0014 modelled fourteen `std/list`
functions inside the normalizer because "a lemma about `length[A]` cannot be
stated"; `std/` ships no law at all.

## Decision

```go4t
law lengthAppend[A](xs List[A], ys List[A]) {
  length(append(xs, ys)) == length(xs) + length(ys)
} proof {
  induction xs
}
```

- **Syntax.** `law name[A, B](params)`: plain type-parameter names of kind
  `*`. Constraints and higher kinds are rejected:
  `law type parameters cannot have class constraints`,
  `law type parameters cannot be higher-kinded`.
- **Meaning.** The equation holds for every choice of the type parameters.
- **Checking.** Type parameters are rigid in the precondition, both sides and
  every tactic expression, as in a generic function. With no constraints, a
  value of type `A` can be passed, stored and returned, and nothing else: it
  has no `Eq`, `Ord`, `Show` or constructors (`cannot split A`).
- **Proof.** Normalization is unchanged; a variable of type `A` is an opaque
  symbol. The eligibility check accepts a law's own rigid type parameters
  where it otherwise requires a supported type. The Float exclusion is
  unaffected: it concerns operations on Float values, and an unconstrained `A`
  admits none.
- **`rewrite`.** Each use instantiates the law's type parameters with fresh
  type variables and unifies its parameter types with the arguments; type
  arguments are never written. The induction hypothesis is the law at its own
  type parameters.
- **Counterexample and witness search.** Every type parameter is instantiated
  at `Int`. A counterexample found there refutes the law and is printed as
  `counterexample (A = Int): xs = …`; finding none proves nothing, as before.
- **`sample(N)`** is rejected: `sample does not support type parameters`.

## Not in this decision

Class-constrained type parameters (they need symbolic dictionaries),
higher-kinded parameters, quantification over functions, and shipping lemmas
in `std/` (every program loads `std/prelude`, so a law there is proved on
every `go4t check`; the `compile-std-check-ns` budget needs a decision first).

## Soundness evidence required of the implementation

- The open-term property test runs polymorphic fixtures with the type
  parameters at `Int`, for both sides and every partial goal.
- Negative fixtures: a false polymorphic law; a law that is true at `Int` only
  because of an `Int` operation and therefore must not type-check at `A`; a
  `rewrite` at two different instantiations in one proof.

## Consequences

- `SPEC.md` §6.5; `ast.LawDecl.TParams`.
- Laws are erased by the self-hosted compiler (ADR 0023).

## Implementation notes

- `types.LawInfo.TParams` retains the unique rigid `TVar` identities. Parameter
  types, preconditions, equation sides and tactic expressions share that scope.
  Module resolution also imports the law's type-parameter names into a separate
  proof file, even when that file precedes the law. Constraint and kind errors
  are reported before resolving constraint names.
- Non-self rewrites use fresh inference variables and retain their inferred
  instantiations in the law-reference annotation. Self rewrites retain the
  enclosing law's rigid variables; automatic induction already constructs its
  hypothesis arguments from those parameter/field types. Imported law references
  use the same path. Law declarations and instantiated references expose their
  types through the existing position index and LSP hover.
- No normalizer rule changed. The existing `unsupportedTypeAt` already treats
  rigid variables as opaque, including generic function/data parameters; the
  law checker now supplies well-scoped law variables to that same path. Concrete
  Float and higher-order quantified parameters retain their exclusions.
- Soundness review:
  - `eval` tests annotations for concrete `Int`, `Bool` and function types;
    a rigid `A` does not enter the scalar-specific branches. Arithmetic and
    class operations on `A` are rejected by the ordinary checker.
  - `types/derive.go` expands `Eq` and `Ord` into ordinary checked instances
    with field dictionaries and parameter constraints. `laws/primitives.go`
    resolves those exact dictionaries and unfolds their checked method bodies;
    missing dictionaries are errors, never guessed from term shapes. Derived
    `Eq[Box[A]]` and `Ord[Box[A]]` cannot bypass the missing `Eq[A]`/`Ord[A]`.
  - `mapComparator`, `eligibleMapDictionary` and `trustedMapInstance` require
    the checked `compare` method and recursively trusted concrete dictionaries.
    Literal key ordering additionally requires literal scalar/tuple terms.
    No comparator is invented for `A`; `Map[Int, A]` may transport opaque
    values, while operations requiring `Ord[A]` are rejected.
  - All fourteen `std/list` models remain owner-checked. Reductions inspect
    `Nil`/`Cons`, transport elements, project declared pairs, or call checked
    predicates/functions. `sum`/`product` require `List[Int]`; other arithmetic
    affects lengths, counts or indices, not opaque elements.
  - Counterexample and witness search substitute only the law's rigid
    parameters with `Int` for candidate generation. They do not mutate the
    checked AST or prove anything from unsuccessful search. Counterexample
    headers list every type binding in declaration order.
- `sample` rejects even phantom law parameters before generator construction;
  the shared `checkSample` guard also protects `AddSampleTests`.
- `polylaws_` fixtures cover induction for length/append, right identity and
  associativity, explicit self-rewrite partial goals, two independent rewrite
  instantiations, `requires`, annotated lambdas, opaque list/map values and
  false laws. The open-term oracle retains rigid open normalization and uses
  `Int` only for runtime observers, generated arguments and partial-goal
  signatures. Type-name annotations in copied source sides are substituted by
  AST position, without changing value names or string literals. Rejected
  arithmetic/class operations use the existing table-driven negative-test
  convention rather than top-level property fixtures.
- No semantic deviation from the decision. Fixtures use separate proofs because
  inline proofs belong to the law-declaration slice. `SPEC.md`'s `LawDecl`
  production gained `[TParams]`, which was absent at this base; generated
  grammar coverage exercises `TParams` in a fixed valid equation.
- Integration boundary: this slice's base oracle covers explicit match/rewrite
  prefixes and structural match variants, but does not visit the generated arms
  of bare induction or automatic induction-hypothesis rewrite prefixes. Those
  traversal changes are owned by the concurrent tactics slice and must be
  verified together with these polymorphic fixtures after integration.
