# Native watch tools

Source authority: client-go v0.37.2 (`72c52dd0cc3b9099b36af11290b0304045dd1ada`), `tools/watch/{until,retrywatcher,informerwatcher}.go` and all three companion test files. Kubernetes-derived policy is Apache-2.0. No production import of client-go is used.

## Public symbol mapping

| Upstream exported symbol | Native implementation |
| --- | --- |
| `ConditionFunc` | `ConditionFunc = func(raw.Event) IO[Outcome[Bool]]` |
| `PreconditionFunc` | `PreconditionFunc[S] = func(S) IO[Outcome[Bool]]` |
| `ErrWatchClosed` | `errWatchClosed()`; message `watch closed before UntilWithoutRetry timeout` |
| `UntilWithoutRetry` | `untilWithoutRetry(raw.Watch,List[ConditionFunc])` |
| `Until` | `until(initialRV,Watcher,RetryAccess,Clock,conditions)` |
| `UntilWithSync` | `untilWithSync(InformerFactory[S],Option[PreconditionFunc[S]],conditions,Clock)` |
| `ContextWithOptionalTimeout` | `contextWithOptionalTimeout(Context,ns,warn) IO[(Context,Cancel)]`; cancel with `sync.cancelContext` |
| `RetryWatcher` | `RetryWatcher {watcher:raw.Watch,task:Task[Unit]}` |
| `NewRetryWatcher` (deprecated) | `newRetryWatcher(initialRV,Watcher,RetryAccess,Clock)`; native IO has an implicit caller context |
| `NewRetryWatcherWithContext` | `newRetryWatcherWithContext(initialRV,Watcher,RetryAccess,Clock,minRestartDelayNs)`; explicit context supplied with `sync.withContext` |
| `RetryWatcher.ResultChan` | `raw.next(retry.watcher)` / `raw.receiveCase(retry.watcher,mapper)` |
| `RetryWatcher.Stop` | `raw.stop(retry.watcher)`; strict completion, not cancellation acknowledgement |
| `RetryWatcher.Done` | `done(retry)`; actual task completion |
| `NewIndexerInformerWatcher` | `newIndexerInformerWatcher(InformerFactory[S])` |
| `NewIndexerInformerWatcherWithLogger` | `newIndexerInformerWatcherWithLogger(InformerFactory[S])`; logging is captured by the real controller factory rather than a second logger registry |
| returned indexer/controller/watch/done tuple | `InformerWatcher {store,hasSynced,watcher,task}` |

`cacheInformerFactory` connects the actual native cache shared informer, with zero resync period and empty indexers. It installs handlers before run. Deletion tombstones forward their retained object; a tombstone without an object raises a typed panic rather than fabricating an object. The generic factory supports other typed cache implementations without REST reach-through.

`canonicalRetryAccess` supplies canonical dependency projections. Its RV getter uses only `GetResourceVersion`, not full metadata access. Error events go through `apierrors.FromObject`, including unstructured status. `RetryAccess` is a typed external-access capability, not a replacement policy implementation.

## Behavior

Conditions run sequentially and the previous matching event is immediately offered to the next condition. Error and EOF retain the last event. Captured-context `tryIO` under an uninterruptible owner maps cancellation/timeout to the canonical interrupted error; cleanup always stops the watch. Unknown strings, nil objects and status objects are not filtered by the raw leaf or conditions.

Retry watches reject initial RV `""` and `"0"`. The successful last delivered event or bookmark supplies the next opaque RV: no numeric comparison or maximum. Bookmarks advance RV but are not delivered. Empty/missing RV and unknown event kinds produce an internal-error event and terminate. Stream 410 forwards the original event and terminates; other status errors retry with integer-nanosecond RetryAfterSeconds. Establishment 410 retries, unlike stream 410. Forbidden/unauthorized establishment emits the original status when available and terminates. Nil watches and other establishment errors retry. Every acquired watch is bracketed, including partial watch-plus-error outcomes. Restart periods are non-sliding and include server retry delay.

The informer event processor uses a condition-protected unbounded FIFO (reverse-on-batch), preserving event order while decoupling callbacks from consumers. Its stop channel wins against blocked output without stranding the producer. Stopping the public watch cancels and joins the controller, stops the pump and joins it. `untilWithSync` checks the optional precondition only after `hasSynced`; true returns no event, false begins ordinary conditions. No precondition means no separate initial-sync wait, matching upstream.

The optional-timeout policy logs the negative-timeout warning, maps negative/zero to cancellation without a deadline, and retains positive nanoseconds exactly.

## Regression mapping (not executed in this assignment)

- `until_test.go` sequential/repeated conditions, EOF, callback error and cancellation → `watch_test.gfx` conditions/last-event/stop cases.
- `retrywatcher_test.go` constructor validation, restarts, bookmark RV, missing RV, unknown event/status, Gone and unread stop → `watch_test.gfx:testOrderedRetryAndBookmarks`, `retrywatcher_test.gfx` cases. Canonical-object cases take real external object/access fixtures rather than nominal fake runtime types.
- `informerwatcher_test.go` processor order/blocked exit and initial synchronization → `watch_test.gfx:testEventProcessorBlockedStopAndOrdering`, `testInitialSyncBarrier`; real cache wiring is `cache_adapter.gfx`.

The raw support leaf also backs native `newFake`, `newFakeWithChanSize`, `newEmptyWatch` and `newProxyWatcher`. Fake sends retain channel backpressure and closed-send failures; stop is idempotent. Proxy stop signals its producer and does not close a caller-owned events channel.
