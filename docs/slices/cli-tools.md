# cli-tools — Pass 12 specification delta

## Commands

```text
gofx check FILE.gfx...
gofx build [-o DIR | --emit-go DIR --package NAME] FILE.gfx...
gofx run FILE.gfx... [-- args...]
gofx test FILE.gfx...
gofx eval 'EXPR' [FILE.gfx...]
```

### `test`

- Files still form one package with shared declarations and the ordinary prelude.
- After parsing, remove user `main` declarations and append a parsed harness AST;
  only then call `types.Check`. A user `main` is neither required nor checked/run.
- Collect non-extern, monomorphic top-level functions with a nonempty suffix after
  `test`, zero value parameters, and the explicit return type `Bool` or `IO[Bool]`.
  Other functions are not tests but remain subject to ordinary type checking.
- Run tests in input-file order, then declaration order, exactly once each.
- True prints `PASS <name>`; false prints `FAIL <name>`; a failing IO prints
  `FAIL <name>: <error>`. IO failure does not stop later tests.
- Print `N passed, M failed`; exit 1 iff M is nonzero, otherwise exit 0.
  An empty suite prints `0 passed, 0 failed` and exits 0.
- The harness calls a private-to-the-command support API (not a prelude binding):
  `testrt.Run(name, action any) any`, where `action` is erased `func() IO[Bool]` and
  the result is erased `IO[Bool]`. The support action reports and captures Go errors;
  panic behavior remains that of the ordinary runtime, not a new test exception policy.
- Generated names avoid collisions with existing functions and class methods.

### `eval`

- Parse EXPR as exactly one expression, preserving its positions under `<eval>`.
  Trailing declarations are rejected. Source files supply ordinary declarations;
  their `main`, if any, is removed before checking.
- Inject EXPR into a parsed `func main() IO[Unit] { println(show(EXPR)) }` AST.
- If that check fails, replace the wrapper with the parsed equivalent of
  `func main() IO[Unit] { do { x <- EXPR; println(show(x)) } }` and check again.
  The second wrapper supplies the IO context; no diagnostic-string matching or
  failed-check partial annotations are used to decide whether it succeeds.
- If both checks fail, return the **original** diagnostic. A successful check's
  warnings are printed once. Runtime errors and process exit statuses propagate.
- With no source files, resolve runtime/extern imports from the current Go module;
  with files, use the module containing the first file, as `run` does.

### `build`

- `-o DIR` overrides the output directory, still writing `DIR/main.go` as an
  executable Go `package main` program. Without output flags, use the existing
  `<first-input-stem>_gen/main.go` path.
- `--emit-go DIR --package NAME` writes `DIR/main.go` with that non-main Go package
  clause and exactly the entry `func Main() rt.IO`; no Go `func main` is emitted.
  It returns the program's IO action; the embedding Go program decides when to run it.
- The source program still requires `func main() IO[Unit]` in either build mode.
- Output flags can appear before or after inputs. `-o` and `--emit-go` are mutually
  exclusive; `--emit-go` and `--package` require each other. `NAME` must be a valid
  Go identifier other than `_` or `main` (keywords are invalid).
- These commands emit Go source, not binaries, matching the existing build command.

## Grammar, types and prelude

No new language grammar, types, classes or prelude declarations. The uniform `any`
ABI is unchanged; the exported Go embedding entry alone has concrete result `rt.IO`.
`codegen.Generate(program, goPackage)` keeps executable output for an empty name or
`main`, and emits `Main() rt.IO` for other valid package names.

Diagnostics retain `file:line:col: message`; synthetic sources use `<eval>` and
`<test>`. CLI usage remains `gofx:1:1: usage: ...`; invalid emitted package names
report `invalid Go package name "..."`. Usage tests check the presence of `usage`
instead of pinning the full wording.

## Removed limitations and cleanup

Programs can now be tested without an application main, expressions can be evaluated
with or without supporting files, build output locations are selectable, and generated
programs can be embedded in Go. Test/eval use temporary source and binary directories
and leave ordinary build output untouched. Child exit status is returned to the CLI
entrypoint only after deferred cleanup; no new persistent runtime state is introduced.

## Verification

All commands ran from `.wt/cli-tools`, through the VM-wide serialized-test runner:

- `go test ./cmd/gofx ./cmd/gofx/testrt ./codegen` — all three packages passed.
  Command coverage includes pure/IO tests, IO errors and continuation, ignored/absent
  main, empty suites, discovery exclusions and source order, eval/EOF/input/error
  paths, original-error preservation, output overrides and an imported emitted package
  in a temporary module with `replace gofx => <worktree>`.
- `go run ./cmd/gofx run examples/cli-tools/main.gfx` — exit 0; ordinary example message.
- `go run ./cmd/gofx check examples/wc/main.gfx` — exit 0, no diagnostics (run once).
- `go run ./cmd/gofx test examples/cli-tools/main.gfx` — 3 passed, 0 failed; exit 0.
- `go run ./cmd/gofx test examples/cli-tools/failing.gfx` — 0 passed, 1 failed; exit 1.
- `go run ./cmd/gofx eval '1 + 2'` — `3`; exit 0.
- `go run ./cmd/gofx eval 'readLine()' < examples/cli-tools/input.txt` —
  `Some(hello from stdin)`; exit 0. The input file was opened inside the runner's
  child because its background shell otherwise redirects inherited stdin to EOF;
  that initial wrapper-only run produced `None`, also with exit 0.
- `go run ./cmd/gofx build -o examples/cli-tools/build_gen examples/cli-tools/main.gfx`
  — exit 0, no output.
- `go run ./cmd/gofx build --emit-go examples/cli-tools/embedded_gen --package embedded examples/cli-tools/main.gfx`
  — exit 0, no output.

The example README records the observed output. Generated example directories were
removed after verification; no binaries, temporary scripts or generated sources remain.

## Deviations

None. The test runtime helper lives under `cmd/gofx/testrt` to keep error capture
local to the assigned command slice without duplicating io-effects APIs.

## Duplicated names

None.

## Integration notes

Other slices also touch `cmd/gofx/main.go` and `codegen/codegen.go`: preserve the
`bind` dispatch from typed-extern and diagnostic source/caret rendering from
 diagnostics when integrating. The non-main entry hook only changes the final
entrypoint emission, not dictionaries, function emission or value-shape registration.
