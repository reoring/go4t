# Slice: concurrency

## Grammar, types, and representation

No new syntax. Add the opaque builtin `Fiber : (*) -> *`, exposed to the compiler
as `types.Fiber`. `Fiber[A]` is an ordinary immutable value type; `Fiber` can be
passed where a unary higher-kinded constructor is expected. It has no language
constructors and no built-in `Show`, `Eq`, `Functor`, or `Monad` instance.

The uniform `any` ABI is unchanged. A fiber is an `*rt.Fiber`: one goroutine, one
completion channel, and a cached value/error. Closing the channel publishes the
result to all awaiters. No await reruns the action; multiple concurrent awaiters
are safe. Running the same `fork` IO again creates a different fiber.

Lists and options retain their generated representations and cross the ABI via
`rt.Shapes`. `parMapIO` constructs its result with `Shapes.Nil`/`Shapes.Cons` and
reads its input with `Shapes.Uncons`. `timeout` uses `Shapes.Some`/`Shapes.None`
through the existing `SomeOf`/`NoneOf` helpers.

For the generated pair returned by `parBoth`, the shared shape contract gains:

```go
Tuple2   func(a, b any) any
Untuple2 func(p any) (a, b any)
```

Codegen emits `rt_tuple2`/`rt_untuple2` and registers them in the existing valid
List/Option shape `init` block; that block also ensures `Tuple2` is declared.
`rt.Tuple2Of(a, b any) any` calls the constructor and panics with
`rt.Shapes is not initialized: Tuple2 is required` if it is absent.

## Prelude externs

All bindings target `"gofx/rt"` and use `any` arguments/results in Go.

| Signature | Go binding | Semantics |
| --- | --- | --- |
| `fork[A](io IO[A]) IO[Fiber[A]]` | `Fork` | Start one goroutine when this IO runs; capture its result, failure, or panic. |
| `await[A](f Fiber[A]) IO[A]` | `Await` | Wait for the fiber and return its cached result or original IO failure. |
| `parMapIO[A, B](xs List[A], f func(A) IO[B]) IO[List[B]]` | `ParMapIO` | Run every mapper/action concurrently, join all, and return values in input order; first observed failure wins. |
| `race[A](a IO[A], b IO[A]) IO[A]` | `Race` | Return the first completed outcome, including failure; abandon the loser. |
| `parBoth[A, B](a IO[A], b IO[B]) IO[(A, B)]` | `ParBoth` | Run and join both actions, returning a pair in argument order; first observed failure wins. |
| `timeout[A](millis Int, io IO[A]) IO[Option[A]]` | `Timeout` | Return `Some` for timely success, propagate timely failure, or return `None` and abandon the late action. |
| `sleep(millis Int) IO[Unit]` | `Sleep` | Defer a millisecond delay; nonpositive values return immediately. |
| `monotonicMillis() IO[Int]` | `MonotonicMillis` | Read elapsed monotonic milliseconds since runtime initialization. |

## Boundaries and failure behavior

- All six concurrency operations are deferred IO descriptions. `parMapIO` also
  defers calling the mapper, and catches panics from either mapper construction
  or execution of its returned IO.
- Empty `parMapIO` returns generated `Nil` without calling the mapper.
- `parMapIO` and `parBoth` finish **all** tasks even when one fails. Their first
  observed failure is saved once, not chosen by input position. Concurrent ties
  have no deterministic ordering. A blocked sibling still prevents completion.
- `await` returns the original Go error unchanged. Panics in concurrent tasks
  become ordinary IO errors with text `panic: <value>` (`fmt` formatting), so
  `Bind` short-circuits normally. Later awaits return that same cached error.
  This includes panics in race losers and timed-out actions.
- `race` is first-completion, not first-success. Its buffered completion channel
  lets the losing goroutine publish its outcome and exit without an awaiter.
- `timeout` with `millis <= 0` returns `None` without starting its action.
  Positive values greater than `math.MaxInt64 / int64(time.Millisecond)` fail
  with `timeout: duration out of range` rather than overflowing the timer.
  When completion and expiry are both ready, the selected outcome is unspecified.
