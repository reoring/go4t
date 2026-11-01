# Native REST client

Port target: client-go v0.37.2, upstream revision `72c52dd0cc3b9099b36af11290b0304045dd1ada`. Apache-2.0.

## Native contract

Requests are immutable and retain the first construction error. File/object/parameter codec builders are explicit IO and retain their errors until execution. HTTP identities, API objects, serializers, and errors are canonical `deps` values; transport, authentication, feature gates, clocks and limiter capabilities are injected. `support/outcome` retains partial values with independent optional errors. HTTP request policy, retries, backoff, warning dispatch, and configuration policy live in gofx. Host packages contain standard HTTP/URL/stream effects and external Kubernetes codecs, never client-go imports.

A `Stream` owns its response until `close`. A watch returns the shared `support/watch.Watch`: EOF closes its channel; stop closes its decoder once and joins its worker. No alternate watch type is exported. `copyConfig` is IO because an exec plugin extension object needs external DeepCopyObject.

`HTTPClient` aliases the canonical external `http.Client`. ConfigAndClient/newRESTClient/newRequestWithClient retain the supplied instance without reallocation, preserving Timeout, redirects, and CookieJar. HTTPClientFor constructs a standard client over the native Transport bridge only when required and otherwise reuses http.DefaultClient. `Environment.authentication` injects auth plugin composition without a REST-to-plugin import cycle.

For an unversioned client with no explicit GroupVersion, URL construction retains the empty version while content negotiation uses `meta.k8s.io/v1` (`metav1.SchemeGroupVersion`), as pinned rest/config.go:470-478 requires. This is not the core API group `/v1`. An explicitly supplied GroupVersion takes precedence for both paths.

### URL backoff configuration

`newURLBackoff(clock sync.Clock, initial Int, maximum Int) IO[URLBackoff]`
keeps the zero-jitter default and uses the supplied clock. Durations are
nanoseconds. To supply an existing native aggregate, use
`newURLBackoffWithBackoff(backoff flowcontrol.Backoff[String]) IO[URLBackoff]`,
then `urlBackoff(adapter) Backoff`. For example, `flowcontrol.newBackOffBy`
accepts the clock, jitter factor, random callback and
`hasExpired: func(sync.Instant,sync.Instant,Int) Bool`. The adapter preserves
that aggregate and its existing entries; updates are visible through both APIs.

`URLBackoff` stores only `backoff: sync.Ref[flowcontrol.Backoff[String]]`.
Flowcontrol alone owns entries, locking, growth, jitter and expiry. REST maps
URLs to their host (including port), advances on 429 or status >499, and resets
on other statuses even when an error is supplied. Calculation reads the stored
delay without expiring it. Sleep uses the current aggregate's clock and remains
context-cancellable. `disable` replaces the aggregate with an empty, zero-delay
native backoff on that clock; existing `Backoff` capabilities observe replacement,
while a caller's original supplied aggregate is not cleared or modified.

`urlbackoff_test.gfx` proves supplied state sharing, deterministic jitter, custom
expiry, default growth/cap/expiry boundaries, host/port keys, reset, disable and
cancellable sleep. After the shared `FormatBytes` extern ABI correction,
`gofx test rest` passed all 21 tests, including all three URL backoff cases.
A temporary public-import consumer also passed the customized aggregate path;
its source was archived outside the shipped examples and removed after execution.
Both checks used the current compiler with default optimization.

## Source capability map

Every exported declaration in the owned production source inventory is listed below. Implicit-context variants deliberately share one native capability; Go pointer-output accessors become direct native return values. Private receiver types with exported methods map to their enclosing native capability rather than exporting private implementation types.

