# Native retry

Authority: client-go v0.37.2 `util/retry/util.go` and `util_test.go`, revision `72c52dd0cc3b9099b36af11290b0304045dd1ada`, plus apimachinery's `wait.ExponentialBackoff`/`Backoff.Step` semantics. All retry, stepping, cap and error decisions execute in gofx.

## Public-symbol mapping

| Upstream | Native |
| --- | --- |
| DefaultRetry | defaultRetry() |
| DefaultBackoff | defaultBackoff() |
| OnError(backoff,retriable,fn) | onError(backoff,retriable,action) |
| RetryOnConflict(backoff,fn) | retryOnConflict(backoff,action) |

`Backoff` is a transparent alias to the **canonical** `deps/k8s.io/apimachinery/pkg/util/wait.Backoff`, not a second external type. `newBackoff(duration,factor,jitter,steps,cap)` constructs that snapshot through its canonical accessors. `defaultRetry()` uses 10ms/factor1/jitter0.1/five attempts; `defaultBackoff()` uses 10ms/factor5/jitter0.1/four attempts. `onErrorWithClock` injects a shared clock and typed random callback for deterministic tests. Durations are integer nanoseconds at native construction/stepping boundaries.

Actions have type `func() IO[Option[support.Error]]`, predicates have type `func(support.Error) Bool`, and the result is `IO[Option[support.Error]]`. `None` is success. The canonical Error handle preserves original error identity and wrapping; conflict detection calls `support.errorIsConflict`. No string matching, message-only translation or replacement error is used.

Each attempt calls the action once. Success and non-retriable failures return immediately. Retriable exhaustion returns the original last failure. Zero attempts return the canonical `wait.ErrWaitTimeout`. Step jitter affects the current sleep, not the next base duration. Hitting the cap sets remaining steps to zero *after the capped step's sleep*, matching the upstream loop. No extra sleep occurs after the last permitted attempt. Retry is not delegated to Go `wait` or original client-go.

## Written tests and integration

`util_test.gfx` covers immediate success/non-retriable exit, multi-attempt success, exhaustion with original identity, zero steps, cap stepping, positive conflict detection and preservation of the original context-cancelled error without a prior retriable error. These map both upstream tests, `TestRetryOnConflict` and `TestRetryOnConflictPreservesContextErrorWithoutLastError`, and add explicit no-attempt/cap checks.

Focused verification: the supervisor-assigned serialized `go run ./cmd/gofx test ports/client-go/util/retry` passed **4 tests**, including preservation of the caller's Backoff steps and original structured errors. The first attempt exposed a canonical-binding regression: the value-model Backoff had been classified as identity, removing the snapshot accessors. The dependency owner restored canonical snapshot mode; this package keeps pure constructors/defaults and never mutates the caller's configuration. A temporary identity adaptation was discarded. No formatter, linter or broader suite was run. Nonfatal unused-callback warnings remain. The only external-policy use is the canonical structured error predicate; retry scheduling remains native.
