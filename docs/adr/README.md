# Architecture decision records

One file per decision, numbered in the order they were taken. Each records
the context, the options considered, the decision, and its measured
consequences; where an ADR has been implemented the status line names the
commit. SPEC.md states the resulting rules; an ADR explains why.

ADRs written in Japanese carry an English summary.

| ADR | Decision | Status |
| --- | --- | --- |
| [0001](0001-equational-laws.md) | Equational laws (`law` / `proof`) are checked by a layer separate from the type system | Accepted |
| [0002](0002-laws-map-and-diagnostics.md) | `Map` is modelled symbolically in laws; stuck-term diagnostics | Accepted |
| [0003](0003-import-time-go-bindings.md) | `import "go:…"` binds Go packages at build time; bindings are cached, never committed | Accepted |
| [0004](0004-laws-float-bearing-data.md) | Float-bearing data types may be law universals; Float values stay unreachable | Accepted |
| [0005](0005-io-tail-lift.md) | A pure expression in an `IO`-expected position is lifted (`ioPure` elision) | Accepted |
| [0006](0006-generic-go-functions.md) | Generic Go functions are instantiated per call site | Accepted |
| [0007](0007-sampled-laws.md) | `sample(N)` laws are tested, not proved, and cannot serve as rewrite lemmas | Accepted |
| [0008](0008-direct-style-io.md) | IO lowers to direct style; value representation measurements | Accepted |
| [0009](0009-cross-package-upcasts-and-lock-values.md) | Upcasts across direct imports; shims take addresses instead of copying lock-bearing values | Accepted |
| [0010](0010-scalar-implementer-upcasts.md) | Named scalar/collection implementers are upcast sources (`client.inNamespaceAsListOption`) | Accepted |
| [0011](0011-lossless-boundary.md) | The Go boundary never converts lossily: JSON is a view, in-out pointers are `Ref[T]`, errors cross as `GoError` | Accepted |
| [0012](0012-law-preconditions-and-order-reasoning.md) | Laws may state a `requires` precondition; Int order reasoning is decided only where it is wrap-free | Accepted |
| [0013](0013-failed-proof-diagnostics.md) | A failed `refl` searches for a counterexample and prints terms in source spelling | Accepted |
| [0014](0014-structural-models-for-std-list.md) | Accumulator-based `std/list` functions get structural models in laws | Accepted |
| [0015](0015-unbounded-recursion-warning.md) | Warn on non-tail self-recursion that no data structure bounds | Accepted |
| [0016](0016-requires-satisfiability.md) | A `requires` clause must be satisfiable, and the checker says when it cannot tell | Accepted |
| [0017](0017-law-declaration-surface.md) | Law declarations: optional and inline proofs, several `requires`, predicate bodies | Accepted |
| [0018](0018-tactics-absurd-and-case-analysis.md) | Tactics: `absurd`, case analysis on expressions, Bool cases, bare `match` | Accepted |
| [0019](0019-lexical-traps-and-numeric-literals.md) | A leading `-` is rejected, comparisons do not chain, Int literals get separators and bases | Accepted |
| [0020](0020-or-patterns.md) | Or-patterns and negative literal patterns | Accepted |
| [0021](0021-polymorphic-laws.md) | Laws may quantify over types | Accepted |
| [0022](0022-declaration-conveniences.md) | Record shorthand, top-level `let`, bare operator sections, trailing commas, spelling hints | Accepted |
| [0023](0023-selfhost-erases-laws-by-token.md) | The self-hosted compiler skips laws token by token and follows the lexical rules | Accepted |

The [standard-library law reach report](../laws-std-reach.md) is a generated
snapshot maintained by `laws/taint_reach_test.go`, not a decision record.

Reading order for the Go boundary: 0003 → 0006 → 0009 → 0010 → 0011. For
laws: 0001 → 0002 → 0004 → 0007 → 0012 → 0016 → 0013 → 0014 → 0017 → 0018 → 0021.
For surface syntax: 0019 → 0020 → 0022 → 0023.