| Pinned source | Exported capability | Native equivalent |
|---|---|---|
| `rest/client.go:47` | `Interface` | `Interface + interfaceGet/interfacePost/interfacePut/interfaceDelete` |
| `rest/client.go:62` | `ClientContentConfig` | `ClientContentConfig` |
| `rest/client.go:87` | `RESTClient` | `RESTClient` |
| `rest/client.go:113` | `NewRESTClient` | `newRESTClient` |
| `rest/client.go:174` | `*RESTClient.GetRateLimiter` | `getRateLimiter` |
| `rest/client.go:213` | `*RESTClient.Verb` | `verb (Request) / clientVerb (RESTClient)` |
| `rest/client.go:218` | `*RESTClient.Post` | `post` |
| `rest/client.go:223` | `*RESTClient.Put` | `put` |
| `rest/client.go:228` | `*RESTClient.Patch` | `patch` |
| `rest/client.go:233` | `*RESTClient.Get` | `get` |
| `rest/client.go:238` | `*RESTClient.Delete` | `delete` |
| `rest/client.go:243` | `*RESTClient.APIVersion` | `apiVersion` |
| `rest/client.go:265` | `*requestClientContentConfigProvider.GetClientContentConfig` | `getClientContentConfig` |
| `rest/client.go:297` | `*requestClientContentConfigProvider.UnsupportedMediaType` | `unsupportedMediaType` |
| `rest/client.go:322` | `clientNegotiatorWithCBORSequenceStreamDecoder.Encoder` | `Negotiator.encode` |
| `rest/client.go:326` | `clientNegotiatorWithCBORSequenceStreamDecoder.Decoder` | `Negotiator.decoder` |
| `rest/client.go:330` | `clientNegotiatorWithCBORSequenceStreamDecoder.StreamDecoder` | `Negotiator.streamDecoder / getClientContentConfig CBOR-sequence policy` |
| `rest/config.go:48` | `DefaultQPS` | `defaultQPS` |
| `rest/config.go:49` | `DefaultBurst` | `defaultBurst` |
| `rest/config.go:52` | `ErrNotInCluster` | `errNotInCluster (canonical sentinel error)` |
| `rest/config.go:56` | `Config` | `Config` |
| `rest/config.go:175` | `sanitizedAuthConfigPersister.GoString` | `configString (redacted capability/extension representations)` |
| `rest/config.go:178` | `sanitizedAuthConfigPersister.String` | `configString (redacted capability/extension representations)` |
| `rest/config.go:184` | `sanitizedObject.GoString` | `configString (redacted capability/extension representations)` |
| `rest/config.go:187` | `sanitizedObject.String` | `configString (redacted capability/extension representations)` |
| `rest/config.go:193` | `*Config.GoString` | `configString (redacted capability/extension representations)` |
| `rest/config.go:199` | `*Config.String` | `configString (redacted capability/extension representations)` |
| `rest/config.go:221` | `ImpersonationConfig` | `ImpersonationConfig (transport alias)` |
| `rest/config.go:235` | `TLSClientConfig` | `TLSClientConfig` |
| `rest/config.go:274` | `TLSClientConfig.GoString` | `tlsClientConfigString` |
| `rest/config.go:280` | `TLSClientConfig.String` | `tlsClientConfigString` |
| `rest/config.go:302` | `ContentConfig` | `ContentConfig` |
| `rest/config.go:329` | `RESTClientFor` | `restClientFor` |
| `rest/config.go:358` | `RESTClientForConfigAndClient` | `restClientForConfigAndClient` |
| `rest/config.go:423` | `UnversionedRESTClientFor` | `unversionedRESTClientFor` |
| `rest/config.go:445` | `UnversionedRESTClientForConfigAndClient` | `unversionedRESTClientForConfigAndClient` |
| `rest/config.go:488` | `SetKubernetesDefaults` | `setKubernetesDefaults` |
| `rest/config.go:533` | `DefaultKubernetesUserAgent` | `defaultKubernetesUserAgent` |
| `rest/config.go:546` | `InClusterConfig` | `inClusterConfig / inClusterConfigWith` |
| `rest/config.go:586` | `IsConfigTransportTLS` | `isConfigTransportTLS` |
| `rest/config.go:597` | `LoadTLSFiles` | `loadTLSFiles` |
| `rest/config.go:629` | `AddUserAgent` | `addUserAgent` |
| `rest/config.go:636` | `AnonymousClientConfig` | `anonymousClientConfig` |
| `rest/config.go:663` | `CopyConfig` | `copyConfig` |
| `rest/config.go:717` | `CodecFactoryForGeneratedClient` | `codecFactoryForGeneratedClient` |
| `rest/exec.go:31` | `ConfigToExecCluster` | `configToExecCluster` |
| `rest/exec.go:66` | `ExecClusterToConfig` | `execClusterToConfig` |
| `rest/fake/fake.go:34` | `CreateHTTPClient` | `createHTTPClient` |
| `rest/fake/fake.go:42` | `roundTripperFunc.RoundTrip` | `RoundTripper.roundTrip` |
| `rest/fake/fake.go:49` | `RESTClient` | `RESTClient` |
| `rest/fake/fake.go:67` | `*RESTClient.Get` | `get` |
| `rest/fake/fake.go:71` | `*RESTClient.Put` | `put` |
| `rest/fake/fake.go:75` | `*RESTClient.Patch` | `patch` |
| `rest/fake/fake.go:79` | `*RESTClient.Post` | `post` |
| `rest/fake/fake.go:83` | `*RESTClient.Delete` | `delete` |
| `rest/fake/fake.go:87` | `*RESTClient.Verb` | `verb (Request) / clientVerb (RESTClient)` |
| `rest/fake/fake.go:91` | `*RESTClient.APIVersion` | `apiVersion` |
| `rest/fake/fake.go:95` | `*RESTClient.GetRateLimiter` | `getRateLimiter` |
| `rest/fake/fake.go:99` | `*RESTClient.Request` | `request` |
| `rest/plugin.go:27` | `AuthProvider` | `AuthProvider` |
| `rest/plugin.go:41` | `Factory` | `Factory` |
| `rest/plugin.go:45` | `AuthProviderConfigPersister` | `AuthProviderConfigPersister` |
| `rest/plugin.go:51` | `*noopPersister.Persist` | `AuthProviderConfigPersister.persist` |
| `rest/plugin.go:60` | `RegisterAuthProviderPlugin` | `registerAuthProviderPlugin` |
| `rest/plugin.go:74` | `GetAuthProvider` | `getAuthProvider` |
| `rest/request.go:65` | `HTTPClient` | `HTTPClient` |
| `rest/request.go:71` | `ResponseWrapper` | `ResponseWrapper / responseWrapper` |
| `rest/request.go:77` | `RequestConstructionError` | `RequestConstructionError` |
| `rest/request.go:82` | `*RequestConstructionError.Error` | `requestConstructionErrorMessage` |
| `rest/request.go:97` | `Request` | `Request / fake.request` |
| `rest/request.go:135` | `NewRequest` | `newRequest` |
| `rest/request.go:180` | `NewRequestWithClient` | `newRequestWithClient` |
| `rest/request.go:189` | `*Request.UseProtobufAsDefaultIfPreferred` | `useProtobufAsDefaultIfPreferred` |
| `rest/request.go:196` | `*Request.UseProtobufAsDefault` | `useProtobufAsDefault` |
| `rest/request.go:215` | `*Request.Verb` | `verb (Request) / clientVerb (RESTClient)` |
| `rest/request.go:223` | `*Request.Prefix` | `prefix` |
| `rest/request.go:233` | `*Request.Suffix` | `suffix` |
| `rest/request.go:242` | `*Request.Resource` | `resource` |
| `rest/request.go:262` | `*Request.BackOff` | `backOff` |
| `rest/request.go:274` | `*Request.BackOffWithContext` | `backOff (implicit IO context)` |
| `rest/request.go:288` | `*Request.WarningHandler` | `WarningHandler / warningHandler` |
| `rest/request.go:299` | `*Request.WarningHandlerWithContext` | `WarningHandlerWithContext / warningHandler (implicit IO context)` |
| `rest/request.go:305` | `*Request.Throttle` | `throttle` |
| `rest/request.go:312` | `*Request.SubResource` | `subResource` |
| `rest/request.go:332` | `*Request.Name` | `name` |
| `rest/request.go:353` | `*Request.Namespace` | `namespace` |
| `rest/request.go:371` | `*Request.NamespaceIfScoped` | `namespaceIfScoped` |
| `rest/request.go:381` | `*Request.AbsPath` | `absPath` |
| `rest/request.go:395` | `*Request.RequestURI` | `requestURI` |
| `rest/request.go:420` | `*Request.Param` | `param` |
| `rest/request.go:432` | `*Request.VersionedParams` | `versionedParams` |
| `rest/request.go:436` | `*Request.SpecificallyVersionedParams` | `specificallyVersionedParams` |
| `rest/request.go:462` | `*Request.SetHeader` | `setHeader` |
| `rest/request.go:475` | `*Request.Timeout` | `timeout` |
| `rest/request.go:487` | `*Request.MaxRetries` | `maxRetries` |
| `rest/request.go:502` | `*Request.Body` | `bodyBytes/bodyReader/bodyObject/bodyFile (typed overload family)` |
| `rest/request.go:546` | `*Request.Error` | `requestError` |
| `rest/request.go:552` | `*Request.URL` | `requestURL` |
| `rest/request.go:760` | `*Request.Watch` | `watch (support/watch.Watch)` |
| `rest/request.go:894` | `*Request.Stream` | `stream` |
| `rest/request.go:1131` | `*Request.Do` | `doRequest` |
| `rest/request.go:1151` | `*Request.DoRaw` | `doRaw` |
| `rest/request.go:1390` | `Result` | `ResponseResult` |
| `rest/request.go:1402` | `Result.Raw` | `resultRaw` |
| `rest/request.go:1409` | `Result.Get` | `resultGet` |
| `rest/request.go:1435` | `Result.StatusCode` | `resultStatusCode` |
| `rest/request.go:1443` | `Result.ContentType` | `resultContentType` |
| `rest/request.go:1451` | `Result.Into` | `resultInto` |
| `rest/request.go:1482` | `Result.WasCreated` | `resultWasCreated` |
| `rest/request.go:1491` | `Result.Error` | `resultError` |
| `rest/request.go:1516` | `Result.Warnings` | `resultWarnings` |
| `rest/request.go:1521` | `NameMayNotBe` | `nameMayNotBe` |
| `rest/request.go:1524` | `NameMayNotContain` | `nameMayNotContain` |
| `rest/request.go:1527` | `IsValidPathSegmentName` | `isValidPathSegmentName` |
| `rest/request.go:1546` | `IsValidPathSegmentPrefix` | `isValidPathSegmentPrefix` |
| `rest/request.go:1558` | `ValidatePathSegmentName` | `validatePathSegmentName` |
| `rest/transport.go:33` | `HTTPClientFor` | `httpClientFor` |
| `rest/transport.go:53` | `TLSConfigFor` | `tlsConfigFor` |
| `rest/transport.go:64` | `TransportFor` | `transportFor` |
| `rest/transport.go:76` | `HTTPWrappersForConfig` | `httpWrappersForConfig` |
| `rest/transport.go:85` | `*Config.TransportConfig` | `resolveTransportConfig (authentication) + transportConfig (projection)` |
| `rest/transport.go:155` | `*Config.Wrap` | `wrap` |
| `rest/url_utils.go:30` | `DefaultServerURL` | `defaultServerURL` |
| `rest/url_utils.go:66` | `DefaultVersionedAPIPath` | `defaultVersionedAPIPath` |
| `rest/url_utils.go:82` | `DefaultServerUrlFor` | `defaultServerUrlFor` |
| `rest/urlbackoff.go:40` | `BackoffManager` | `BackoffManager` |
| `rest/urlbackoff.go:46` | `BackoffManagerWithContext` | `BackoffManagerWithContext` |
| `rest/urlbackoff.go:57` | `URLBackoff` | `URLBackoff / newURLBackoffWithBackoff(flowcontrol.Backoff[String])` |
| `rest/urlbackoff.go:63` | `NoBackoff` | `noBackoff` |
| `rest/urlbackoff.go:66` | `*NoBackoff.UpdateBackoff` | `Backoff.updateBackoff` |
| `rest/urlbackoff.go:70` | `*NoBackoff.UpdateBackoffWithContext` | `Backoff.updateBackoff (implicit IO context)` |
| `rest/urlbackoff.go:74` | `*NoBackoff.CalculateBackoff` | `Backoff.calculateBackoff` |
| `rest/urlbackoff.go:78` | `*NoBackoff.CalculateBackoffWithContext` | `Backoff.calculateBackoff (implicit IO context)` |
| `rest/urlbackoff.go:82` | `*NoBackoff.Sleep` | `Backoff.sleep` |
| `rest/urlbackoff.go:86` | `*NoBackoff.SleepWithContext` | `Backoff.sleep (implicit IO context)` |
| `rest/urlbackoff.go:100` | `*URLBackoff.Disable` | `disable` |
| `rest/urlbackoff.go:118` | `*URLBackoff.UpdateBackoff` | `Backoff.updateBackoff` |
| `rest/urlbackoff.go:123` | `*URLBackoff.UpdateBackoffWithContext` | `Backoff.updateBackoff (implicit IO context)` |
| `rest/urlbackoff.go:138` | `*URLBackoff.CalculateBackoff` | `Backoff.calculateBackoff` |
| `rest/urlbackoff.go:144` | `*URLBackoff.CalculateBackoffWithContext` | `Backoff.calculateBackoff (implicit IO context)` |
| `rest/urlbackoff.go:148` | `*URLBackoff.Sleep` | `Backoff.sleep` |
| `rest/urlbackoff.go:152` | `*URLBackoff.SleepWithContext` | `Backoff.sleep (implicit IO context)` |
| `rest/urlbackoff.go:172` | `*backoffManagerNopContext.UpdateBackoffWithContext` | `Backoff.updateBackoff (implicit IO context)` |
| `rest/urlbackoff.go:176` | `*backoffManagerNopContext.CalculateBackoffWithContext` | `Backoff.calculateBackoff (implicit IO context)` |
| `rest/urlbackoff.go:180` | `*backoffManagerNopContext.SleepWithContext` | `Backoff.sleep (implicit IO context)` |
| `rest/warnings.go:32` | `WarningHandler` | `WarningHandler / warningHandler` |
| `rest/warnings.go:39` | `WarningHandlerWithContext` | `WarningHandlerWithContext / warningHandler (implicit IO context)` |
| `rest/warnings.go:56` | `SetDefaultWarningHandler` | `setDefaultWarningHandler` |
| `rest/warnings.go:65` | `SetDefaultWarningHandlerWithContext` | `setDefaultWarningHandler (implicit IO context)` |
| `rest/warnings.go:82` | `warningLoggerNopContext.HandleWarningHeaderWithContext` | `WarningHandler.handleWarningHeader (implicit IO context)` |
| `rest/warnings.go:87` | `NoWarnings` | `noWarnings` |
| `rest/warnings.go:89` | `NoWarnings.HandleWarningHeader` | `WarningHandler.handleWarningHeader / warningWriterHandler` |
| `rest/warnings.go:90` | `NoWarnings.HandleWarningHeaderWithContext` | `WarningHandler.handleWarningHeader (implicit IO context)` |
| `rest/warnings.go:97` | `WarningLogger` | `warningLogger` |
| `rest/warnings.go:99` | `WarningLogger.HandleWarningHeader` | `WarningHandler.handleWarningHeader / warningWriterHandler` |
| `rest/warnings.go:103` | `WarningLogger.HandleWarningHeaderWithContext` | `WarningHandler.handleWarningHeader (implicit IO context)` |
| `rest/warnings.go:125` | `WarningWriterOptions` | `WarningWriterOptions` |
| `rest/warnings.go:134` | `NewWarningWriter` | `newWarningWriter` |
| `rest/warnings.go:148` | `*warningWriter.HandleWarningHeader` | `WarningHandler.handleWarningHeader / warningWriterHandler` |
| `rest/warnings.go:171` | `*warningWriter.WarningCount` | `warningCount` |
| `rest/watch/decoder.go:32` | `Decoder` | `Decoder` |
| `rest/watch/decoder.go:38` | `NewDecoder` | `newDecoder` |
| `rest/watch/decoder.go:47` | `*Decoder.Decode` | `decode` |
| `rest/watch/decoder.go:70` | `*Decoder.Close` | `close` |
| `rest/watch/encoder.go:32` | `Encoder` | `Encoder` |
| `rest/watch/encoder.go:37` | `NewEncoder` | `newEncoder` |
| `rest/watch/encoder.go:46` | `*Encoder.Encode` | `encode` |
| `rest/with_retry.go:40` | `IsRetryableErrorFunc` | `IsRetryableErrorFunc` |
| `rest/with_retry.go:42` | `IsRetryableErrorFunc.IsErrorRetryable` | `IsRetryableErrorFunc callback invocation` |
| `rest/with_retry.go:53` | `WithRetry` | `WithRetry` |
| `rest/with_retry.go:103` | `RetryAfter` | `RetryAfter` |
| `rest/with_retry.go:148` | `*withRetry.IsNextRetry` | `WithRetry.isNextRetry` |
| `rest/with_retry.go:195` | `*withRetry.Before` | `WithRetry.before` |
| `rest/with_retry.go:238` | `*withRetry.After` | `WithRetry.after` |
| `rest/with_retry.go:268` | `*withRetry.WrapPreviousError` | `WithRetry.wrapPreviousError` |
| `rest/with_retry.go:302` | `*wrapPreviousError.Unwrap` | `generic suffixError.Unwrap preserves final error chain` |
| `rest/with_retry.go:303` | `*wrapPreviousError.Error` | `canonical Error.errorMessage (previous cause suffix)` |
| `rest/zz_generated.deepcopy.go:25` | `*TLSClientConfig.DeepCopyInto` | `deepCopyIntoTLSClientConfig` |
| `rest/zz_generated.deepcopy.go:51` | `*TLSClientConfig.DeepCopy` | `deepCopyTLSClientConfig` |
| `util/apply/apply.go:34` | `NewRequest` | `newRequest` |

