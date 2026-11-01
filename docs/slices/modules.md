# Modules — Wave B specification delta

This is the integration delta for the modules slice. `SPEC.md`, `DESIGN.md`, and
`selfhost/` are unchanged by this slice.

## Grammar

```ebnf
File          = "package" Ident Separator* Import* Declaration* .
Import        = "import" [Ident] String Separator* .
QualifiedName = Ident "." Ident .
```

Imports precede declarations. `import "geometry"` selects the last path component
as the alias; `import g "geometry"` selects `g`. Imports are file-scoped, not
module-scoped. There are no dot imports, wildcard imports, export blocks, or
blank imports. Repeated/invalid aliases are errors; importing one module under
different aliases is allowed and does not duplicate its declarations.

Qualified values, constructors (including record constructors), types, classes,
instance heads, constraints, and constructor patterns are supported. Qualified
expression syntax shares `ast.Field` with record field selection. `_`-prefixed
constructors retain constructor syntax, including nullary patterns.

## Resolution and module semantics

A module is a directory of `.gfx` files with one package spelling. Directory
inputs load all immediate `.gfx` files in lexical filename order, not recursively.
Explicit entry filenames select only those files; all must share one directory.
Declarations are mutually visible across files within a module. Dependencies load
all of their directory's `.gfx` files. The entry module is last in the loader's
result. Dependencies are deduplicated by resolved identity before traversing their
imports, permitting recursive module graphs.

Import paths must be nonempty, relative, clean slash-separated paths without
backslashes or a leading `../`. Search order is:

1. The importing module's nearest ancestor containing `gofx.mod`; if absent,
   the entry program's directory. The file must contain one line `module <name>`.
   An exact `<name>` resolves to that root, and a `<name>/` prefix is stripped
   before looking up a directory.
2. Nonempty `GOFX_PATH` entries in environment path-list order.
3. The compiler's embedded `std/` tree, for paths beginning with `std/`.

An existing matching directory is authoritative: malformed sources, mismatched
package clauses, and empty source directories report errors rather than falling
through. A filesystem module's identity is its absolute directory; embedded
modules use `std/<name>`. Import aliases do not affect identity or instance scope.

Logical import identity is recorded separately in `Module.ImportPath`. A
filesystem override of `std/prelude`, `std/list`, `std/result`, `std/io`, or
`std/float` keeps its intrinsic/reexport identity without replacing its absolute
filesystem identity. This also preserves standard builtin aliases and avoids
spurious shadow warnings from standard definitions. Source-root overrides win
over `GOFX_PATH`, earlier path entries win over later ones, and filesystem
overrides win over embedded sources. Importing the same filesystem directory
through a module-name-prefixed path does not create a second instance set.

Every top-level declaration is public unless its name starts with `_`. Privacy
is checked at qualified access, for values, types, classes, and constructors.
Private declarations remain accessible throughout their own module. Instances are
global for the loaded graph, with no orphan rule; duplicate `(class, type)`
instances report both owning module paths when they differ.

The implicit prelude is a narrow namespace. Same-module declarations and lexical
bindings take precedence. Top-level declarations shadowing a prelude name produce
the nonfatal warning `shadows prelude name X`. Explicit imports never inject their
names unqualified.

## Standard library distribution

`std.FS` embeds `*/*.gfx`. The former monolithic `prelude` package is removed.
Existing Wave A signatures and behavior are retained under these owners; all
nonimplicit operations require qualification. Operation names are not shortened:
for example, `regex.regexMatch`, `map.mapInsert`, and `array.arrayGet`.

