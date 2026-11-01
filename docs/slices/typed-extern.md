# typed-extern

## Language and compilation delta

The extern grammar is unchanged:

```gofx
extern func f(x Int, s String) Bool = "import/path".F
```

`codegen.Generate` now resolves each distinct Go package with `extern.Loader`.
The loader uses `go list -json -export -deps` and the standard-library
`go/importer` export-data importer, honoring the current module, replacements,
build constraints, and Go toolchain. `Loader.Dir` optionally selects another
module context. Its cache lasts one compilation, not across source edits.
`Package.Functions` reports exported package-level functions in lexical order,
with a `go/types.Signature` exposing parameter/result types. Methods are not
package-level extern targets. No third-party dependencies were added.

The generated gofx functions retain their uniform `any` ABI and dictionary
parameters. Each foreign call is inspected independently:

| Declared gofx value | Natural Go representation | Adapter |
| --- | --- | --- |
| `Int` | `int64` (also Go `int`; see deviation) | Assert `int64`; convert at the `int` boundary and back |
| `Float` | `float64` | Assert `float64`; no new Float builtin is declared here |
| `String` | `string` | Assert `string` |
| `Bool` | `bool` | Assert `bool` |
| `Unit` | `rt.Unit` | Assert `rt.Unit`; a Go function with no result is not Unit |
| `IO[T]` | `rt.IO` | Assert `rt.IO` on input; forward the action on output |
| `List[String]` | `[]string` | `rt.ListToStrings` / `rt.ListFromStrings` |
| `List[Int]` | `[]int64` | `rt.ListToInts` / `rt.ListFromInts` |
| `Option[T]`, primitive `T` | adjacent `(T, bool)` values | Expand inputs into payload/presence; box outputs with `rt.SomeOf` / `rt.NoneOf` |
| Other types | `any` | Pass through without changing representation |

Primitive Option payloads are Int, Float when available, String, Bool, and Unit.
An absent Option supplies the Go zero payload and `false`; a present zero payload
remains `Some`, not `None`. Options of other types use `any`. List conversions
preserve order, map empty/nil slices to `Nil`, and do not share mutable slice
storage with generated immutable lists. True Go aliases are accepted; distinct
named primitive types are not silently coerced.

If a Go function has all-`any` parameters and an `any` result, the existing plain
forwarding wrapper is retained. An all-`any` function returning `rt.IO` also
retains forwarding when the declaration returns `IO[T]`. Otherwise every
parameter and result must match its natural representation. Go generics and
variadic functions are not extern targets. Invalid signatures fail generation
with a positioned diagnostic:

```text
FILE:LINE:COL: extern F: Go signature X does not match declared type Y
```

Here `F` is the exported Go function name; `X` omits parameter names and `Y` is
the declared semantic gofx function type. Missing exported functions report
`extern F: exported Go function not found in PATH`; package-loading errors keep
the declaration position. As before, `check` checks the gofx types only; foreign
signature validation occurs during generation (`build` and `run`).

## Error-returning functions and HTTP

A Go function returning `(T, error)` can implement a declared `IO[T]`. Its call,
argument conversions, and result boxing happen inside the generated `rt.IO`
closure. Creating the action performs no effect. A non-nil error returns
`(nil, err)` and participates in ordinary IO short-circuiting; a successful
result is boxed as its declared type. Re-running the action repeats the Go call
and reconstructs mutable slices from immutable lists.

`rt/stdhttp.GetStatus` is now exactly:

```go
func GetStatus(url string) (int64, error)
```

It returns the HTTP status (including non-2xx statuses), or zero plus the network
error. Its generated adapter supplies the laziness that formerly lived inside
the runtime function. `examples/http/main.gfx` is unchanged. Its existing binding
`httpGet(url String) IO[Int]` now uses the natural adapter. Tests cover the
unchanged example's generation, lazy requests against a local test server, and
network errors, without contacting an external HTTP server.

## Binding command

```text
gofx bind IMPORT [-o FILE.gfx] [--prefix STR]
```

