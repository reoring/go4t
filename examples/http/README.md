# Examples

Run these commands from the repository root.

## HTTP

`go run ./cmd/gofx run examples/http/main.gfx` requires network access. It prints
the HTTP status and an `Option` result using the `Show` class and string
concatenation:

```text
status: 200
twice: 23
```

## Tree

`go run ./cmd/gofx run examples/tree/main.gfx` needs no network access. It defines
a recursive `Tree`, a `Functor[Tree]` instance, and a polymorphic
`render[A Show]` function with `|`-separated match arms. Mapping over the tree
multiplies each value by ten:

```text
((. 10 .) 20 .)
```

## Generated Go

`go run ./cmd/gofx build examples/http/main.gfx` writes
`examples/http/main_gen/main.go` with `package main`; the tree example similarly
uses `examples/tree/main_gen/main.go`. Each source file gets its own
`<base>_gen/` directory, so multiple `.gfx` programs can share a source directory.
`run` builds this directory and executes it with `go run` from the module root.
Generated `*_gen/` directories are ignored by Git. Legacy `<base>_gen.go` files
are neither written nor removed.
