# CLI tools

Run these commands from the repository root. `go run ./cmd/go4t` is the source-tree
spelling of `go4t`; an installed binary accepts the same arguments.

## Tests

`main.gt` has three passing tests, including an `IO[Bool]` test. Its ordinary
`main` is ignored by the test command.

```console
$ go run ./cmd/go4t test examples/cli-tools/main.gt
PASS testArithmetic
PASS testWords
PASS testIO
3 passed, 0 failed
```

Exit status: **0**.

The deliberately failing test lives in a separate file, so it does not make the
normal example fail. It has no `main`:

```console
$ go run ./cmd/go4t test examples/cli-tools/failing.gt
FAIL testDeliberateFailure
0 passed, 1 failed
exit status 1
```

Exit status: **1**. The final `exit status 1` is printed to stderr by Go's `go run`
wrapper, not by `go4t test`. An installed `go4t` prints only the two result lines.
An IO error is reported as `FAIL testName: error message`, and later tests still run.

## Evaluate an expression

```console
$ go run ./cmd/go4t eval '1 + 2'
3
$ go run ./cmd/go4t eval 'readLine()' < examples/cli-tools/input.txt
Some(hello from stdin)
```

Both exit **0**. Pure values use `show`; IO expressions are executed and their
results use `show`. Optional `.gt` files supply functions, types and instances:

```sh
go4t eval 'testArithmetic()' examples/cli-tools/main.gt
```

Standalone evaluation uses the current Go module to resolve `go4t/rt` and extern
imports. With source files, their module is used instead, just as with `go4t run`.

## Output directories and Go embedding

Both commands below completed with exit **0** and no output:

```sh
go run ./cmd/go4t build -o examples/cli-tools/build_gen examples/cli-tools/main.gt
go run ./cmd/go4t build --emit-go examples/cli-tools/embedded_gen --package embedded examples/cli-tools/main.gt
```

The first writes a normal `package main` program to `build_gen/main.go`. The second
writes `package embedded` to `embedded_gen/main.go`, with no Go `main` function.
A Go caller imports the generated package and runs its exported entry:

```go
import (
    embedded "go4t/examples/cli-tools/embedded_gen"
    "go4t/rt"
)

func main() {
    var action rt.IO = embedded.Main()
    rt.RunMain(action)
}
```

`cmd/go4t` includes an integration test that emits a package into a temporary Go
module, imports it using `replace go4t => <repository>`, and executes `Main()`.

## Ordinary example execution

```console
$ go run ./cmd/go4t run examples/cli-tools/main.gt
Run this example with go4t test.
```

Exit status: **0**. Generated directories from the recorded runs are not checked in.
