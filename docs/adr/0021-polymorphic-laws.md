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
