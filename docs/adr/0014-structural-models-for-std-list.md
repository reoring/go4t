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

### Landed models and exactness

The final set contains 14 declarations. `N` below means `Nil`, `x:r` means
`Cons(x, r)`, `++` means `append`, and all integer arithmetic is modulo
2^64. These are proof-normalizer equations, not runtime replacements.

| Function | Empty case | Constructor case | Why exactly equal to the shipped runtime |
| --- | --- | --- | --- |
| `length(xs)` | `0` | `1 + length(r)` | The fold adds one per cell; association does not change a modular sum. |
| `sum(xs)` | `0` | `x + sum(r)` | Addition has identity zero and is associative modulo 2^64, including overflow. |
| `product(xs)` | `1` | `x * product(r)` | Multiplication has identity one and is associative modulo 2^64, including overflow. |
| `count(xs, p)` | `0` | `(if p(x) { 1 } else { 0 }) + count(r, p)` | The same pure predicate contributes the same zero/one indicator; modular addition preserves the result. |
| `reverse(xs)` | `N` | `reverse(r) ++ [x]` | Consing each input onto the accumulator reverses precisely the input sequence. |
| `zipWith(xs, ys, f)` | `N` | `N` if `ys=N`; `f(x,y) : zipWith(r,s,f)` if `ys=y:s` | The accumulator followed by reverse preserves pair order and truncation at the shorter list; each callback receives the same pair. |
| `unzip(xs)` | `(N,N)` | Let `(as,bs)=unzip(r)`; return `(x.0:as, x.1:bs)` | Both reversed accumulators recover the original pair-component order. |
| `partition(xs, p)` | `(N,N)` | Let `(yes,no)=partition(r,p)`; prepend `x` to `yes` if `p(x)`, otherwise to `no` | Both partitions are stable and contain exactly the elements selected by the same pure predicate. |
| `flatten(xs)` | `N` | `x ++ flatten(r)` | The nested reverse-accumulating folds and final reverse equal concatenation in input order, including empty inner lists. |
| `intercalate(sep, xs)` | `N` | `x ++ (if r=N { N } else { sep ++ intercalate(sep,r) })` | Separators occur exactly between inner lists, including empty ones, never after the last list. |
| `init(xs)` | `N` | `N` if `r=N`; otherwise `x:init(r)` | The accumulator discards exactly the final element and reverses the retained prefix. |
| `span(xs, p)` | `(N,N)` | If `p(x)`, prepend `x` to the first component of `span(r,p)`; otherwise `(N,x:r)` | Both definitions stop at the first false predicate, preserving the prefix and the untouched suffix. |
| `findIndex(xs, p)` | `None` | `Some(0)` if `p(x)`; otherwise increment the index in `findIndex(r,p)`, retaining `None` | The first matching position is unchanged; incrementing the suffix index reproduces the accumulator even on Int wraparound. |
| `indexed(xs)` | `N` | `(0,x) : map(indexed(r), func((i,a)) => (i+1,a))` | Each position is its predecessor plus one modulo 2^64, and values remain in input order. |

Ownership is resolved from the checked module's `ImportPath == "std/list"`
and each function's owning module path. The source name alone grants no trust.
Neutral list arguments retain the original global call, never its fold body;
`zipWith` also waits for the second list's constructor when the first is `Cons`.
There are no new term kinds or runtime definitions. Tuple-valued recursions
bind the recursive pair once to avoid exponential duplicate evaluation.
`init` and `intercalate` normalize the tail's emptiness as a separate Boolean
test, preventing case refinement from unfolding an open tail indefinitely.

Eligibility still scans the real source bodies and callbacks for taint and
termination. The model-only calls to `append` and the owning List `fmap`
also receive termination certificates. A model does not admit an effectful,
extern-dependent or nonterminating callback.

### Functions left source-defined

Every other top-level function in `std/list/list.gt` is accounted for below.
An exact equation alone is not a reason to add a second model when the source
is already structural, or when another proof boundary still blocks its use.

