# Modules — Wave B specification delta

## Integration seams

- Worktree: `/workspace/wip/gofx/.wt/modules`; branch `slice/modules`. No commit, push, stash, checkout, or merge was performed.
- `types.Check(modules []*ast.Module) (*types.Program, error)` replaces the files API. The final module is the entry module.
- `ast.Module` currently has `Path`, `Name`, and `Files []*ast.File`. `ast.Import` has `Pos`, `End`, `Alias`, `Path`, and loader-assigned `Resolved`; `ast.File.Imports` is `[]ast.Import`.
- `modules.Load(inputs []string) ([]*ast.Module, error)` accepts entry files or a directory. `modules.LoadFiles(files []*ast.File, directory string)` supports already-parsed entry sources (editor/test/eval integration). Explicit entry files must share one directory; directory inputs select all `.gfx` files, lexically ordered. Imported modules load their directory's files.
- The ONLY public generator is now `codegen.Generate(*types.Program, string) (map[string][]byte, error)`. The former single-file lowering stage is private `generate`, used by structural tests inside package `codegen`. There is no public `GenerateModules` compatibility alias. This final API cutover is implemented but has NOT yet been tested.
- Main approved the Go packaging clarification: one file per gofx module, all in ONE Go package (`main`, or the build `--package`). The module's last path component supplies a collision-disambiguated symbol prefix. Entry output is `main.go`; dependencies use module-named files. Separate Go packages would conflict with lower-case symbols, global instances, and module recursion.
- Named module syntax is cloned before qualification. Caller AST names remain intact; `Program.Files`/`Program.Modules` contain checked clones. `Program.ModuleNames[canonical]` is the original spelling; `Program.Owners[canonical]` is the module identity. These are the LSP index integration seam. Unnamed single-module test environments retain their existing direct annotations.
- `expandDeriving` now accepts `(file *ast.File, sourceNames map[string]string)`. `deriveDebug` uses original constructor spellings for rendered labels. The JSON slice must similarly preserve original JSON constructor tags and displayed type labels when integrated.
- `extern.BindOptions.Package` defaults to `main`. CLI `bind IMPORT [FILE.gfx|DIR...] [-o FILE.gfx|DIR] [--prefix STR]` accepts receiving module context; directory output writes `bindings.gfx`. Existing commands/flags remain supported.
- Fmt/LSP were developed in separate worktrees. Fmt must render `File.Imports` after package. LSP must replace its old prelude/files loader with `modules.LoadFiles` and use `Program.ModuleNames` to match source tokens. Main owns those merge adaptations.

## WIP state at interruption

User requested an immediate stop for a harness restart. This is an interruption handoff, NOT a completion claim.

### Implemented

- Import/alias parsing; qualified expressions via existing `ast.Field`; dotted type/class/instance/pattern names; private nullary constructor parsing.
- Directory loader, nearest `gofx.mod` root, module-name prefix resolution, ordered `GOFX_PATH`, embedded standard library fallback, package consistency, file-scoped imports, module deduplication.
- Namespace qualification, `_` privacy (`NAME is private to module MODULE`), global instance collection with cross-module duplicate errors naming both module paths, prelude shadow warnings (`shadows prelude name X`), narrow implicit prelude scope, qualified record constructors, local mutually recursive function groups.
- Standard library split into `std/prelude`, list/string/regex/map/set/array/io/conc/result/float, plus state/lazy to preserve Wave A functionality. `std/embed.go` embeds `*/*.gfx`. Old `prelude/prelude.go` and `prelude/prelude.gfx` were deleted; the prelude tests were moved to `std/*_test.go`.
- Implicit names: `Functor Monad Show Debug Eq Ord Foldable Traversable Ordering Option Result List Unit map flatMap fold filter length append reverse head tail show debug println print eprintln ioPure ioFail unit fst snd compare eq min max toFloat`. Reexported data types bring `LT EQ GT None Some Err Ok Nil Cons`. Nonlisted class methods require explicit `prelude.fmap`, `prelude.pure`, `prelude.bind`, `prelude.foldr`, `prelude.traverse`.
- Per-module Go output, mangled cross-module references/dictionaries, and working `rt.Shapes` registration for List/Option/Result/Ordering/Tuple2. CLI check/build/run/test/eval directory handling; directory run output is `<DIR>/_gen` (file inputs still use `<file-base>_gen`).
- Migrated existing examples and checker/codegen/CLI/stdlib tests. Mechanical helpers initially missed interpolated references and corrupted some local bindings; those observed failures were repaired. The temporary runtime fixture qualification shim introduced by a helper was removed in favor of explicit qualified fixture strings.
- Added parser/modules tests, types module/privacy/root-order/instance/shadow tests, codegen per-module executable test, CLI directory tests, and `examples/modules/{gofx.mod,main.gfx,geometry/point.gfx,geometry/translate.gfx}`.

