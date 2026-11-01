# Native leader election

Native go4t implementation of `k8s.io/client-go/tools/leaderelection` and
`resourcelock`, pinned to v0.37.2 / revision
`72c52dd0cc3b9099b36af11290b0304045dd1ada`. Kubernetes-derived source is
Apache-2.0. This is a go4t API, not a Go-interface compatibility wrapper.
There is no production dependency on the original client-go implementation.

## Construction and boundaries

- `resourcelock.LeaseClient` is a namespace-scoped capability containing get,
  create, and update callbacks over canonical coordination/v1 `Lease` objects.
  `leaseClient` adapts the generated `typed/coordination/v1.LeaseInterface`.
- `new` and `newWithLabels` construct the only factory-supported lock in the
  pinned release, `leases`. The removed endpoints/configmaps/composite factory
  strings return their original migration errors. `multiLock(primary,secondary)`
  still implements the exported compositional MultiLock behavior; it does not
  resurrect removed resource implementations.
- `newFromKubeconfig` copies the immutable REST configuration, applies
  `max(1 second, renewDeadline / 2)` and the `leader-election` user agent, and
  constructs a real generated coordination client. No source config is mutated.
- `LeaderElectionConfig` contains a required lock and required started/stopped
  callbacks, an optional new-leader callback and health adapter, three Int
  nanosecond durations, releaseOnCancel, name, and coordinated mode. Required
  native values replace nullable Go interfaces/functions; invalid timing and
  empty identity preserve the original validation error text and order.
- `newLeaderElector(config,clock,metricsRegistry)` returns
  `IO[Outcome[Option[LeaderElector]]]`. `runOrDie` installs the optional health
  adapter and runs; failed construction raises the original domain error.
- The `MetricsRegistry` is an explicitly shared application aggregate. The first
  `setProvider` wins. A new elector emits initial Off; acquisition emits On;
  slow-path renewal emits SlowpathExercised. Finalization turns the metric off.
- Candidate construction is either `newCandidate` with injected `CandidateClient`
  and `CandidateInformer`, or `newCandidateForClient` with an explicit
  `features.FeatureRegistry` first argument and the actual generated clientset.
  The generated informer is separate from leader-elected application
  informers, selects `metadata.name`, scopes namespace, and resyncs at five minutes.
  Its cache-sync waiter tracks the installed handler, including initial delivery.
  The registry is passed through to the generated informer so feature gate
  overrides are resolved at reflector creation/run, rather than frozen here.

## State and ordering

The lease lock caches the last canonical Lease snapshot and resourceVersion.
Get replaces it, Create retains the returned snapshot, and Update sends its
resourceVersion unchanged; API conflicts remain original structured errors.
Only explicitly configured labels are merged, preserving unrelated labels and
nil/empty distinctions. LeaseSpec conversion preserves optional strategy and
preferredHolder, and uses the upstream int32 narrowing behavior. Record JSON
has original field names/order, exact integer text, and metav1 Time encoding;
no integer is serialized through a Float.

The election aggregate keeps a separate monotonic local `sync.Instant` for
observation. Wire acquire/renew timestamps never determine expiry. Unchanged
raw records do not refresh observation. Normal validity is strictly before
expiry; coordinated mode rejects strictly after expiry, preserving the boundary
asymmetry. Optimistic renewal falls through to Get on any failed update. Slow
acquisition preserves acquire time/transition count for the holder and increments
transitions only when taking over. Coordinated mode never creates or steals a
lease and refuses renewal when preferredHolder marks end-of-term.

Release always refetches before testing local leadership, writes the empty
holder with a one-second duration, preserves transitions, and retries conflicts
within a clock-driven renewDeadline. Timers are bracketed and stopped, and
clock-driven cancellation interrupts in-flight requests. Run brackets successful
acquisition, cancels the started callback's context on exit, releases when
configured, emits stopped-leading, then calls OnStoppedLeading even if no
acquisition occurred. OnNewLeader is asynchronous and deduplicated by reported
identity. This protocol does **not** provide fencing or guarantee one running
critical section; callers must finish guarded work before requesting release.

