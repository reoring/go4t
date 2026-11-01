# Directory modules

Run from the repository root:

```sh
go run ./cmd/gofx check examples/modules
go run ./cmd/gofx run examples/modules
```

The run exits 0. Observed stdout:

```text
point: (3, 4)
squared distance: 25
translated: Point{x: 4, y: 6}
local map: 42
```

Observed stderr (the warning does not prevent execution):

```text
examples/modules/main.gfx:6:1: warning: shadows prelude name map
func map(n Int) Int { n * 2 }
^
```

`gofx.mod` establishes the `example` module root. `main.gfx` imports the
`geometry` directory both by its default alias and as `g`; its two source files
share declarations. The example uses qualified record construction and matching,
a derived `Debug` instance across modules, and a local `map` that shadows the
implicit prelude name. `_helper` stays private to `geometry`.

Generated Go files are written to `examples/modules/_gen/`: one per gofx
module, all in package `main`, including `geometry.go` and the entry `main.go`.