| Function(s) | Reason for exclusion |
| --- | --- |
| `fold` | The source already exposes structural descent with an arbitrary accumulator. Reassociation is unsound for arbitrary callbacks; accumulator generalization remains necessary. |
| `foldl` | Generic Foldable continuation encoding with an arbitrary callback, not a fixed List aggregation admitting reassociation. |
| `toList`, `foldMapList` | Generic Foldable wrappers over `foldr`, not List accumulator loops. |
| `append`, `filter`, `zip`, `head`, `tail`, `take`, `drop`, `mapMaybe`, `elem`, `any`, `all`, `find`, `last`, `dropWhile`, `lookup` | Already constructor-structural; no accumulator replacement is needed. Existing termination/dictionary limitations still apply. |
| `concatMap`, `catOptions` | Delegate to already structural List bind / `mapMaybe`; no accumulator obstacle of their own. |
| `forEach`, `mapM` | Already structural, with arbitrary Monad dictionaries; no effect-order model is introduced. |
| `maximum`, `minimum` | Reassociating the left fold is not sound for arbitrary user `Ord` instances (including nontransitive or asymmetric comparisons and tie behavior). No new lawful-Ord witness restriction is introduced. |
| `nub` | Reusing `nub(r)` and then filtering it assumes equivalence laws for arbitrary user `Eq` instances. Filtering before recursion can preserve the source's first-occurrence behavior, but recurses on a computed list rather than exposing `nub(r)` to constructor-tail induction. |
| `sortBy`, `sort`, `sortOn` | Depend on unmodelled array sorting externs; outside this decision. |
| `range`, `rangeAcc`, `replicate`, `replicateAcc`, `iterate`, `iterateAcc` | Integer-controlled generation, not constructor induction over an input List. Arithmetic descent remains outside the termination checker. |
| `unfold`, `unfoldAcc` | Arbitrary state/step generation with no guaranteed structural descent; a model must not certify potentially infinite generation. |
| `splitAt`, `splitAtAcc` | An exact equation exists, but the source helper changes `n` before its decreasing list parameter and fails the current lexicographic termination check. Modelling it would not make its laws eligible without broadening termination trust. |
| `chunksOf`, `chunksOfAcc` | Recursive input comes from `splitAt`, not a certified constructor tail; the same termination boundary remains. |
| `takeWhile` | A projection of `span`, so it benefits from the `span` model without a separate model. |
| `zipWithAcc`, `findIndexFrom`, `initAcc`, `spanAcc`, `indexedFrom` | Explicit-accumulator implementation helpers. Their constructor descent is already exposed; public wrappers remove the initial-accumulator obstruction without inventing new helper invariants. |

Instance methods (`Functor.fmap`, `Monad.pure`/`bind`, `Foldable.foldr`,
`Traversable.traverse`, `Show.show`, `Debug.debug`, and derived Eq/Ord methods)
remain source-defined. The structural methods need no replacement; Show/Debug
must preserve arbitrary dictionary behavior rather than assume callback laws.

### Verification contract and deviations

`laws/testdata/list_models.gt` proves library length-of-append, mapping
preservation for length/count, and sum/product append. The corresponding
`list_models_sequences.gt`, `list_models_pairs.gt` and `list_models_indices.gt`
fixtures add a constructor-induction law for every other model, zip truncation,
and full predicate traversals. `list_models_false.gt` rejects false length-of-append
and a user-owned `length` returning 42 while still proving the library law.
The todo example now uses library `length` and has no structural copy.

The existing open-term runtime oracle covers all these fixture sides after
normalizing fresh variables and constructor-headed partial forms. Its
deterministic list samples include lengths 0, 1, 2, 8, 32 and 64, random
lengths, nested lists, and Int edge values/random bits. Long-list lengths are
staggered across parameters so each parameter gets every forced size without
unnecessarily multiplying the largest inputs to `intercalate`. Fixture batches
keep the generated runtime programs within the oracle's existing 90-second
per-program deadline; no substitutions or model equations were dropped.
Concrete substituted readback has a larger test-only fuel allowance; open
normalization and production proofs retain the existing 10,000-step budget.

No runtime source, dependency or language syntax changed. The sole design
boundary clarified during implementation is that an exact but still
termination-ineligible equation (`splitAt`, `chunksOf`) is not useful for
this structural-induction slice and is left source-defined. The trust-page
update is owned by the integrator; this table supplies its content.
