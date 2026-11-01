# io-effects

## Grammar and types

No grammar or builtin-type changes. The prelude now declares:

```gofx
type Result[E, A] = Err(E) | Ok(A) deriving Eq, Ord, Show
instance Functor[Result[E]]
instance Monad[Result[E]]
```

`fmap` changes only `Ok`; `pure(a)` is `Ok(a)`; `bind` returns the first `Err`
without calling the continuation. Existing partial-instance and `do` inference
handle `Result[E]` without checker changes. Derived equality compares constructor
and payload; ordering puts `Err` before `Ok`, then compares payloads. Derived
`Show` renders `Err(e)` / `Ok(a)` with each payload's `show` (strings unquoted).
Both type parameters receive the relevant Eq/Ord/Show constraints.

The bank, calc and inventory examples now use this prelude declaration and its
instances; their redundant Result types and Show/Functor/Monad instances were
removed. Prelude names remain non-redeclarable.

## Prelude signatures

| Signature | Semantics |
| --- | --- |
| `mapErr[E, F, A](r Result[E, A], f func(E) F) Result[F, A]` | Transform only the error payload. |
| `fromResult[E, A](r Result[E, A], d A) A` | Return the Ok payload, or the supplied default on Err. |
| `okOr[E, A](o Option[A], e E) Result[E, A]` | Some becomes Ok; None becomes Err(e). |
| `toOption[E, A](r Result[E, A]) Option[A]` | Ok becomes Some; Err becomes None. |
| `isOk[E, A](r Result[E, A]) Bool` | True exactly for Ok. |
| `isErr[E, A](r Result[E, A]) Bool` | True exactly for Err. |
| `either[E, A, B](r Result[E, A], onErr func(E) B, onOk func(A) B) B` | Invoke exactly the matching callback. |
| `liftResult[E Show, A](r Result[E, A]) IO[A]` | Ok becomes ioPure; Err becomes ioFail(show(e)). |
| `ioFailWith[E Show, A](e E) IO[A]` | Fail with the error's Show rendering. |
| `tryIO[A](io IO[A]) IO[Result[String, A]]` | Run lazily; success becomes Ok, returned errors and recovered panics become Err. |
| `readFile(path String) IO[String]` | Read the complete file; filesystem errors fail the IO. |
| `writeFile(path String, content String) IO[Unit]` | Create or truncate, preserving existing runtime parent-directory creation. |
| `appendFile(path String, content String) IO[Unit]` | Create or append, creating missing parent directories; write/close errors fail. |
| `fileExists(path String) IO[Bool]` | True for an existing file/directory, false for a missing path; other stat errors fail. |
| `listDir(path String) IO[List[String]]` | Sorted entry names, not full paths, including directory entries. |
| `removeFile(path String) IO[Unit]` | Remove a file or empty directory; a missing path fails. |
| `eprintln(s String) IO[Unit]` | Write s and a newline to stderr. |
| `print(s String) IO[Unit]` | Write s to stdout without a newline. |
| `nowMillis() IO[Int]` | Current Unix time in milliseconds, sampled when run. |
| `monotonicMillis() IO[Int]` | Elapsed monotonic milliseconds since runtime initialization. |
| `formatTime(millis Int, layout String) String` | Pure UTC formatting of Unix milliseconds with a Go time layout. |
| `randomInt(lo Int, hi Int) IO[Int]` | Uniform inclusive interval, supporting the entire signed Int range; not cryptographic. |
| `sleep(millis Int) IO[Unit]` | Sleep for positive milliseconds; nonpositive values return immediately. |

`randomInt` rejects `lo > hi` when run with exactly
`randomInt: lo must be <= hi`. `sleep` rejects positive milliseconds that would
overflow Go's `time.Duration` with exactly `sleep: duration out of range`.
Created parent directories use 0755 and files 0644, subject to umask, matching
existing WriteFile. Filesystem and stream errors retain the Go error text.

## Runtime and codegen contract

The uniform `any` ABI remains unchanged; `IO` is `func() (any, error)`.
`rt.TryIO(io any) any` produces an IO with a nil Go error after either success,
IO failure or a recovered panic. Returned errors use `err.Error()`; panics use
`fmt.Sprint(panicValue)` without an added prefix. Capture covers the action and
its bind continuations, not strict evaluation while constructing the argument.
As before, explicit process exit is not an IO failure and cannot be recovered.

`rt.RunMain(io any)` recovers panics raised while running the IO and prints
`error: panic: <msg>\n` to stderr, then exits 2. Ordinary ioFail/extern errors
still print `error: <msg>\n` and exit 1; success returns normally.

