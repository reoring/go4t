# Native flowcontrol

Authority: client-go v0.37.2 (`72c52dd0cc3b9099b36af11290b0304045dd1ada`), `throttle.go`, `backoff.go`, and both corresponding test files. All token-accounting and backoff decisions are native gofx; synchronization and timers use the shared `support/sync` effects. Nanosecond durations are Int, QPS/jitter are Float.

## Capabilities and behavior

`PassiveRateLimiter` carries `tryAccept: func() IO[Bool]`, `stop: func() IO[Unit]` and `qps: Float`. `RateLimiter` carries that passive capability and `accept`/`wait` actions. `Clock` is an injectable pair of `now` and `sleep` callbacks; `clockCapabilities(sync.Clock)` adapts the shared real/fake clock, and `newTokenBucket*WithCapabilities` accepts an arbitrary typed clock. No second synchronization runtime is introduced.

The bucket starts full, refills only up to burst, and serializes reservations. Immediate rejection does not mutate the bucket. Reservations consume future capacity; cancellation restores only capacity not consumed by later reservations. Duration conversion uses the same floating token rate and Int64 cap as the upstream x/time/rate algorithm. `Accept` uses the injected clock; `Wait` deliberately uses real time and the implicit caller context, matching upstream's separate `rate.Limiter.Wait` path. Wait rejects requests exceeding burst or the current deadline before committing a reservation, and restores reservations if cancelled. Timer allocation/cleanup uses shared `bracket`; caller-context inspection and rollback run inside an uninterruptible coordinator with explicitly restored original context for the wait.

Exact user-visible native IO failure messages include `rate: Wait(n=1) exceeds limiter's burst N`, `rate: Wait(n=1) would exceed context deadline`, and fake-never's `can not be accept`. Context cancellation is propagated as the existing typed IO cancellation, not matched through error text.

Upstream token-bucket `Stop` is a **no-op**, despite the PassiveRateLimiter comment suggesting otherwise. The native implementation preserves the executable behavior. Fake-always succeeds, QPS=1. Fake-never rejects TryAccept, blocks Accept until Stop, and Wait fails immediately with the upstream message.

`Backoff[A]` owns a mutex and a persistent comparator-keyed map. `newBackOffBy` takes a comparator, initial/max nanoseconds, shared clock, jitter factor, random callback and expiry predicate. Ord conveniences cover the upstream constructors. The immutable record exposes clock/random/expiry policy for caller customization. `next` uses eventTime for expiration but the current clock for lastUpdate. Initial jitter is not capped, subsequent doubling plus jitter is capped. Default expiration is strictly `eventTime-lastUpdate > 2*maximum`, not `>=`. `isInBackOffSince` and `isInBackOffSinceUpdate` preserve their distinct time comparisons. GC deletes only expired entries; reset/delete remove one key.

## Exported symbol mapping

| Upstream symbols | Native equivalent |
| --- | --- |
| PassiveRateLimiter.TryAccept/Stop/QPS | PassiveRateLimiter.tryAccept/stop/qps |
| RateLimiter.Accept/Wait and embedded PassiveRateLimiter | RateLimiter.accept/wait/passive |
| Clock (Now/Sleep and passive-clock capability) | Clock.now/sleep plus shared Instant arithmetic; clockCapabilities |
| NewTokenBucketRateLimiter | newTokenBucketRateLimiter |
| NewTokenBucketPassiveRateLimiter | newTokenBucketPassiveRateLimiter |
| NewTokenBucketRateLimiterWithClock | newTokenBucketRateLimiterWithClock; arbitrary callback clock via newTokenBucketRateLimiterWithCapabilities |
| NewTokenBucketPassiveRateLimiterWithClock | newTokenBucketPassiveRateLimiterWithClock; arbitrary callback clock via newTokenBucketPassiveRateLimiterWithCapabilities |
| NewFakeAlwaysRateLimiter, NewFakeNeverRateLimiter | newFakeAlwaysRateLimiter, newFakeNeverRateLimiter |
| Backoff; Backoff.Clock, Backoff.HasExpiredFunc | Backoff[A]; clock, hasExpired immutable policy fields |
| NewFakeBackOff, NewBackOff, NewFakeBackOffWithJitter, NewBackOffWithJitter | newFakeBackOff, newBackOff, newFakeBackOffWithJitter, newBackOffWithJitter; fully explicit comparator/policy via newBackOffBy |
| Backoff.Get/Next/Reset/IsInBackOffSince/IsInBackOffSinceUpdate/GC/DeleteEntry | get/next/reset/isInBackOffSince/isInBackOffSinceUpdate/gc/deleteEntry |
| Backoff embedded RWMutex synchronization | aggregate-owned mutex; callers no longer manually mutate internal maps |

## Written regression coverage

`flowcontrol_test.gfx` retains burst/refill, token Stop no-op, reservation cancellation, always/never, exact expiry, high-water/reset/GC, custom expiry/jitter, distinct since/since-update windows and the upstream 60-step precision case with an injected advancing clock. Workqueue tests additionally exercise bucket future reservation delays and multiconsumer bounded visitation.

Mapping: upstream TestBasicThrottle/TestIncrementThrottle/TestThrottle/TestMultithreadedThrottling → serialized burst/refill/reservation cases; TestAlwaysFake/TestNeverFake → fake lifecycle; TestWait → context deadline/rollback implementation plus integration-context checks; TestRatePrecisionBug → testRatePrecisionAndInjectedSleep. Backoff's TestSlowBackoff/TestBackoffReset/TestBackoffHighWaterMark → exponential/reset/exact-expiry; TestBackoffGC/TestAlternateBackoffGC/TestAlternateHasExpiredFunc → custom GC; TestIsInBackOffSinceUpdate → window comparison; TestBackoffWithJitter → deterministic bounded jitter callback.

Focused verification: the supervisor-assigned serialized `go run ./cmd/gofx test ports/client-go/util/flowcontrol` passed **7 tests**. The source uses the correct zero-argument `unit()` call. No formatter, linter or broader suite was run. Nonfatal unused-callback and defaulted-Unit warnings remain.

The additional supervisor-assigned **real token Wait proof** passed all three scenarios through the serialized command `go run ./cmd/gofx run ports/client-go/util/flowcontrol/waitproof`: available-token success with exact token state; a captured 100ms deadline rejecting a 10000-second wait with every reservation-state field unchanged; and cancellation of an observed committed reservation, actual task join, exact restoration according to `min(burst, advancedTokens + 1)`, and the exact subsequent reservation delay. There are no sleep-based readiness assertions: cancellation waits for negative committed token state under a bounded 1000ms context. These regressions are retained in `wait_test.gfx`; the throwaway consumer and generated directory were deleted after execution. No production policy change was needed.
