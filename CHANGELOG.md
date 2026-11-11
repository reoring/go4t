# Changelog

All notable changes to go4t. The format follows
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/). go4t is pre-1.0:
the grammar, standard library and Go ABI may change between minor versions.

## [Unreleased]

### Added

- **`requires` on laws.** `law name(params) requires cond { lhs == rhs }` states
  a precondition; the proof starts with it assumed, and `rewrite` with such a
  law must establish it ([ADR 0012](docs/adr/0012-law-preconditions-and-order-reasoning.md)).
- **Unsatisfiable preconditions are reported.** A `requires` clause whose
  conditions contradict each other (`a < 5 && a > 7`) is an error instead of
  a vacuously true law; when no argument satisfying it is found in the
  candidate search, a failed proof says so and a successful one carries a
  warning ([ADR 0016](docs/adr/0016-requires-satisfiability.md)). A range can
  be written once as a `Bool` function and shared: `requires plausible(t, at, now)`.
- **Order reasoning for `Int` in laws.** Comparisons are decided by the
  total-order closure of the assumptions (transitivity, antisymmetry), and by
  linear reasoning where interval analysis proves no operand can wrap. `a + 1 > a`
  now proves under `requires a < 9223372036854775807` and stays unprovable
  without it.
- **Counterexamples for failed proofs.** When `refl` fails, `go4t check` searches
  for concrete arguments that make the two sides differ and prints them, or
  says that none was found and the proof needs a case split, rewrite or
  induction ([ADR 0013](docs/adr/0013-failed-proof-diagnostics.md)).
- **Structural models for `std/list` in laws.** Fourteen accumulator-based
  functions (`length`, `sum`, `product`, `count`, `reverse`, `zipWith`,
  `unzip`, `partition`, `flatten`, `intercalate`, `init`, `span`, `findIndex`,
  `indexed`) now reduce structurally in proofs, so `induction xs` works over
  the library functions directly
  ([ADR 0014](docs/adr/0014-structural-models-for-std-list.md)).
- **Warning for recursion go4t cannot bound.** A function that calls itself
  in a non-tail position with no argument that is a structural part of its
  parameter is reported: stack exhaustion is a fatal Go error, not a
  recoverable panic. `std/list` is tested on million-element lists
  ([ADR 0015](docs/adr/0015-unbounded-recursion-warning.md)).
- Fuzz targets for the lexer, parser and formatter; CI fuzzes each briefly.
- `examples/go-decision-template`: one go4t decision behind a Go API, as a
  copyable Go module.
- `docs/laws-trust.md`: what a proved law rests on and how each part is checked.
- `CONTRIBUTING.md`, `SECURITY.md`, this changelog; English summaries on the
  ADRs written in Japanese.

### Changed

- `go4t build`/`run` of a program that binds many Go packages resolves their
  export data in one `go` invocation instead of one per package (248 → 1 for
  the client-go example; warm build 30 s → 9 s). `go4t run`, `test` and `eval`
  build their temporary binary with `-buildvcs=false`, so a commit in the
  enclosing repository no longer forces a relink.
- Law diagnostics print terms in source spelling: `now - at` instead of
  `(-1 * at#2) + now#3`; the `#n` suffix appears only for a real name clash.
- CI runs on the minimum supported Go (the `go` line in `go.mod`) and on the
  current stable release. Tests that bind newer standard-library APIs or
  third-party modules say which Go release they need and are skipped below it.
- `DESIGN.md` moved to `docs/history/DESIGN.md`; it is historical and
  `SPEC.md` is the only definition of current behaviour.
- The generated standard-library reach snapshot moved from
  `docs/adr/0001-laws-std-reach.md` to `docs/laws-std-reach.md`.

### Fixed

- Go 1.27: generated shims referenced unexported type aliases of the bound
  package (`[]pkg.number`) and did not compile; they now spell out the
  aliased type.
- `go vet ./...` failed on Go 1.22: the committed `examples/typed-extern` shim
  had been generated with a newer toolchain and referenced
  `url.URL.AppendBinary`. Committed generated bindings are now produced with
  the minimum supported Go.
- README: the exported Go function for `freshness` is `decide.FreshnessFn`
  (the name `Freshness` is the type).

## [0.1.0] - 2026-11-05

First tagged release: the language, standard library, `go:` imports,
`--emit-go`, equational laws, LSP server and self-hosted compiler as described
in `SPEC.md` at that tag.

[Unreleased]: https://github.com/reoring/go4t/compare/v0.1.0...HEAD
[0.1.0]: https://github.com/reoring/go4t/releases/tag/v0.1.0
