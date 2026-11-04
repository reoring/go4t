# CLI reference

[日本語](cli.ja.md)

| Command | Purpose |
| --- | --- |
| `go4t check [--full-types] FILE.gt\|DIR...` | Load, type-check and verify laws/proofs; warnings do not fail; foreign ABI is not checked here. |
| `go4t build [--full-types] [-o FILE] FILE.gt\|DIR...` | Build an executable; retain generated Go sources beside the input. |
| `go4t build [--full-types] --emit-go DIR [--package NAME] FILE.gt\|DIR...` | Emit Go sources only; package defaults to `main`. Other package names produce typed exports (§8.7). |
| `go4t run [--full-types] FILE.gt\|DIR... [-- args...]` | Build and execute; inherits stdin; preserves exit status. |
| `go4t test FILE.gt\|DIR...` | Run zero-arg `testName` functions returning `Bool` or `IO[Bool]`. |
| `go4t eval 'EXPR' [FILE.gt\|DIR...]` | Evaluate a pure value or IO result; imports come from the first file. |
| `go4t bind IMPORT [FILE.gt\|DIR...] [-o FILE.gt\|DIR] [--prefix STR] [--shim DIR]` | Generate declarations and optional Go shims for inspection or legacy use. |
| `go4t fmt [-w\|-d\|-l] FILE...\|DIR` | Canonical formatting; `-l` exits 1 on differences. |
| `go4t init <module-path>` / `go4t get <path>@<version>` | Project and dependency management. |
| `go4t lsp` | Stdio language server (diagnostics, hover, definition, symbols). |

Only `check` verifies proofs; `build`, `run`, `test` and `eval` omit laws/proofs
from generated code but still require their source to parse and type-check.

`go4t build` writes the executable in the current directory, named after the
first input directory or the first input file without `.gt`. `-o FILE` selects
another executable path; an existing directory is rejected. Generated sources
remain in `DIR/_gen` for a directory input or `FILE_gen` for a file input.
`--emit-go DIR` selects source-only output and cannot be combined with `-o`.
`--package NAME` is available only with `--emit-go`.

Building requires Go on PATH. The compiler reuses a Go module that provides
`go4t`, or supplies its embedded runtime in a temporary module without changing
the user's `go.mod`. Go build diagnostics are printed on stderr and failures
exit nonzero.

`go4t get <path>@<version>` runs `go get` and records the selected version in
`go4t.mod`; it does not generate bindings. Use `import "go:<path>"` to bind Go
packages on demand while loading the program.

Exit codes: ordinary IO errors 1, uncaught action panic 2. Recover inside the
program with `io.tryIO(action) : IO[Result[io.IoError, A]]`. With
`GO4T_TRACE=1` an uncaught panic also prints the Go stack, which locates the
shim or binding that raised it.

`go:` imports cache generated bindings and Go shims under
`os.UserCacheDir()/go4t/bind/<key>/` (`bindings.gt` and `shim.go`). Set
`GO4T_CACHE=/path/to/cache` to use `/path/to/cache/bind/`, or `GO4T_CACHE=off`
to regenerate on every invocation. Keys include the go4t binary identity, Go
version, package export-data content, direct imports' export-data hashes sorted
by import path, binding options, and shim import path. Changing a direct import
invalidates the binding; editing or touching a user `.gt` file does not.
Checking and building can have separate entries because their shim import paths
differ. Missing or unparsable entries are regenerated, and cache write failures
do not prevent compilation. `GO4T_TRACE=1` reports per-package cache hits,
misses, disabled caching, and cache errors on stderr.

## Documentation tests

`go test ./docs ./std` extracts every `go4t` fence from SPEC, tutorial, stdlib,
`docs/laws.md` and the README and runs the same loader and type checker as
`go4t check`; `go4t error` fences must be rejected.

`selfhost/` is a separately owned in-progress self-hosting compiler and is not
part of the release surface.
