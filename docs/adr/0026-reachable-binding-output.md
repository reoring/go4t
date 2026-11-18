# ADR 0026: Check whole packages, emit reachable declarations

Status: Accepted. Supersedes the build-output rule of [ADR 0003](0003-import-time-go-bindings.md); retains the cache identities of [ADR 0006](0006-generic-go-functions.md).

## Context and options

A controller with 314 lines of go4t produced 160 Go files (5,944,137 bytes).
Package-wide shims were written during loading, including conversion functions
for APIs that no entry declaration called. The module splitter also emitted
every loaded dependency. The Go linker may discard dead binary code, but it
cannot reduce source that a Go project must store and compile.

Pruning imports before checking would change what the checker can resolve and
could hide invalid laws or generic applications. Making the package cache
caller-specific would also discard useful sharing across different programs.

## Decision

Keep import-time binding, type checking and package-template caching complete.
After checking, project the program for emission. Root every runtime declaration
in the entry module, even private helpers and functions not called by `main`.
Follow resolved function references (including function values), signatures,
data fields, classes, selected instances and their dictionary contexts. Mirror
`String` methods contribute concrete `Debug` dictionaries. Keep module naming
based on loader order; omit files with no emitted declaration. Laws and proofs
are still checked by `check`, not turned into executable roots.

Only then materialize shims, before codegen inspects their Go ABI. Select the
reachable wrappers from the cached Go template, follow local converter function
references and retain their imports. Generic calls use their already validated
per-tuple companions; neither package nor tuple cache entries are rewritten to
reflect a particular caller. Explicit `go4t bind` still exposes the full API.
Both executable and library builds use the same emission plan.

Reachable ADTs keep all constructors, and selected dictionaries keep their
methods and superclasses. The shared runtime shape initializer groups List,
Option, Ordering and tuple callbacks. Reachable Go calls can return an error
classified by a transitive dependency: keep that dependency's classifier
initializer without importing its unrelated wrappers. Re-emission removes
obsolete generated files, not unrelated Go sources.

## Consequences

On the same controller, the emitted output was 25 files (122,681 bytes),
including the unchanged 21,210-byte typed export API. Its Go module built.
The approval controller went from 161 files to 32, and its behavioral tests
passed. These are source measurements, not binary-size or compile-time claims.
