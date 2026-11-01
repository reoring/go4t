# Native workqueue

Port source: client-go v0.37.2, revision `72c52dd0cc3b9099b36af11290b0304045dd1ada`. Policy, heap, deduplication, metrics and worker loops are gofx. The only host effects used are the shared `support/sync` primitives. No production client-go import or locally defined synchronization runtime is used.

## Native API contract

`newBy[A](func(A,A) Ordering) IO[Typed[A]]` accepts any item type with an explicit identity ordering. `new[A Ord]()` is the convenience form. `newWithConfigBy`/`newWithConfig` accept `QueueConfig[A]`. Comparison equality defines item identity; the comparator must remain stable while an item is queued, dirty, processing or tracked by a rate limiter. Keys are not restricted to strings.

`ReadyQueue[A]` is an injectable record of `touch`, `push`, `pop`, and `length` callbacks. All execute inside the owning queue lock and must not reenter that queue. The default FIFO uses front/back persistent lists and releases removed references. `QueueInterface[A]` provides the full queue capability for custom delayed-queue injection. `DelayingInterface[A]` adds an `addAfter` callback and permits arbitrary custom delaying implementations in `RateLimitingQueueConfig[A]`.

All duration arguments/results are integer nanoseconds. `get(q)` returns `IO[Option[A]]`: `None` means empty and shut down, replacing Go's zero item plus shutdown boolean. `Some` must be paired with `done(q,item)`. `length` is informational, not a synchronization gate. Shutdown rejects future Add but allows ready items to be consumed. Done can requeue dirty work even after shutdown.

The drain predicate is exactly `processing != empty && drain`; pending ready items do **not** extend this wait. An ordinary `shutDown` clears `drain` and broadcasts, releasing concurrent drain calls. No stronger pending-empty condition is imposed.

Delayed input has capacity 1000 and uses atomic send-versus-stop selection. Its indexed binary min-heap has one entry per identity; updating a deadline only moves it earlier. Heap positions and the identity index are persistent maps. No stale-heap-entry accumulation or per-item goroutines are used. Equal deadlines have no extra FIFO promise. The next timer is bounded by the upstream ten-second heartbeat interval; shutdown does not promote unexpired work.

`RateLimiter[A]` has `when: func(A) IO[Int]`, `forget: func(A) IO[Unit]`, and `numRequeues: func(A) IO[Int]`. Exponential and fast/slow constructors have comparator and Ord forms. Max-of evaluates **every** child; max-wait caps only the returned delay. Bucket reservations are global to the bucket, return zero requeues and retain their reservation on Forget. Default controller policy is max-of 5ms/1000s exponential and a 10 QPS/100 burst bucket; default item policy is 1ms/1000s exponential. Forget neither calls Done nor removes already delayed items.

`parallelizeUntil(workers,pieces,work,options)` claims chunks under one lock and starts at most min(workers,chunks) tasks, not one task per piece. `withChunkSize` returns an immutable option. Workers check the captured original cancellation context before each callback; all workers are joined by an uninterruptible coordinator, while callbacks run with the original context. Already-running user callback semantics remain those of its IO action.

### Metrics registry decision

Gofx has no mutable top-level binding. The supervisor-approved equivalent of upstream's process-global provider is an explicit `MetricsRegistry`: create one per application, share it in every `QueueConfig`/`DelayingQueueConfig`/`RateLimitingQueueConfig`, then call `setProvider(registry,provider)`. Only the first assignment wins within that shared registry. A per-queue provider overrides it. Unnamed queues, and constructors without an application registry/provider, remain no-op and start no metric task. There is no hidden global registry or unsafe string-keyed host storage.

Metric capabilities remain typed records: GaugeMetric, CounterMetric, SettableGaugeMetric, HistogramMetric and SummaryMetric. Providers create all seven upstream metric families. Depth tracks dirty transitions, not merely ready length. Timing retains nanosecond clock precision before conversion to seconds. Metrics execute under the queue lock and shutdown joins the periodic worker.

