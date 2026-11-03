# CLI reference

[日本語](cli.ja.md)

| Command | Purpose |
| --- | --- |
| `go4t check [--full-types] FILE.gt\|DIR...` | Load, type-check and verify laws/proofs; warnings do not fail; foreign ABI is not checked here. |
| `go4t build [--full-types] [-o DIR] FILE.gt\|DIR...` | Emit one Go file per go4t module into one Go package. |
| `go4t build --emit-go DIR --package NAME FILE.gt\|DIR...` | Emit an importable Go package with typed exports (§8.7). |
| `go4t run [--full-types] FILE.gt\|DIR... [-- args...]` | Build and execute; inherits stdin; preserves exit status. |
| `go4t test FILE.gt\|DIR...` | Run zero-arg `testName` functions returning `Bool` or `IO[Bool]`. |
| `go4t eval 'EXPR' [FILE.gt\|DIR...]` | Evaluate a pure value or IO result; imports come from the first file. |
| `go4t bind IMPORT [-o FILE] [--prefix STR]` | Generate Go extern declarations. |
| `go4t fmt [-w\|-d\|-l] FILE...\|DIR` | Canonical formatting; `-l` exits 1 on differences. |
| `go4t init <module-path>` / `go4t get <path>@<version>` | Project and dependency management. |
| `go4t lsp` | Stdio language server (diagnostics, hover, definition, symbols). |

Only `check` verifies proofs; `build`, `run`, `test` and `eval` omit laws/proofs
from generated code but still require their source to parse and type-check.

Exit codes: ordinary IO errors 1, uncaught action panic 2. Recover inside the
program with `io.tryIO(action) : IO[Result[io.IoError, A]]`. With
`GO4T_TRACE=1` an uncaught panic also prints the Go stack, which locates the
shim or binding that raised it.

`go:` imports cache generated bindings and Go shims under
`os.UserCacheDir()/go4t/bind/<key>/` (`bindings.gt` and `shim.go`). Set
`GO4T_CACHE=/path/to/cache` to use `/path/to/cache/bind/`, or `GO4T_CACHE=off`
to regenerate on every invocation. Keys include the go4t binary identity, Go
version, package export-data content, binding options, and shim import path;
editing or touching a user `.gt` file does not invalidate its Go bindings.
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