| Module | Owned declarations and operations | Implicit reexports |
| --- | --- | --- |
| `std/prelude` | `Functor`, `Monad`, `Show`, `Debug`, `Eq`, `Ord`, `Foldable`, `Traversable`; `Ordering`, `Option`; primitive/tuple/Option instances and helpers; `map`, `flatMap`, `min`, `max`, `unit`, `fst`, `snd` | Classes, data types and the listed core helpers/methods below; builtin `Unit` |
| `std/list` | `List` and instances; folds/conversions, traversal, zips, sorting, slicing, searching, grouping and remaining list helpers | `List`, `Nil`, `Cons`, `fold`, `filter`, `length`, `append`, `reverse`, `head`, `tail` |
| `std/result` | `Result` and instances; Option helpers (`fromOption`, `maybe`, `isSome`, `isNone`); Result transformations/conversions and `liftResult` | `Result`, `Err`, `Ok` |
| `std/io` | IO instances and primitives; console, arguments, environment, files, clocks, `tryIO`, `ioFailWith` | `println`, `print`, `eprintln`, `ioPure`, `ioFail` |
| `std/float` | Float instances and math/format/parse helpers; `minInt`, `maxInt`, `abs` | `toFloat` |
| `std/string` | String operations, conversions, parsing, code points, and padding | None |
| `std/regex` | Regex matching, finding, replacing, splitting and captures | None |
| `std/map` | Map operations and `Show`, `Eq`, `Functor` instances | None |
| `std/set` | Set operations and `Show`, `Eq` instances | None |
| `std/array` | Array operations and `Show`, `Eq`, `Functor` instances | None |
| `std/conc` | `fork`, `await`, `parMapIO`, `race`, `parBoth`, `timeout` | None |
| `std/state` | `State`, its `Functor`/`Monad` instances, `runState`, `evalState`, `execState`, `get`, `put`, `modify` | None |
| `std/lazy` | `Lazy`, `delay`, `force` (nonmemoizing) | None |

The exact implicit list is:

```text
Functor Monad Show Debug Eq Ord Foldable Traversable Ordering Option Result List Unit
map flatMap fold filter length append reverse head tail show debug
println print eprintln ioPure ioFail unit fst snd compare eq min max toFloat
```

Reexporting a data type also exposes its constructors:
`LT EQ GT None Some Err Ok Nil Cons`. The compiler links reexports to their
original declarations, not copied definitions. All loaded instances, including
operator instances, participate globally. `Int`, `Float`, `String`, `Bool`, `IO`,
`Map`, `Set`, `Array`, `Fiber`, and tuple types remain compiler builtins.

Other class methods are not implicit: explicitly import `std/prelude` and use
`prelude.fmap`, `prelude.pure`, `prelude.bind`, `prelude.foldr`, or
`prelude.traverse`. Instance method declarations retain their original names.
An explicit `prelude` import can also qualify the reexported operations/types.

## Diagnostics

Loader and checker errors retain `file:line:col: message` and source spans for
CLI excerpts/editor consumers. Module-specific messages include:

- `invalid import path "PATH"`
- `module "PATH" not found` (or `not loaded` for a manually assembled checker graph)
- `entry files must belong to one directory`
- `no .gfx files in DIR`
- `invalid gofx.mod in DIR: expected module <name>`
- `package NAME differs from NAME`
- `duplicate or invalid import alias ALIAS`
- `unknown import alias ALIAS`
- `module MODULE has no exported name NAME`
- `NAME is private to module MODULE`
- `duplicate instance CLASS[TYPE] in modules FIRST and SECOND`
- `warning: shadows prelude name NAME`

Existing missing-pattern, missing-instance, and unused-local diagnostics remain
unchanged. `examples/diagnostics/bad_unused.gfx` intentionally exits 0 with
warnings, unlike the other negative diagnostics fixtures; warnings are not errors.

## Code generation layout

The only public generator is:

```go
codegen.Generate(program *types.Program, pkg string) (map[string][]byte, error)
```

The returned keys are Go basenames. There is one file per gofx module. All files
belong to **one Go package**, `main` by default or the CLI's `--package` value.
The entry file is `main.go`; dependency filenames use sanitized last path
components with deterministic collision disambiguation. Dependency `main`
uses `main_module.go`, with additional collision suffixes when needed.

Top-level Go values use module-prefixed `pkg_name_` spelling; data/class names,
constructors, and instance dictionaries also carry their owning module prefix.
Cross-module references and dictionaries use those same identities. Runtime
`rt.Shapes` registration continues to refer to List, Option, Result, Ordering,
and Tuple2 correctly. Generated imports are per-file and source `//line`
directives survive splitting. The uniform `any` ABI and typed-extern adapters
are unchanged. Internal structural tests may use private `generate`; there is
no public single-file generator or `GenerateModules` compatibility alias.

