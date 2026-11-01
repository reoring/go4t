# Explicit client metric extensions

Native counterpart of client-go v0.37.2 `tools/metrics/metrics.go` (Apache-2.0). No process-global mutable metric registry is installed. Applications create a `Registry` and pass it to REST/transport/credential/certificate consumers.

`newRegistry`, `register`, `ensureRegistered`, `provider`, `emptyOptions`, and `noopProvider` cover the upstream Register/EnsureRegistered/global-provider capability. The first whole RegisterOpts wins, even if all fields are absent. Absent fields use the upstream-equivalent no-op metrics. An absent RegisterFn does not consume the deferred once. Concurrent ensure callers wait for the winning callback to finish; its completion is recorded even when its IO fails. The callback runs outside the registry mutex. A consumer can explicitly supply a Provider override without changing registry ownership. Provider resolution reuses the recorded provider rather than rebuilding callback records on every observation.

## Upstream-to-native field/type map

All callbacks return IO and receive the implicit cancellation/context environment.

| Upstream metric global / RegisterOpts field | Native Provider / RegisterOpts field | Capability |
| --- | --- | --- |
| ClientCertExpiry | clientCertExpiry | ExpiryMetric.set(Option[sync.Instant]) |
| ClientCertRotationAge | clientCertRotationAge | DurationMetric.observe(Int nanoseconds) |
| RequestLatency | requestLatency | LatencyMetric.observe(verb, canonical URL, Int nanoseconds) |
| ResolverLatency | resolverLatency | ResolverLatencyMetric.observe(host, Int nanoseconds) |
| RequestSize | requestSize | SizeMetric.observe(verb, host, Float bytes) |
| ResponseSize | responseSize | SizeMetric.observe(verb, host, Float bytes) |
| RateLimiterLatency | rateLimiterLatency | LatencyMetric |
| RequestResult | requestResult | ResultMetric.increment(code, method, host) |
| ExecPluginCalls | execPluginCalls | CallsMetric.increment(Int exitCode, callStatus) |
| ExecPluginPolicyCalls | execPluginPolicyCalls | PolicyCallsMetric.increment(status) |
| RequestRetry | requestRetry | RetryMetric.incrementRetry(code, method, host) |
| TransportCacheEntries | transportCacheEntries | TransportCacheMetric.observe(Int) |
| TransportCreateCalls | transportCreateCalls | TransportCreateCallsMetric.increment(result) |
| TransportCAReloads | transportCAReloads | TransportCAReloadsMetric.increment(result, reason) |
| TransportCertRotationGCCalls | transportCertRotationGCCalls | TransportCertRotationGCCallsMetric.increment() |
| TransportCacheGCCalls | transportCacheGCCalls | TransportCacheGCCallsMetric.increment(result) |
| RegisterFn | registerFn | Option[func() IO[Unit]] |

`RegisterOpts` wraps metric fields in Option; `Provider` exposes resolved non-optional capabilities. There is no silent replacement of a supplied metric. Registry sharing and consumer overrides are explicit.

`metrics_test.gt` prepares first-registration/deferred-once, ensure-before-register, explicit override, and concurrent completion-wait traces. Tests are intentionally not executed in this worker assignment. Wiring into other owners' consumers must use the shared contract published at `local://clientgo-contract-Events.md`.
