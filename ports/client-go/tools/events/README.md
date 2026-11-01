# Native events/v1 recording

Source authority: Kubernetes client-go v0.37.2, commit `72c52dd0cc3b9099b36af11290b0304045dd1ada`, `tools/events/{interfaces,event_broadcaster,event_recorder,helper,fake}.go`. Kubernetes-derived code retains Apache-2.0 licensing.

The event payload is the canonical `deps/k8s.io/api/events/v1.Event`; regarding/related objects use canonical core/v1 references. `EventSink` supplies create/update and strategic-merge patch callbacks returning `IO[Outcome[Option[Event]]]`. `newEventSink` adapts the generated typed EventsV1Interface. `eventSinkImpl` is the namespace-selecting callback constructor and rejects empty namespaces with the original operation-specific errors.

## Public capability mapping

| Upstream capability | Native API |
| --- | --- |
| EventRecorder, EventRecorderLogger, AnnotatedEventRecorder | aliases of `tools/internal/events.EventRecorder`; `eventf`, `annotatedEventf`, `withLogger` callbacks |
| EventSink, EventSinkImpl.Create/Update/Patch | `EventSink`, `eventSinkImpl`, `newEventSink` |
| NewBroadcaster | `newBroadcaster`; explicit-clock constructor `newBroadcasterWithClock` |
| NewRecorder | `newRecorder`; explicit optional Scheme and reporting controller, hostname-derived reporting instance |
| StartEventWatcher, watcher Stop | `startEventWatcher`, `stopWatcher` |
| StartLogging, StartStructuredLogging | `startLogging`, `startStructuredLogging`; explicit typed logger |
| StartRecordingToSink, StartRecordingToSinkWithContext | `startRecordingToSink`, `startRecordingToSinkWithContext`; implicit IO context replaces Go context arguments and explicit Context supports independent cancellation |
| Shutdown | `shutdown` cancels workers and joins them; stopped watchers are removed |
| EventBroadcasterAdapter and both constructors | `EventBroadcasterAdapter`, `newEventBroadcasterAdapter`, `newEventBroadcasterAdapterWithContext`; `adapterClients` adapts the real generated clientset |
| Adapter NewRecorder / DeprecatedNewLegacyRecorder / StartRecordingToSink / Shutdown | `adapterNewRecorder`, `deprecatedNewLegacyRecorder`, `adapterStartRecordingToSink`, `adapterShutdown` |
| GetFieldSelector | `getFieldSelector`, retaining v1/v1beta1/core prefix selection and unknown-version errors |
| FakeRecorder, NewFakeRecorder | `FakeRecorder`, `newFakeRecorder`, `fakeRecorder`; optional channel and verbose formatting retained |

Native formatting boundary: eventf receives an already formatted String instead of a Go variadic `interface{}` list. Annotation maps use the canonical foreign-map bridge; nil remains distinct from an empty map. Deprecated context-free and stop-channel entrypoints map to the explicit cancellation Context rather than duplicate aliases.

## Policy

Series identity includes type/action/reason/controller/instance and every field of both references, but excludes the note. First emission creates; second establishes count 2 and patches; further identical emissions increment cached count/time without immediate writes. Thirty-minute refresh renews series; six-minute idle expiry writes the final count and removes it only on a non-retry response. Singleton expiry does not write. New-event attempts do not apply server mutations to the cache. Refresh does apply successful server results, matching the pinned implementation.

Patch NotFound falls back to creation with an empty resourceVersion. AlreadyExists retries only for a series; other StatusError and RequestConstructionError responses terminate. Other failures retry at most twelve attempts with 10-second, 25%-jittered delay and cancellation. Retry policy is native; Go code under the shared internal package performs only external JSON/strategic-patch codecs, time conversion, entropy, hostname and logging.

## Prepared evidence (not executed)

`events_test.gt` prepares patch→404→create→409 traces, terminal request-construction failure, count 1→2→3 plus heartbeat/final write, namespace rejection and reference-key distinctions. `retry_test.gt` prepares the twelve-attempt transport-failure bound. Shared internal queue tests prepare FIFO/drain, stopped admission, and slow-listener overflow. These are unexecuted acceptance inputs, not a claim of passing validation.
