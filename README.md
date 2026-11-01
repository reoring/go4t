# gofx

A small, statically typed functional language with Go-flavoured syntax that
compiles to ordinary Go: algebraic data types and records, exhaustive pattern
matching, higher-kinded type classes, explicit IO, persistent collections,
concurrency, typed Go externs, modules, and derived JSON codecs.

## Install and quick start

Requires **Go 1.22 or later** on PATH, including when gofx builds/runs generated
programs. From this checkout:

```sh
go install ./cmd/gofx
# Add $(go env GOPATH)/bin to PATH if it is not there already.
gofx eval '1 + 2'
```

The last command prints `3`. The installed binary embeds the runtime Go sources,
so `gofx run hello.gfx`, `gofx test hello.gfx`, and `gofx eval '1 + 2'` work from
any directory, even without a `go.mod` or a copy of this checkout. Only the Go
toolchain must remain on PATH; no third-party Go dependencies are needed.
Without installing, use `go run ./cmd/gofx` from the checkout root instead of
`gofx` in the commands below.

Create `hello/main.gfx` anywhere with this complete program:

```gofx
package main
import "std/string"
func main() IO[Unit] {
  let names = [" Ada ", " Bob "] |> map(string.trim)
  println("Hello, " + string.join(" and ", names) + "!")
}
```

```sh
gofx check hello/main.gfx
gofx run hello/main.gfx
# Hello, Ada and Bob!
gofx build hello/main.gfx
# Generated Go sources: hello/main_gen/main.go plus dependency module files.
```

`build` emits **Go source**, not a binary. `run` compiles and executes that
package. If the surrounding Go module resolves `gofx` (`go list -m gofx` succeeds),
it retains the generated sources there as before. Otherwise, `run`, `test`, and
`eval` use an automatically cleaned temporary module containing the embedded
runtime; they do not leave `_gen` directories beside your source.

`build` still writes `<file>_gen` (or the directory selected by `-o`/`--emit-go`).
Outside a usable Go module it prints a compilation note: to compile those sources
yourself, configure a module with `require gofx v0.0.0` and
`replace gofx => /absolute/path/to/gofx`, then run `go build` in the generated
directory. Externs outside `gofx/rt` and its subpackages require a real application
Go module providing those packages and `gofx`. A `gofx.mod` file controls language
import roots, not Go dependencies.

## CLI overview

| Command | Purpose and important flags |
| --- | --- |
| `gofx check FILE.gfx\|DIR...` | Load modules and type-check; warnings do not fail; no foreign ABI check. |
| `gofx build [-o DIR] FILE.gfx\|DIR...` | Emit one file per gofx module into one Go package. |
| `gofx build --emit-go DIR --package NAME FILE.gfx\|DIR...` | Emit an embeddable non-main package exposing `Main() rt.IO`; incompatible with -o. |
| `gofx run FILE.gfx\|DIR... [-- args...]` | Build and execute; inherits stdin and preserves child status in the installed CLI. |
| `gofx test FILE.gfx\|DIR...` | Run zero-arg monomorphic `testName` functions returning Bool/IO[Bool]; main optional and ignored. |
| `gofx eval 'EXPR' [FILE.gfx\|DIR...]` | Evaluate and show a pure value or IO result; supporting imports come from the first file. |
| `gofx bind IMPORT [FILE.gfx\|DIR...] [-o FILE.gfx\|DIR] [--prefix STR]` | Generate Go extern declarations; unsupported signatures get skip comments. |
| `gofx fmt [-w\|-d\|-l] FILE...\|DIR` | Print/write/diff/list canonical formatting; -l exits 1 on differences; flags exclusive. |
| `gofx lsp` | Start the stdio language server. |

Directory compilation includes immediate `.gfx` files; formatting recurses.
Explicit entry files must share a directory. Imports are file-local and qualified;
only the small core prelude is implicit. For example import `"std/io"` before
using `io.readFile`, and `"std/prelude"` before `prelude.pure`.

Run source tests and expression probes:

```sh
gofx test examples/cli-tools/main.gfx
gofx eval 'testArithmetic()' examples/cli-tools/main.gfx
gofx bind strings --prefix str
```