### Observed verification

All commands ran from this worktree through `/home/vagrant/.codex/skills/serialize-vm-tests/scripts/serialize-vm-tests --` unless noted.

Most recent FULL non-selfhost source-package regression command:

```text
go test ./ast ./lexer ./parser ./modules ./types ./codegen ./extern ./rt/... ./std ./cmd/gofx/...
```

Observed at 16:19:55 JST: exit 0. `lexer`, `parser`, `types`, `codegen`, `extern`, `rt`, `rt/stdhttp`, `std`, `cmd/gofx`, and `cmd/gofx/testrt` passed; ast/modules had no package-local test files. Earlier runs failed on stale fixture imports, filenames, collision snapshots, and directory output location; those failures were fixed before this green run.

IMPORTANT: after that green run, the public generator was renamed/cut over from `GenerateModules` to `Generate` (map return), the old `Generate` became private `generate`, its internal test callers were migrated, std executable tests were changed to write/run multiple files, and final output-filename collision handling was adjusted. **No test/build has run after these final edits. There is no observed post-cutover failure output yet; their status is unverified.**

Example runs:

- Built temporary `.modules-check` using `go build -o .modules-check ./cmd/gofx` (this binary predates the final generator API rename).
- All 31 existing `examples/*/main.gfx` programs ran with exit 0. The batch completed through `tree`, then its Eval cell reached its 180-second deadline; `typed-extern` and `wc` were subsequently run separately to obtain explicit results. Typed extern used both `main.gfx` and `gostd.gfx`.
- IO-effects used `-- .modules-fixtures/missing.conf .modules-fixtures/output`, avoiding modification of the pre-existing root `io-effects.log`/`io-effects.report.txt`.
- The wc run exited 0 and printed `lines: 0, words: 0`; input supplied to the outer serialization runner did not reach the program. For a nonempty-input rerun, pipe input INSIDE the runner's child shell.
- `go run ./cmd/gofx run examples/modules` exited 0 and printed:

```text
examples/modules/main.gfx:6:1: warning: shadows prelude name map
func map(n Int) Int { n * 2 }
^
point: (3, 4)
squared distance: 25
translated: Point{x: 4, y: 6}
local map: 42
```

### Exact next steps

1. Run `gofmt -w` ONLY on Go files changed in this slice (the latest new/edited files still need final formatting).
2. Run the full non-selfhost regression command above again after the final public generator cutover; fix any compile/runtime failures. Existing full-suite acceptance is explicitly authorized for this slice. Do not touch selfhost.
3. Run `go run ./cmd/gofx check examples/wc/main.gfx` once for the named acceptance check (not yet successfully exercised as that exact command), and rerun the modules directory example against the final source.
4. Add `examples/modules/README.md` with the observed output/warning above and commands. Complete this document's grammar, semantics, standard module distribution, diagnostics, verification, and deviations sections; retain the Integration seams section.
5. Add/finish direct `bind` directory-context/output coverage (implementation is present; directory-specific bind behavior was not separately exercised). Verify the migrated negative diagnostics fixtures and `examples/cli-tools/failing.gfx` still fail for their intended reasons; their main positive counterparts already ran.
6. Inspect the source-root standard override corner before declaring done: `[INFERENCE]` a filesystem override specifically for `std/prelude` or one of its reexport-owning std modules may lose intrinsic/reexport identity because filesystem modules currently use absolute `Module.Path`, while qualifier linkage keys std modules by `std/...`. The resolution-order tests currently cover local regular modules, GOFX_PATH regular modules, GOFX_PATH `std/string`, and embedded fallback, all passing. A proposed `Module.ImportPath` field/standard-identity map was only considered; **NO such edit was started or landed**. Preserve filesystem identity for deduplication while linking standard reexports by resolved import identity; add focused coverage rather than silently narrowing resolution order.
7. Cleanup only this slice's generated artifacts: `.modules-check`, `.modules-fixtures/`, the `examples/*/main_gen/` directories created by the example runs, and `examples/modules/_gen/`. Preserve pre-existing root IO-effects artifacts. `docs/slices/modules-std-split.md` is a temporary worker authority describing the old prelude line-based split; consolidate useful facts into this delta then remove that scaffold if no longer needed.
8. Final report must list files/contracts, exact observed commands/results, example output, the approved single-Go-package clarification, and any unresolved risks. No commits or pushes.

### File ownership/history notes

All changes are in this worktree. Main dispatched StdSplit (std sources/existing examples), TestApiMigrate (mechanical Check signature migrations), and FixtureMigrate (stdlib-aware tests). They have stopped editing; Modules owns integration. FixtureMigrate explicitly acknowledged that its initial "no unresolved snippets" claim was inaccurate. Do not assume helper reports supersede the observed verification above.