`rt.ShapeFuncs` gains `Ok func(a any) any` and `Err func(e any) any`.
Codegen emits `rt_ok(a any) any` and `rt_err(e any) any` constructing
`*Result_Ok`/`*Result_Err` and assigns them to `rt.Shapes.Ok`/`Err` during init.
The Result registration also works without List/Option and follows their init
when those shapes exist, so their struct assignment cannot erase Result shapes.
`rt.OkOf(value any) any` / `rt.ErrOf(value any) any` use the registered factories;
unregistered use panics with `rt.Shapes is not initialized: Ok is required` /
`rt.Shapes is not initialized: Err is required`, consistent with Some/None.

## Removed limitations

Programs no longer need to declare their own Result/instances. Recoverable IO
errors and Go panics can now be handled in the language instead of necessarily
terminating the program. File IO, timestamp formatting, inclusive random Ints,
sleep, stderr printing and no-newline stdout printing are available in the
prelude. Uncaught action panics no longer leak a Go stack trace through RunMain.

## Verification

All commands were run from `/workspace/wip/gofx/.wt/io-effects`. Resource-heavy
commands used the prefix:

```sh
SERIALIZE_VM_TESTS_MODE=shared /home/vagrant/.codex/skills/serialize-vm-tests/scripts/serialize-vm-tests --
```

- `go test ./rt -run '^TestRunMain$' -count=1`: initial red; new panic cases
  observed the old Go stack trace rather than the required single diagnostic.
- `go test ./rt ./types ./codegen ./prelude`: rt, types and prelude passed;
  codegen's new shape assertion initially depended on gofmt alignment. The
  assertion was corrected to check signatures/constructors without whitespace
  alignment. The helper execution test already passed.
- `go test ./codegen`: passed after that correction (`ok gofx/codegen 0.271s`).
- `go run ./cmd/gofx run examples/io-effects/main.gfx -- examples/io-effects/missing.conf /tmp/gofx-io-effects.i6winO/missing`: exit 0; defaults, Ok(7), first parse error; report and UTC log read back.
- `go run ./cmd/gofx run examples/io-effects/main.gfx -- examples/io-effects/config.txt /tmp/gofx-io-effects.i6winO/present`: exit 0; file config, Ok(18), first parse error; report and UTC log read back.
- `go run ./cmd/gofx check examples/wc/main.gfx`: exit 0, no diagnostics (run once).
- `go run ./cmd/gofx run examples/wc/main.gfx -- 3 < examples/wc/input.txt`:
  exit 0, output below. Redirection must be opened inside the serialized child
  (`runner -- bash -c 'go run ... < examples/wc/input.txt'`); an initial attempt
  to pass stdin into the runner itself produced zero counts because the runner
  starts its command in a background subshell with stdin detached.

```text
lines: 12, words: 60
   24 the
    5 garden
    5 leaves
```

Tests cover Result do dictionaries and rejection of mixed monads/error types,
prelude redeclaration and missing Show constraints, migrated example checking,
compiled Result helpers/instances/short-circuiting and panic capture, actual
RunMain subprocess exit codes, lazy/repeated TryIO, filesystem boundaries,
stream errors, UTC/monotonic time, sleep overflow and random range boundaries.
Go files were formatted with gofmt; no repo-wide suite, vet or other formatter
was run. Temporary example outputs and generated example Go directories were
removed after capturing evidence; no commits or pushes were made.

## Deviations

- `randomFloat` is deliberately omitted, as instructed: Float does not exist in
  this slice's base worktree. No other contract deviation.
- The parallel deriving slice replaces `deriving Show` with `deriving Debug`.
  Integration must migrate Result to its final deriving convention while
  retaining the explicit user-facing Show semantics specified above; the
  deriving owner was notified. This slice follows its own fixed base contract.

## Duplicated names

None: this slice did not introduce names owned by another slice. Concurrency
may duplicate sleep/monotonicMillis to compile independently; retain the
io-effects-owned implementations and remove those duplicate definitions at
integration.

## Unresolved

Per Main's explicit scope instruction, `selfhost/` is a separately owned,
in-progress project and was not edited. Its `base.gfx` declares a one-parameter
`Result[A] = Err(Diag) | Ok(A)`, Functor/Monad instances and `isOk`, while
`check.gfx` declares `liftResult`. These collide with the new prelude names if
compiled together. Its owner must migrate to `Result[Diag, A]`, remove redundant
instances/helpers and rename the checker-specific liftResult. No unresolved
issue remains within this slice's assigned runtime/prelude/example scope.