## Exported production symbol mapping

Go methods map to lower-camel free functions with explicit receiver, except interface methods which map to callback fields. Deprecated Go generic/non-generic aliases share the **same native implementation**, not compatibility wrappers.

| Upstream symbols | Native equivalent |
| --- | --- |
| Interface, TypedInterface; Type, Typed | Typed[A], QueueInterface[A], asInterface |
| Queue; DefaultQueue; Queue.Touch/Push/Len/Pop | ReadyQueue[A]; defaultQueue; touch/push/length/pop callbacks |
| QueueConfig, TypedQueueConfig | QueueConfig[A] |
| New, NewTyped | new / newBy |
| NewWithConfig, NewTypedWithConfig, NewNamed | newWithConfig / newWithConfigBy; config.name |
| Typed.Add/Len/Get/Done/ShutDown/ShutDownWithDrain/ShuttingDown | add/length/get/done/shutDown/shutDownWithDrain/shuttingDown |
| DelayingInterface, TypedDelayingInterface | DelayingInterface[A], DelayingQueue[A], asDelayingInterface |
| DelayingQueueConfig, TypedDelayingQueueConfig | DelayingQueueConfig[A] |
| NewDelayingQueue, NewTypedDelayingQueue, TypedNewDelayingQueue, NewDelayingQueueWithConfig, NewTypedDelayingQueueWithConfig | newDelayingQueue / newDelayingQueueBy with config |
| NewNamedDelayingQueue, NewDelayingQueueWithCustomClock, NewDelayingQueueWithCustomQueue | same constructor, config.name/clock/queue |
| AddAfter; delaying ShutDown | addAfter; shutDownDelaying (also adapter queue.shutDown) |
| RateLimitingInterface, TypedRateLimitingInterface | RateLimitingQueue[A] plus its delaying.queue capability |
| RateLimitingQueueConfig, TypedRateLimitingQueueConfig | RateLimitingQueueConfig[A] |
| NewRateLimitingQueue, NewTypedRateLimitingQueue, NewRateLimitingQueueWithDelayingInterface | newRateLimitingQueue(limiter,delayingCapability) |
| NewRateLimitingQueueWithConfig, NewTypedRateLimitingQueueWithConfig, NewNamedRateLimitingQueue | newRateLimitingQueueWithConfig / newRateLimitingQueueWithConfigBy; config.name |
| AddRateLimited, Forget, NumRequeues | addRateLimited, forget, numRequeues |
| RateLimiter, TypedRateLimiter | RateLimiter[A] callback record |
| BucketRateLimiter, TypedBucketRateLimiter | newBucketRateLimiter with native flowcontrol.TokenBucket |
| ItemExponentialFailureRateLimiter, TypedItemExponentialFailureRateLimiter; NewItemExponentialFailureRateLimiter, NewTypedItemExponentialFailureRateLimiter | newItemExponentialFailureRateLimiter / newItemExponentialFailureRateLimiterBy |
| ItemFastSlowRateLimiter, TypedItemFastSlowRateLimiter; NewItemFastSlowRateLimiter, NewTypedItemFastSlowRateLimiter | newItemFastSlowRateLimiter / newItemFastSlowRateLimiterBy |
| MaxOfRateLimiter, TypedMaxOfRateLimiter; NewMaxOfRateLimiter, NewTypedMaxOfRateLimiter | newMaxOfRateLimiter |
| WithMaxWaitRateLimiter, TypedWithMaxWaitRateLimiter; NewWithMaxWaitRateLimiter, NewTypedWithMaxWaitRateLimiter | newWithMaxWaitRateLimiter |
| DefaultControllerRateLimiter, DefaultTypedControllerRateLimiter | defaultControllerRateLimiter / defaultControllerRateLimiterBy |
| DefaultItemBasedRateLimiter, DefaultTypedItemBasedRateLimiter | defaultItemBasedRateLimiter / defaultItemBasedRateLimiterBy |
| Every limiter When/Forget/NumRequeues method | when/forget/numRequeues callbacks |
| GaugeMetric, CounterMetric, SettableGaugeMetric, HistogramMetric, SummaryMetric; their Inc/Dec/Set/Observe methods | same named capability records; inc/dec/set/observe |
| MetricsProvider; NewDepthMetric, NewAddsMetric, NewLatencyMetric, NewWorkDurationMetric, NewUnfinishedWorkSecondsMetric, NewLongestRunningProcessorSecondsMetric, NewRetriesMetric | MetricsProvider and corresponding lowerCamel creator fields |
| SetProvider | setProvider(registry,provider) |
| DoWorkPieceFunc, Options, WithChunkSize, ParallelizeUntil | DoWorkPieceFunc, Options, withChunkSize, parallelizeUntil |

