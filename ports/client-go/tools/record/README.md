# Native core/v1 event recording

Authority: client-go v0.37.2 at `72c52dd0cc3b9099b36af11290b0304045dd1ada`, `tools/record/event.go`, `events_cache.go`, `fake.go`, and `util/util.go`. Apache-2.0.

All event/reference/source values are the canonical dependency types. `EventSink` create/update callbacks return `IO[Outcome[Option[core.Event]]]`; patch additionally receives exact patch bytes as String. `eventSinkImpl` binds the generated core/v1 EventInterface's namespace-aware expansion methods, including their nullable early-error results.

## Capability mapping

| Upstream | Native |
| --- | --- |
| EventSink | EventSink callback record; eventSinkImpl typed-client constructor |
| EventRecorder / EventRecorderLogger | EventRecorder / EventRecorderLogger; event, eventf, annotatedEventf, withLogger callbacks |
| EventRecorderAdapter / NewEventRecorderAdapter | newEventRecorderAdapter; intentionally discards modern related/action fields, like upstream |
| EventBroadcaster / NewBroadcaster | EventBroadcaster / newBroadcaster(List[BroadcasterOption]) |
| NewBroadcasterForTests / NewBroadcasterWithCorrelatorOptions | newBroadcasterForTests / newBroadcasterWithCorrelatorOptions |
| BroadcasterOption / WithContext / WithSleepDuration / WithCorrelatorOptions | BroadcasterOption / withContext / withSleepDuration / withCorrelatorOptions |
| StartEventWatcher / StartRecordingToSink / StartLogging / StartStructuredLogging | lowerCamel free functions with explicit broadcaster |
| NewRecorder / Shutdown | newRecorder / shutdown |
| CorrelatorOptions / NewEventCorrelator / NewEventCorrelatorWithOptions | same lowerCamel constructors; explicit clock is required |
| EventCorrelator.EventCorrelate / UpdateState | eventCorrelate / updateState |
| EventCorrelateResult | EventCorrelateResult, preserving optional event, exact patch, skip and independent Outcome error |
| EventSourceObjectSpamFilter / NewEventSourceObjectSpamFilter / Filter | same native type, newEventSourceObjectSpamFilter, filterEvent |
| EventSpamKeyFunc / EventFilterFunc / EventAggregatorKeyFunc / EventAggregatorMessageFunc | typed native function aliases; filters are IO |
| EventAggregator / NewEventAggregator / EventAggregate | same native type, newEventAggregator, eventAggregate |
| EventAggregatorByReasonFunc / EventAggregatorByReasonMessageFunc | eventAggregatorByReasonFunc / eventAggregatorByReasonMessageFunc |
| FakeRecorder / NewFakeRecorder | FakeRecorder / newFakeRecorder / fakeRecorder capability; IncludeObject→includeObject |
| util.ValidateEventType / IsKeyNotFoundError / GenerateEventName | util.validateEventType / isKeyNotFoundError / generateEventName |

Go variadic formatting is replaced by an already-formatted String. Native callers can use interpolation; no heterogeneous universal value is needed. Annotations retain the dependency ForeignMap type. A reference without a namespace produces an event in `default`; the involved reference remains unchanged. Source component/host also populate reportingController/reportingInstance.

Correlation ordering is aggregation, count/patch observation, then spam filtering. Filtered observations still advance count. Independent bounded LRU caches preserve source/object rate buckets, aggregation groups and recorded names/resourceVersions. Defaults: 4096 entries; 10 distinct messages in a 600-second sliding interval; burst 25 at 1/300 events/second. Event keys include fieldPath, while aggregate/spam grouping omit it according to the upstream functions. Server success updates correlation state using the returned event key. Repeated events preserve first timestamp/name/resourceVersion and construct a strategic patch from the count/message/lastTimestamp-cleared baseline.

Construction and StatusError failures terminate legacy writes; NotFound patch falls back to create with resourceVersion cleared. Other failures permit twelve total attempts; only the first delay is randomized. Shutdown disables retries for queued events and closes/drains listeners. Context cancellation stops listeners and the broadcaster; explicit shutdown is idempotent.

Prepared, unexecuted deterministic evidence: `correlation_test.gt` covers duplicate/server-state patches, aggregation threshold, burst/refill and nil event errors; `record_test.gt` covers default namespace/source fields, shutdown and terminal construction errors. Shared internal tests cover FIFO, overflow and stopping. No build, test, formatter or lint command was run for this assignment.