Health is healthy before attachment or while observing another holder. For the
local holder it fails strictly after leaseDuration+tolerance using the local
observation clock. Error text is
`failed election to renew leadership on lease <name>`.

The candidate worker uses the native typed rate-limiting workqueue, one key,
Done finalization, five-minute successful requeue, and the default controller
rate limiter on errors. It creates missing candidates and otherwise updates
only RenewTime on the cached canonical snapshot. Update events enqueue only when
PingTime is after RenewTime. Candidate informer tasks are cancelled and joined;
the queue and its delaying worker shut down on all exits.

## Upstream symbol/behavior mapping

| Pinned source | Native capability |
| --- | --- |
| leaderelection.go: JitterFactor, LeaderElectionConfig, LeaderCallbacks, LeaderElector | jitterFactor, same-named typed records |
| NewLeaderElector, Run, RunOrDie, GetLeader, IsLeader, Check | newLeaderElector, run, runOrDie, getLeader, isLeader, check |
| acquire, renew, release, tryRelease, tryAcquireOrRenew, tryCoordinatedRenew | same lower-camel functions |
| observation locking, isLeaseValid, maybeReportTransition | getObservation/setObservedRecord, local validity predicates, maybeReportTransition |
| healthzadaptor.go: HealthzAdaptor, NewLeaderHealthzAdaptor, Name, Check, SetLeaderElection | HealthzAdaptor, newLeaderHealthzAdaptor, healthName, healthCheck, setLeaderElection |
| metrics.go: LeaderMetric, MetricsProvider, SetProvider | typed callback records, explicit MetricsRegistry/setProvider |
| leasecandidate.go: LeaseCandidate, CacheSyncWaiter, NewCandidate, Run | LeaseCandidate, CacheSyncWaiter, newCandidate/newCandidateForClient, runCandidate |
| ensureLease, newLeaseCandidate, enqueueLease, processNextWorkItem | same lower-camel functions; native queue |
| resourcelock/interface.go: record, recorder, config, Interface, constants, constructors | canonical record/capabilities, constant functions, new/newWithLabels/newFromKubeconfig |
| resourcelock/leaselock.go: LeaseLock, methods, conversion functions | LeaseLock, get/create/update/recordEvent/asInterface, lower-camel conversions |
| resourcelock/multilock.go: MultiLock, UnknownLeader, ConcatRawRecord | multiLock capability composition, unknownLeader, concatRawRecord |

## Deterministic regression sources

The test modules contain consumer-visible controlled-lock and fake-clock traces:

- acquire, optimistic renew, conflict fallback, transition/acquire-time retention;
- stale observation despite future remote timestamps and exact expiry takeover;
- release refetch and conflict retry, empty-holder/one-second record;
- coordinated missing lease, assigned holder, strategy retention, preferred holder;
- health attachment, strict tolerance boundary, standby behavior;
- cancellation release-before-stopped callback, stop without acquisition,
  clock-driven interruption of an in-flight renewal;
- first-provider metrics, initial Off/acquisition On/slowpath ordering;
- asynchronous new-leader deduplication and timing validation;
- candidate create, ping acknowledgment, version preservation, queue retry,
  handler-registration failure cleanup;
- resourceVersion conflicts/refetch, label merge, record encoding without numeric
  truncation, lease narrowing, removed factories, MultiLock unknown leader.

Source mapping: upstream `leaderelection_test.go`, `healthzadaptor_test.go`,
`leasecandidate_test.go`, and `resourcelock/leaselock_test.go` were inspected.
No acceptance command or live lease operation was run in this implementation
assignment. Main must run focused native tests for this directory and
`resourcelock` after shared/generated dependency integration. Compilation and
behavior are not yet claimed as observed verification.