`doc.go` is represented by this contract. `queue.go`, `delaying_queue.go`, `rate_limiting_queue.go`, `default_rate_limiters.go`, `metrics.go` and `parallelizer.go` have native implementation here (the rate-limiting aggregate shares `delaying_queue.gfx`).

## Retained behavior coverage

The native tests below were executed in the supervisor-assigned focused acceptance pass; they are not a claim that every upstream benchmark or host observation has been reproduced.

- `queue_test.go`: Basic/AddWhileProcessing/Len/Reinsert/Collapse/CollapseWhileProcessing map to duplicate/reinsert, injectable-touch and length tests; drainage/no-drain/forced-drain/dirty-after-shutdown map to explicit lifecycle tests. GarbageCollection's removed-item ownership is implemented by FIFO removal rather than Go finalizer assertions. Queue benchmark is an upstream performance oracle, not a native pass claim.
- `delaying_queue_test.go`: SimpleQueue/Deduping/AddTwoFireEarly/CopyShifting map to zero-delay/shutdown, heap ordering/replacement, and real worker earlier-wins tests with fake-clock/input barriers. Its benchmark remains a performance oracle.
- `default_rate_limiters_test.go`: exponential overflow/counts/reset, fast/slow threshold, every-child max-of and max-wait map to `default_rate_limiters_test.gfx`. Test-only StepRateLimiter/NewStepRateLimiter is represented by injected capability callbacks, not a production API.
- `rate_limiting_queue_test.go`: all delay forwarding, per-item counts and Forget semantics map to `rate_limiting_queue_test.gfx`.
- `metrics_test.go`: dirty depth, 50µs latency, 25µs processing, unfinished/oldest seconds, first-provider/override and shutdown behavior map to native metric tests and joined shutdown.
- `parallelizer_test.go`: all four original visitation configurations are retained; added three-worker cancellation barrier verifies bounded callback entry and joined exit. Benchmark remains an upstream performance oracle.

Focused verification: serialized `go run ./cmd/gofx test ports/client-go/util/workqueue` passed **18 tests**, including the retained cancellation-cleanup test asserting all three callback finalizers completed before return. The initial run exposed an unparenthesized `&& match` expression in a test; that syntax was corrected. `unit` values were corrected to calls to `unit()` throughout the owned source/tests, as required by SPEC. The implicit prelude exports `reverse`, `min`, and `max` (SPEC §2.3); their unqualified calls are valid. Constructor publication is masked across task startup and task-handle registration; blocking Get/drain waits remain interruptible.

The supervisor-assigned temporary native consumer also passed duplicate/Get/readd/Done single-requeue, force-released drain, fake-clock delayed earlier-wins, rate count/reset and joined worker cancellation. The smoke source and its generated Go directory were removed. No formatter, linter or broader suite was run. Unused-callback/defaulted-Unit and the intentional prelude `length` shadow warnings remain nonfatal. Upstream performance and GC-specific host observations are not claimed as exercised.