Ordinary IO errors exit 1; an uncaught action panic exits 2. The outer
`go run ./cmd/gofx ...` launcher adds `exit status N` and returns 1 for any
nonzero child status. `gofx check` cannot validate a foreign Go signature;
`build`/`run` perform that validation.

## Examples

Commands run from the checkout root with an installed `gofx` (or substitute
`go run ./cmd/gofx`). This table covers **every examples subdirectory**. Use the
explicit files below instead of blindly running directories containing negative
fixtures or multiple main functions. HTTP examples need network access;
concurrency timings and filesystem timestamps vary. Example READMEs contain
historical observed output; current imports/signatures are defined by SPEC/std.

| Example | Purpose | Run command |
| --- | --- | --- |
| [alias-fields](examples/alias-fields) | Transparent aliases and immutable common-field updates. | `gofx run examples/alias-fields/main.gfx` |
| [bank](examples/bank) | Stateful ledger, typed validation and HTTP audit (network). | `gofx run examples/bank/main.gfx` |
| [calc](examples/calc) | Expression interpreter with Result errors and free-variable analysis. | `gofx run examples/calc/main.gfx` |
| [cards](examples/cards) | Poker classification, derived ordering and seeded State shuffle. | `gofx run examples/cards/main.gfx` |
| [cli-tools](examples/cli-tools) | Pure/IO tests, eval and embedding; also run `gofx test examples/cli-tools/main.gfx`. | `gofx run examples/cli-tools/main.gfx` |
| [codegen-quality](examples/codegen-quality) | Million-step direct-self tail recursion and source locations. | `gofx run examples/codegen-quality/main.gfx` |
| [collections](examples/collections) | Persistent Map/Set/Array, grouping and snapshot preservation. | `gofx run examples/collections/main.gfx` |
| [concurrency](examples/concurrency) | Fork/await, ordered parallel work, race and timeout. | `gofx run examples/concurrency/main.gfx` |
| [deriving](examples/deriving) | Structural Debug and derived Functor/Foldable/Traversable. | `gofx run examples/deriving/main.gfx` |
| [diagnostics](examples/diagnostics) | Successful exhaustive Bool matching; sibling bad files demonstrate failures/warnings. | `gofx run examples/diagnostics/main.gfx` |
| [eqord](examples/eqord) | Custom equality/ordering and stable list operations. | `gofx run examples/eqord/main.gfx` |
| [float-num](examples/float-num) | Statistics, Float parsing/formatting, IEEE numeric edges. | `gofx run examples/float-num/main.gfx` |
| [fmt](examples/fmt) | Canonical formatter input; also try `gofx fmt -d examples/fmt/before.gfx`. | `gofx run examples/fmt/before.gfx` |
| [http](examples/http) | Natural Go HTTP extern and Option; requires network. | `gofx run examples/http/main.gfx` |
| [inventory](examples/inventory) | Records, inventory validation and HTTP notification (network). | `gofx run examples/inventory/main.gfx` |
| [io-effects](examples/io-effects) | Read configuration, validate, write a report and append a log (filesystem effects). | `out=$(mktemp -d); gofx run examples/io-effects/main.gfx -- examples/io-effects/config.txt "$out/report"` |
| [json-deriving](examples/json-deriving) | JSON schema derivation, round trips and nested error paths. | `gofx run examples/json-deriving/main.gfx` |
| [lists](examples/lists) | List Functor/Monad, list literals and cartesian do. | `gofx run examples/lists/main.gfx` |
| [local-recursion](examples/local-recursion) | Recursive lambda lets and mutually recursive local functions. | `gofx run examples/local-recursion/main.gfx` |
| [lsp](examples/lsp) | Cross-module hover/definition playground. | `gofx run examples/lsp/main.gfx` |
| [modules](examples/modules) | Directory loading, aliases, qualified names and intentional prelude shadow warning. | `gofx run examples/modules` |
| [monoid](examples/monoid) | User-defined Monoid instances with generic folds and State. | `gofx run examples/monoid/main.gfx` |
| [multi-constraint](examples/multi-constraint) | Multiple classes per parameter and superclass dictionaries. | `gofx run examples/multi-constraint/main.gfx` |
| [orders](examples/orders) | ADT transformations, Option validation and HTTP warehouse notice (network). | `gofx run examples/orders/main.gfx` |
| [parser](examples/parser) | User-defined Parser Monad and token-based arithmetic parsing. | `gofx run examples/parser/main.gfx` |
| [perf](examples/perf) | Native scalar signatures, bounded specialization and tail loops. | `gofx run examples/perf/main.gfx` |
| [pipe-partial](examples/pipe-partial) | Multiline pipelines, call placeholders and operator sections. | `gofx run examples/pipe-partial/main.gfx` |
| [records](examples/records) | Records, interpolation and let polymorphism; intentional Unit-default warning. | `gofx run examples/records/main.gfx` |
| [routes](examples/routes) | Shortest paths, record updates and unreachable Option results. | `gofx run examples/routes/main.gfx` |
| [scheduler](examples/scheduler) | Custom task ordering, guards and a bounded scheduling plan. | `gofx run examples/scheduler/main.gfx` |
| [state](examples/state) | Pure counter/stack transitions and generic monadic sequencing. | `gofx run examples/state/main.gfx` |
| [stdlib-extras](examples/stdlib-extras) | Regex log parsing, stable grouping/sorting, table formatting and Lazy. | `gofx run examples/stdlib-extras/main.gfx` |
| [strings](examples/strings) | Unicode strings, parsing, replacement and padding. | `gofx run examples/strings/main.gfx` |
| [tree](examples/tree) | Recursive Tree Functor and constrained rendering. | `gofx run examples/tree/main.gfx` |
| [typed-extern](examples/typed-extern) | Generated strings-package bindings and natural Go ABI adapters. | `gofx run examples/typed-extern/main.gfx examples/typed-extern/gostd.gfx` |
| [wc](examples/wc) | Stdin line/word frequencies with a top-N command argument. | `gofx run examples/wc/main.gfx -- 3 < examples/wc/input.txt` |