- A positive timeout starts its timer before launching the action. Successful
  completion is always wrapped in `Some`, even when the erased result is nil.
  Failure before expiry is not converted into `None`.
- The duplicated `sleep` has the same upper bound and returns
  `sleep: duration out of range` for positive overflow.
- Abandonment is **not cancellation**: losing/late IO continues and may perform
  effects until it finishes or the process exits. There is no cancellation or
  global fiber join. `rt.RunMain` is unchanged; it runs only `main`'s IO, and
  unawaited fibers do not keep the process alive. Awaited panic errors use the
  ordinary IO-failure exit path, not a direct panic in `RunMain`.

Diagnostics retain the existing `file:line:col: message` form. Examples of the
unchanged diagnostic machinery applied to this builtin are
`kind mismatch applying Fiber to 2 arguments`,
`type mismatch: expected Fiber[String], got Fiber[Int]`,
`duplicate declaration Fiber`, and `no instance Show[Fiber[Int]]`.

## Removed limitations

Programs can now fork, join, race, parallel-map, and time-bound IO without
externs in user code. The previous statement that the language has no concurrency
primitive no longer applies. No general exception syntax or mutable-reference
API is introduced.

## Duplicated names

- **io-effects:** the exact prelude externs `sleep(millis Int) IO[Unit]` and
  `monotonicMillis() IO[Int]`, plus Go implementations `rt.Sleep` and
  `rt.MonotonicMillis`, are needed to compile and measure the required example.
  They live in `rt/conc_time.go`, including its monotonic origin. Delete that
  entire duplicate file and one copy of each extern when integrating io-effects;
  keep the example and timing boundary tests against the owning implementations.
- **Collections / StdlibExtras shared shape protocol:** `Shapes.Tuple2`,
  `Shapes.Untuple2`, `rt_tuple2`, `rt_untuple2`, `rt.Tuple2Of`, the generated
  registration, and the fake-importer shape fields are intentionally identical
  shared additions. Keep one implementation on integration. These exact names
  follow the primary's final coordination decision.

## Deviations

None from the fixed Pass 12 concurrency signatures or behavior. Timeout duration
boundaries and `parBoth` failure/join behavior above specify previously unstated
cases. The shared tuple bridge is the primary-approved representation extension.

## Verification

Every Go validation command below ran through
`env SERIALIZE_VM_TESTS_MODE=shared /home/vagrant/.codex/skills/serialize-vm-tests/scripts/serialize-vm-tests --`.
The runner selected its exclusive tier under the VM memory guard.

- `go test ./types -run TestFiberType -count=1`: initially failed because `Fiber`
  was unknown and not reserved, establishing the missing builtin regression.
- `go test ./types ./codegen ./prelude ./rt`: passed. The first implementation
  run caught two new-test assumptions (the checker reports the full applied
  type; gofmt aligns neighboring function declarations). Those assertions were
  corrected to the existing contracts, without changing production behavior.
- `go test -race ./rt`: passed (`ok gofx/rt`, 3.067s).
- `go run ./cmd/gofx run examples/concurrency/main.gfx`: passed. Observed
  `parMapIO` wall time was 301 ms versus 600 ms total sleeps. The example README
  records complete output, including ordered results, repeated await, a mixed
  pair, the race winner, `None`, and `Some(ready)`.
- `go run ./cmd/gofx check examples/wc/main.gfx`: passed once, with no diagnostics.

Runtime tests cover concurrent/repeated await, IO failure identity and bind
short-circuiting, panic capture (including nil panic), lazy fork/map/race/timeout,
parallel task starts and result ordering, empty mapping, mapper panic, joining
remaining tasks after failure, both race winner positions, abandoned late panic,
pair shapes, timeout `Some`/`None`/failure/panic, and duration bounds. No full-repo
suite, vet, live-system commands, commits, or pushes were run.