The command writes a complete `package main` declaration file to stdout or the
specified file. Exported functions are emitted in lexical order; parameter names
are stable `p0`, `p1`, etc. With no prefix, the first Unicode letter of the Go
function is lower-cased. `--prefix str` instead produces `strToUpper` from
`ToUpper`, avoiding prelude-name collisions without changing language scoping.
An unprefixed declaration colliding with a prelude function or class method is
preceded by `// collides with prelude: name`. It is not silently renamed or
omitted; use a prefix before compiling it alongside the prelude.

Representable `(T, error)` results generate `IO[T]`; primitive `(T, bool)` results
generate `Option[T]`. Adjacent Go parameters are emitted independently: bind does
not guess that arbitrary `(T, bool)` inputs represent an Option. Explicit extern
declarations may use the Option input adapter.

Unsupported functions receive `// skipped: Name (reason)`. These include
variadic/generic functions, pointers, callbacks, unsupported integer widths,
unsupported named types, unsupported multiple results, and no-result functions.
`any` and `rt.IO` lack enough type information to infer a safe gofx declaration,
so bind skips those even though an explicit typed extern can use them. Reserved
gofx names are skipped with a suggestion to use a prefix. Float declarations are
enabled by a checker capability probe; `float64` is skipped when the Float
builtin is unavailable and enabled automatically when the float-num slice is
integrated.

## Added runtime signatures

No prelude functions or types are added.

- `rt.ListFromInts(values []int64) any`: constructs generated `List[Int]` in input order.
- `rt.ListToInts(list any) []int64`: copies generated `List[Int]` into a Go slice in input order.

The existing shape initialization errors are preserved: missing `Cons`/`Nil`
panics with `rt.Shapes is not initialized: Cons and Nil are required`; missing
`Uncons` panics with `rt.Shapes is not initialized: Uncons is required`.
No shape struct fields were added.

## Removed limitations

Users no longer need to hand-write an all-`any` Go wrapper for representable
natural signatures. Go errors can cross the foreign boundary through lazy IO,
and binding declarations can be generated from package metadata. The existing
single-program/multiple-file command behavior is reused; no module/import or
prelude-shadowing feature was added.

## Approved deviations

1. Go `int` is also bridged to gofx `Int`, with canonical gofx representation
   still `int64`. This is necessary because the required `strings.Repeat` accepts
   Go `int`, not `int64`. Go's ordinary integer conversion rules apply (on 32-bit
   targets, out-of-range arguments narrow). No other integer widths are coerced.
2. The primary requested `--prefix STR` and collision comments to make generated
   standard-library bindings usable without prelude shadowing. The example uses
   the checked-in, unmodified `gostd.gfx` generated with `--prefix str`, and runs
   both files explicitly rather than hand-writing aliases in `main.gfx`:
   `go run ./cmd/gofx run examples/typed-extern/main.gfx examples/typed-extern/gostd.gfx`.

## Duplicated names

None. Float, Result, Debug, and other slice-owned language names were not declared.

## Integration

The adapter is emitted inside the existing function body pipeline: source `//line`
directives, self-tail-call lowering for non-externs, `go/format`, and embedded
`goPackage` output remain intact. `bind` dispatch precedes general argument
validation and coexists with `test`, `eval`, and build output options. Integer
list conversions are additive; Result, tuple, and Ordering shape helpers remain.
The pipe/partial lowering fixture uses the same real foreign package as the
other extern tests, so package analysis validates it without removing coverage.
The `Alias` fixture is skipped without a prefix now that `alias` is a language
keyword; prefixed emission and true Go alias matching remain covered.

Integration acceptance passed for all non-selfhost packages with `go vet` and
`go test`, and for all 31 `examples/*/main.gfx` programs (including both files for
typed-extern). The diagnostics missing-constructor check fails as intended with
`missing: None, Some(_)` and a source underline; `bind strings --prefix str`
prints complete prefixed declarations. The records example retains only its
intentional Unit-defaulting warning.