The IO example's command writes only inside its new `$out` directory. Inspect
`$out/report.report.txt` and `$out/report.log`, then remove that directory when
finished. Omitting its output-prefix argument instead writes root-level fixtures.
The word-count command supplies both checked-in stdin and its `3` argument.
Typed externs require both listed files (or their directory); do not run main alone.
`fmt/before.gfx` is already canonical, so `fmt -d` on it currently prints nothing.

Intentional negative examples are not normal smoke programs:

```sh
gofx test examples/cli-tools/failing.gfx                 # exit 1
gofx check examples/diagnostics/bad_hint.gfx              # exit 1
gofx check examples/diagnostics/bad_unused.gfx            # warnings, exit 0
```

## Documentation and editors

- [Tutorial for Go developers](docs/tutorial.md): classes, IO/do, errors, modules,
  tests, FFI, concurrency and JSON, with checked complete examples.
- [Language specification](SPEC.md): current grammar, type/effect rules, CLI,
  deriving, implementation limits and consolidated boundary tables.
- [Standard library reference](docs/stdlib.md): every std declaration's signature,
  semantics, instances and boundaries, generated from the source inventory.
- [Editor setup](editors/README.md): stdio LSP and a dependency-free VS Code client,
  including `gofx.executablePath`. Supports diagnostics, hover, definition and
  document symbols; not completion/rename.
- [Design history](DESIGN.md): historical slice contracts; SPEC describes the
  merged implementation where those earlier contracts differ.

## Documentation regression checks

The documentation package extracts every gofx fence in SPEC, tutorial, stdlib and
this README into temporary files. It calls the same module loader/type checker
as `gofx check`; `gofx error` fences must be rejected. Named `// dir:` groups and
`// file:` sections exercise real multi-file modules. These are **check tests**,
not claims that every snippet's IO was executed. A separate inventory test parses
all embedded std sources and requires module-local declaration/signature rows,
constructors, methods and derived instances.

```sh
go test ./docs ./std
```

On the shared development VM, run that focused acceptance through the VM wrapper:

```sh
/home/vagrant/.codex/skills/serialize-vm-tests/scripts/serialize-vm-tests -- go test ./docs ./std
```

`selfhost/` is a separately owned in-progress project, not part of this acceptance.