## CLI

```text
gofx check FILE.gfx|DIR...
gofx build [-o DIR | --emit-go DIR --package NAME] FILE.gfx|DIR...
gofx run FILE.gfx|DIR... [-- args...]
gofx test FILE.gfx|DIR...
gofx eval 'EXPR' [FILE.gfx|DIR...]
gofx bind IMPORT [FILE.gfx|DIR...] [-o FILE.gfx|DIR] [--prefix STR]
```

Directory run/build defaults to `<DIR>/_gen`; explicit file inputs retain
`<first-file-base>_gen`. `build` emits the file map; `run` builds and executes the
whole generated package. Stale compiler-generated Go files are removed when
rewriting an output directory; unrelated files are preserved. Test/eval harnesses
use the same loader and isolate their generated programs from ordinary builds.

`bind` uses the receiving source module's package spelling and directory for Go
package loading. An existing output directory receives `bindings.gfx`. Without
receiving sources, generated bindings retain `package main`; the output path
alone does not infer source context.

## Deviations and approved clarifications

- Approved packaging clarification: one Go package with per-module files, not
  separate Go packages. Separate packages would conflict with lower-case source
  values, recursive module graphs, and global instance dictionaries.
- `std/state` and `std/lazy` supplement the required split to preserve existing
  Wave A functionality outside the deliberately narrow prelude.
- Fmt and LSP are owned by separate slices. This slice supplies their loader,
  import AST, and source-name seams rather than introducing competing commands.
- A raw single unnamed module remains supported for explicitly constructed
  checker test/embedding environments; it does not load std automatically.

## Integration seams

- Worktree `/workspace/wip/gofx/.wt/modules`, branch `slice/modules`. No commit,
  push, stash, checkout, or merge is part of this assignment.
- `types.Check(modules []*ast.Module) (*types.Program, error)` replaces the files
  API. The last module is the entry. `Program.Modules` and `Program.Files` expose
  checked syntax clones for named modules; caller syntax is not renamed.
- `ast.Module` contains `Path`, `ImportPath`, `Name`, and `Files []*ast.File`.
  `Path` is the deduplication/ownership identity; `ImportPath` carries the logical
  import identity (in particular `std/...` for standard declarations).
  `ast.Import` contains `Pos`, `End`, `Alias`, `Path`, and loader-assigned
  `Resolved`; `ast.File.Imports` is `[]ast.Import`.
- `modules.Load(inputs []string) ([]*ast.Module, error)` handles paths.
  `modules.LoadFiles(files []*ast.File, directory string)` handles already-parsed
  entry sources for editors, tests, and eval. Neither API typechecks sources.
- `Program.ModuleNames[canonical]` gives the original declaration spelling;
  `Program.Owners[canonical]` gives the module identity. LSP must use these when
  matching source tokens, and replace its former prelude/files loader with
  `modules.LoadFiles`. Fmt must render imports after the package clause. Main
  owns the cross-slice merge adaptations.
- `expandDeriving(file *ast.File, sourceNames map[string]string)` preserves
  original constructor labels in derived Debug output. The JSON slice must
  preserve original constructor tags and displayed type labels via the same
  mapping when integrated.
- `codegen.Generate(*types.Program, string) (map[string][]byte, error)` is the
  only public generation API. Callers write/build all files in the returned map.
- `extern.BindOptions.Package` defaults to `main`; CLI bind supplies the receiving
  module package when sources are given.

## Verification

All commands ran from the slice worktree. Changed Go files were formatted with
`gofmt -w`, excluding selfhost and unrelated sources. The VM serialization runner
wrapped tests/vet; CLI runs were also exercised directly.

### Regression suite

The initial post-Generate-cutover regression command exited 0:

```sh
go test ./ast ./lexer ./parser ./modules ./types ./codegen ./extern ./rt/... ./std ./cmd/gofx/...
```

Its first attempt timed out waiting for an unrelated VM lock holder before any
tests started; the next bounded attempt acquired the lock and passed.