## Retained regressions and verification boundary

`request_test.gfx` retains stored-error, path/namespace, ordered query, request-URI replacement, 429 retry-ceiling, opaque-reader nonreplayability, malformed JSON, partial body read errors, creation status, warning deduplication, cancellation with an explicit start handshake, in-cluster sentinel identity, anonymous credential removal, CBOR Accept scrubbing, Config equality, and partial Outcome regressions. `util/apply/apply_test.gfx` covers all four feature combinations. The upstream source tests and `oracle/expected.json` remain the parity authority, including request bytes and ordered query expectations.

No build, formatter, linter, parser check, behavioral test, or suite was run by this implementation worker. Main must exercise the owned modules and integration paths before acceptance. A symbol mapping and retained test are not evidence that the code passed.

## Canonical metrics

`newEnvironment` is the stable six-argument constructor. `withMetrics(environment, registry, override)` attaches the canonical `tools/metrics.Registry` and optional Provider; `clientWithMetrics` attaches it to a directly constructed RESTClient. Both remain explicit. Every observation resolves the registry again, so attaching before provider registration does not freeze a no-op provider. Registry registration/deferred-once policy remains exclusively in tools/metrics.

The pinned observation sites are implemented as follows:

| Source site | Native observation |
|---|---|
| request.go:672-693 | Rate-limiter elapsed nanoseconds, verb, sanitized URL template; absent limiter and context cancellation/deadline emit no limiter observation |
| request.go:1013-1024 | Actual net/http DNSStart/DNSDone callbacks; native clock, synchronized start/host state, resolver host label |
| request.go:1034-1036 | Whole Do/DoRaw request elapsed time, including preflight errors and throttle; cleanup observation survives cancellation |
| request.go:1091-1094 | Actual request ContentLength after each Do/DoRaw attempt; omit unknown length and non-nil zero-length body |
| request.go:1144-1146,1168-1170 | Response bytes once, if success or nonempty partial response; omit empty failed response |
| with_retry.go:238-265; request.go:860-887 | Result counter on every attempt; retry counter only attempts after the first; response status wins, otherwise bounded `<error>` code; verb and actual request host |

Watch and Stream retain result/retry/DNS observations, and Stream retains limiter observations. Neither emits ordinary request latency/request size/response size. URL templates redact namespace/name and query parameter values with the pinned core/named-group path rules; raw object paths are never substituted as metric labels.

`metrics_test.gfx` registers a recording canonical provider after attachment and drives the real HTTP client against a scripted loopback server through a 429 retry and success. It asserts counters, byte sizes, DNS, limiter events, registration-once and redacted URL labels. Additional real request-path cases cover preflight failure, streaming exclusions, and bounded transport-error labels. These tests are written, not executed; no metric parity acceptance is claimed without Main's focused run.
