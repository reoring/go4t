# Go interop: embedding, bindings, projects

[日本語](go-interop.ja.md)

## Embedding in a Go program

This is the intended workflow.

1. Write the decision in `decide/decide.gt`: a projection record `Observed`,
   an action sum, and `func decide(o Observed) Action`.
2. Generate: `go4t build --emit-go decide --package decide decide/decide.gt`
   (put it in a `//go:generate` line). The package exports `Decide`, Go mirror
   types for every record and sum (`ObservedGo`, `ActionRollout{}`,
   `TtlTtlNanos{F0: …}`), and `Main() rt.IO`.
3. In Go: read the world, fill `Observed`, call `Decide`, switch on the result,
   perform the effects. Nothing in `decide` can perform I/O; the compiler
   enforces it.

Rules that keep the boundary clean (SPEC §8.7, tutorial §12):

- Project only what the decision needs; every call copies the record.
- Never cross a sentinel. `Option[Int]` crosses as a pointer-free mirror;
  compare with `Some(x)`, do not unwrap with `-1`.
- Cross durations and timestamps as `Int` nanoseconds — the representation of
  `time.Duration` and `UnixNano` — so no rounding happens at the boundary.
- A validity flag beside a list becomes a sum per element; "first failure in
  order" then matches the original Go loop exactly.

Runnable template: [examples/go-shell](../examples/go-shell). Real ports:
[examples/approval-controller](../examples/approval-controller),
[examples/attachment-repair-controller](../examples/attachment-repair-controller).

## Calling Go from go4t

```sh
go4t bind strings --prefix str          # prints extern declarations
go4t bind modernc.org/sqlite -o bindings.gt
```

Generated externs cover functions, methods on opaque handles, slices, options
and errors; signatures that cannot be mirrored get a skip comment instead of a
wrong binding. [examples/sql-service](../examples/sql-service) is an HTTP service
over SQLite with Prometheus metrics built this way; [examples/typed-extern](../examples/typed-extern)
shows the ABI adapters. `std/http` needs no bindings.

## Projects and dependencies

A go4t library is a Go module with `go.mod`, `go4t.mod` and `.gt` files.

```sh
go4t init example.com/myapp            # creates go.mod + go4t.mod
go4t get example.com/lib@v1.0.0        # go get, then record in go4t.mod
```

Go selects versions and keeps checksums; `go4t.mod` lists language imports.
Imports are file-local and qualified (`import "std/io"` → `io.readFile`); only
the small core prelude is implicit. Resolution order: the importing module's
source root, `go4t.mod` requires, `GO4T_PATH`, embedded std. See
[examples/deps](../examples/deps) for an offline local-replacement setup.