After the bind tests and standard-override fix, `go list ./...` was filtered to
exclude paths containing `selfhost`. The exact expanded acceptance commands were:

```sh
go vet gofx/ast gofx/cmd/gofx gofx/cmd/gofx/testrt gofx/codegen gofx/extern gofx/lexer gofx/modules gofx/parser gofx/rt gofx/rt/stdhttp gofx/std gofx/types
go test gofx/ast gofx/cmd/gofx gofx/cmd/gofx/testrt gofx/codegen gofx/extern gofx/lexer gofx/modules gofx/parser gofx/rt gofx/rt/stdhttp gofx/std gofx/types
```

Both exited 0. Vet emitted no Go diagnostics. All ten test-bearing packages
passed; `ast` and `modules` have no package-local tests (loader integration is
covered by types/codegen/CLI tests).

Focused commands also exited 0:

```sh
go test ./cmd/gofx -run TestBindReceivingModuleAndDirectoryOutput -count=1
go test ./types -run TestStandardOverridesKeepPreludeIdentity -count=1
go test ./codegen -run 'TestGenerate(FilesAndRuntimeShapes|FilesystemStandardOverrides)' -count=1
```

The override test first failed in all ten source-root/GOFX_PATH cases with
missing `Functor`, `List`, `Result`, `ioFail`, or `toFloat`, establishing the
absolute-path versus logical-identity defect. After the fix, those cases passed,
including source-root/ordered-GOFX_PATH precedence and filesystem deduplication.
Executable tests passed with all five core standard modules overridden from
both search locations, preserving List/Option/Result runtime shapes and output.
Bind coverage checks directory and file receiving contexts, output
`bindings.gfx`, non-main package spelling, and Go imports found relative to the
receiving module.

### Examples and negative fixtures

```sh
go run ./cmd/gofx check examples/wc/main.gfx
go run ./cmd/gofx run examples/modules
for e in examples/*/main.gfx; do d=$(dirname $e); extra=""; [ -f $d/gostd.gfx ] && extra="$d/gostd.gfx"; args=""; [ -f $d/args.txt ] && args="-- $(cat $d/args.txt)"; in=/dev/null; [ -f $d/input.txt ] && in=$d/input.txt; go run ./cmd/gofx run $e $extra $args < $in > /dev/null || echo FAIL $e; done
```

All exited 0. The wc check emitted no compiler output. All 32 loop inputs ran;
stdout was empty (no `FAIL`). Stderr contained only the modules `map` shadow
warning and the records example's existing `None`-defaulted-to-Unit warning.
The final directory run printed the exact output/warning in
`examples/modules/README.md`. The loop's root IO-effects artifacts were restored
to their pre-existing contents.

| Command suffix after `go run ./cmd/gofx` | Exit | Observed intended result |
| --- | --- | --- |
| `test examples/cli-tools/failing.gfx` | 1 | `FAIL testDeliberateFailure`; `0 passed, 1 failed` |
| `check examples/diagnostics/bad_hint.gfx` | 1 | `no instance Show[Hidden]` with instance/deriving hint |
| `check examples/diagnostics/bad_missing_ctor.gfx` | 1 | `non-exhaustive match; missing: None, Some(_)` |
| `check examples/diagnostics/bad_missing_literal.gfx` | 1 | `non-exhaustive match; missing: Some(1)` |
| `check examples/diagnostics/bad_missing_nested.gfx` | 1 | `non-exhaustive match; missing: Some(Some(_))` |
| `check examples/diagnostics/bad_missing_tuple.gfx` | 1 | `non-exhaustive match; missing: (true, _)` |
| `check examples/diagnostics/bad_unused.gfx` | 0 | Three unused-local warnings for `forgotten`, `argument`, and `value` (approved warning-only exception) |

### Cleanup

Removed `.modules-check`, `.modules-fixtures/`, every generated
`examples/*/main_gen/`, and `examples/modules/_gen/`. Preserved the pre-existing
root `io-effects.log` and `io-effects.report.txt`. The temporary
`modules-std-split.md` authority is folded into this delta and removed.
No commits, pushes, or selfhost modifications were performed.

