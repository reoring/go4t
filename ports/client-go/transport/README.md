# Native transport

Copyright 2015, 2018, 2020 The Kubernetes Authors. Licensed under the Apache
License, Version 2.0: <https://www.apache.org/licenses/LICENSE-2.0>.

Authority: client-go v0.37.2, revision
`72c52dd0cc3b9099b36af11290b0304045dd1ada`. This directory owns the eight immediate
production Go source files and five immediate upstream test files. `spdy/` and
`websocket/` are owned by Streaming, not omitted from the overall port.

## API and ownership

`Config`, `TLSConfig`, `ImpersonationConfig`, `RoundTripper`, `TokenSource`, and
`ResettableTokenSource` are gofx-facing APIs. HTTP requests/responses, TLS configs,
certificates, connections, OAuth tokens, URLs, and errors use canonical `deps/`
identities. TLS bytes use `Option[String]`: strings carry exact bytes; None and
Some("") retain distinct source presence. No credentials have Debug/Show instances.

`roundTripper(run,cancel,close)` is the stable three-callback constructor.
The capability also carries `lifetime`, `httpIdentity`, and effectful
`wrappedRoundTripper`. Use `layer(base,run)` to extend a transport: it retains the
base lifetime, exposes the base for traversal, and clears direct HTTP identity.
Changing only the `roundTrip` field on a value with `httpIdentity=Some(...)` is not
a wrapper: `toHTTP` intentionally returns that original HTTP identity unchanged.

Create one `Environment` per intended shared cache, with the application-wide
`features.FeatureRegistry`. `newEnvironment` supplies real clock/default diagnostics;
record updates inject fake clocks, diagnostics, and refresh intervals. No implicit
process-wide feature/cache registry is created. Domain failures return the shared
`Outcome`, including partial external results; effect cancellation remains IO.

Holder identity is explicit and stable through `newGetCertHolder`/`newDialHolder`.
A present holder always contains a callable function; Go's invalid present-holder
with nil-function state is unrepresentable through these constructors. Config's
presence and precedence fields are otherwise preserved, including independent
reload flags, CA/cert/key filenames and data, and optional protocol order.

## Policy locations

- `config.gfx`: schema, predicates, wrapper composition and holder identity.
- `middleware.gfx`: custom -> debug -> auth -> user-agent -> impersonation
  construction order; request traversal occurs in reverse. Request headers are
  cloned before mutation. Explicit Authorization wins over every token source.
  Auth-proxy headers are overwritten on a clone; impersonation uses the user
  header as the all-fields override marker. Extra keys percent-encode bytes,
  including percent itself.
- `tls.gfx`: inline-data/file precedence, reload provenance, root policy,
  TLS 1.2 minimum, static/file/callback certificate precedence, and one-second
  certificate/error caching with the upstream strict `age > 1 second` boundary.
- `cache.gfx`: native ordered keys/maps, serialized cacheable construction,
  uncacheable proxy functions, weak/strong feature-controlled storage,
  replacement-generation eviction, and explicit lifetime attachment. Keys use
  source filenames for reloadable material and bytes otherwise. Credentials,
  user-agent and wrappers are not key inputs. NextProtos retains upstream
  comma-joined key semantics. DefaultTransport is not entered in the cache.
- `rotation.gfx`: native connection registry and certificate-change decisions,
  first-certificate exemption, atomic CA transport replacement and five-minute
  reload throttle. Failed/empty/unchanged CA reloads retain the previous transport;
  successful replacement closes only the old transport's idle connections.
- `rotation_controller.gfx`: immediate + periodic certificate work, deduplication,
  5ms..1000s exponential retry combined with a 10 QPS/100 burst limiter, using the
  existing native workqueue/flowcontrol implementations. Shutdown cancels and
  joins the poller and shuts down/joins the delaying queue. Cache-GC disabled mode
  intentionally retains upstream non-cancellation semantics.
- `token_source.gfx`: canonical OAuth-token cache, strict expiration/leeway and
  reset-age comparisons, stale-success fallback on refresh errors, token-file
  precedence, and post-401 reset without replay. Newly acquired tokens are not
  invalidated by the same request. A pre-dispatch OAuth failure closes the request
  body; refreshable static bearer failure instead uses its configured fallback.
  Expiry conversion uses Runtime's lossless TimeValue/InstantValue bridge, not
  UnixNano, so real monotonic timestamps and fake-clock timestamps both retain
  their comparison semantics. Legacy OAuth CancelRequest follows the pinned
  dependency's one-time deprecation notice/no-op; explicit Authorization still
  dispatches cancellation to the base. Context cancellation remains functional.
- `debug.gfx`: native context-level selection, redaction, header/curl formatting,
  and timing presentation. Generic trace callbacks only collect network timings.
- `host/`: generic HTTP/TLS/file/crypto/trace/weak-reference effects and canonical
  conversions. It does not import client-go, choose credentials, own transport
  caches, schedule refresh, or implement native middleware decisions.

Weak entries retain holder identities but never the returned lifetime token.
The lifetime's stored base does not point back to its token. Cleanup captures
only the worker, cache/key/generation and diagnostics; it cancels/joins actual
worker completion before generation-checked eviction. Callback projections also
retain the token through the end of each operation. Auth wrappers cannot turn a
weak entry into an accidental strong cache entry.

`Diagnostics.event` is the native metrics extension port: `cache:hit`,
`cache:miss`, `cache:miss-gc`, `cache:uncacheable`, `cache_entries` (decimal count),
`cache_gc:deleted`, `cache_gc:skipped`, `certificate_rotation_gc`, and
`ca_reload:{success|failure}:{unchanged|updated|read_error|empty|ca_parse_error}`.
It also receives certificate reload errors. `metrics.gfx` connects these events
to the canonical `tools/metrics.Provider`: call `withMetrics(env,registry,override)`
or `newEnvironmentWithMetrics(featureRegistry,registry,override)` after registering
the application metrics. Provider resolution invokes deferred registration;
explicit overrides retain the canonical registry's precedence. No second metrics
registry is introduced and no transport-provider names are switched upon.

## Exported production symbol mapping

All immediate production exported declarations are individually mapped below.
Methods on unexported Go receiver types are included because they are inventory
entries. Native record callback fields replace interface methods.

| Upstream file / symbol | Native capability |
| --- | --- |
| config.go / Config | Config, defaultConfig |
| config.go / DialHolder | DialHolder, newDialHolder |
| config.go / ImpersonationConfig | ImpersonationConfig, defaultImpersonationConfig |
| config.go / Config.HasCA | hasCA |
| config.go / Config.HasBasicAuth | hasBasicAuth |
| config.go / Config.HasTokenAuth | hasTokenAuth |
| config.go / Config.HasCertAuth | hasCertAuth |
| config.go / Config.HasCertCallback | hasCertCallback |
| config.go / Config.Wrap | wrap (returns replacement Config) |
| config.go / TLSConfig | TLSConfig, defaultTLSConfig |
| config.go / GetCertHolder | GetCertHolder, newGetCertHolder |
| transport.go / New | newTransport |
| transport.go / TLSConfigFor | tlsConfigFor |
| transport.go / WrapperFunc | WrapperFunc |
| transport.go / Wrappers | wrappers |
| transport.go / ContextCanceller | contextCanceller |
| transport.go / contextCanceller.RoundTrip | returned RoundTripper.roundTrip |
| cache.go / tlsCacheKey.String | cacheKeyString (key data redacted) |
| cache.go / trackedTransport.RoundTrip | attachLifetime(...).roundTrip |
| cache.go / trackedTransport.WrappedRoundTripper | attachLifetime(...).wrappedRoundTripper |
| ca_rotation.go / atomicTransportHolder.RoundTrip | newCARotatingTransport(...).roundTrip |
| ca_rotation.go / atomicTransportHolder.WrappedRoundTripper | newCARotatingTransport(...).wrappedRoundTripper |
| cert_rotation.go / CertCallbackRefreshDuration | Environment.certRefreshNanos (default 300000000000) |
| cert_rotation.go / dynamicClientCert.GetClientCertificate | loadClientCertificate |
| round_trippers.go / HTTPWrappersForConfig | httpWrappersForConfig |
| round_trippers.go / DebugWrappers | debugWrappers |
| round_trippers.go / NewAuthProxyRoundTripper | newAuthProxyRoundTripper |
| round_trippers.go / authProxyRoundTripper.RoundTrip | returned roundTrip |
| round_trippers.go / SetAuthProxyHeaders | setAuthProxyHeaders (explicit mutation) |
| round_trippers.go / authProxyRoundTripper.CancelRequest | returned cancelRequest |
| round_trippers.go / authProxyRoundTripper.WrappedRoundTripper | returned wrappedRoundTripper |
| round_trippers.go / NewUserAgentRoundTripper | newUserAgentRoundTripper |
| round_trippers.go / userAgentRoundTripper.RoundTrip | returned roundTrip |
| round_trippers.go / userAgentRoundTripper.CancelRequest | returned cancelRequest |
| round_trippers.go / userAgentRoundTripper.WrappedRoundTripper | returned wrappedRoundTripper |
| round_trippers.go / NewBasicAuthRoundTripper | newBasicAuthRoundTripper |
| round_trippers.go / basicAuthRoundTripper.RoundTrip | returned roundTrip |
| round_trippers.go / basicAuthRoundTripper.CancelRequest | returned cancelRequest |
| round_trippers.go / basicAuthRoundTripper.WrappedRoundTripper | returned wrappedRoundTripper |
| round_trippers.go / ImpersonateUserHeader | impersonateUserHeader() |
| round_trippers.go / ImpersonateUIDHeader | impersonateUIDHeader() |
| round_trippers.go / ImpersonateGroupHeader | impersonateGroupHeader() |
| round_trippers.go / ImpersonateUserExtraHeaderPrefix | impersonateUserExtraHeaderPrefix() |
| round_trippers.go / NewImpersonatingRoundTripper | newImpersonatingRoundTripper |
| round_trippers.go / impersonatingRoundTripper.RoundTrip | returned roundTrip |
| round_trippers.go / impersonatingRoundTripper.CancelRequest | returned cancelRequest |
| round_trippers.go / impersonatingRoundTripper.WrappedRoundTripper | returned wrappedRoundTripper |
| round_trippers.go / NewBearerAuthRoundTripper | newBearerAuthRoundTripper |
| round_trippers.go / NewBearerAuthWithRefreshRoundTripper | newBearerAuthWithRefreshRoundTripper |
| round_trippers.go / bearerAuthRoundTripper.RoundTrip | returned roundTrip |
| round_trippers.go / bearerAuthRoundTripper.CancelRequest | returned cancelRequest |
| round_trippers.go / bearerAuthRoundTripper.WrappedRoundTripper | returned wrappedRoundTripper |
| round_trippers.go / DebugLevel | DebugLevel |
| round_trippers.go / DebugJustURL | debugJustURL() |
| round_trippers.go / DebugURLTiming | debugURLTiming() |
| round_trippers.go / DebugCurlCommand | debugCurlCommand() |
| round_trippers.go / DebugRequestHeaders | debugRequestHeaders() |
| round_trippers.go / DebugResponseStatus | debugResponseStatus() |
| round_trippers.go / DebugResponseHeaders | debugResponseHeaders() |
| round_trippers.go / DebugDetailedTiming | debugDetailedTiming() |
| round_trippers.go / DebugByContext | debugByContext() |
| round_trippers.go / NewDebuggingRoundTripper | newDebuggingRoundTripper |
| round_trippers.go / debuggingRoundTripper.CancelRequest | returned cancelRequest |
| round_trippers.go / debuggingRoundTripper.RoundTrip | returned roundTrip |
| round_trippers.go / headersMap.String | headersString |
| round_trippers.go / headersMap.MarshalLog | maskedHeaders (typed entries rather than an untyped logger object) |
| round_trippers.go / debuggingRoundTripper.WrappedRoundTripper | returned wrappedRoundTripper |
| token_source.go / TokenSourceWrapTransport | tokenSourceWrapTransport |
| token_source.go / ResettableTokenSource | ResettableTokenSource |
| token_source.go / ResettableTokenSourceWrapTransport | resettableTokenSourceWrapTransport |
| token_source.go / NewCachedFileTokenSource | newCachedFileTokenSource |
| token_source.go / NewCachedTokenSource | newCachedTokenSource |
| token_source.go / tokenSourceTransport.RoundTrip | returned roundTrip |
| token_source.go / tokenSourceTransport.CancelRequest | returned cancelRequest |
| token_source.go / tokenSourceTransport.WrappedRoundTripper | returned wrappedRoundTripper |
| token_source.go / fileTokenSource.Token | fileTokenSource(...).token |
| token_source.go / cachingTokenSource.Token | returned ResettableTokenSource.token |
| token_source.go / cachingTokenSource.ResetTokenOlderThan | returned resetTokenOlderThan |

`cache_go118.go` exports no production declarations; its comparability premise is
replaced by native `CacheKey` Ord and explicit holder identities. All Config and
TLSConfig source fields are represented in the native schemas; there is no Go
source-drop-in API promise.

## Parity probes and verification status

`transport_test.gfx`, `lifecycle_test.gfx`, `http_test.gfx`,
`cancellation_test.gfx`, `cache_test.gfx`, and `metrics_test.gfx` contain executable native probes for
cloning/auth precedence, wrapper order, proxy/impersonation sanitation, cache keys,
cache reuse/uncacheable proxies, token-file and stale-token fallback, strict
refresh/reset boundaries, 401 no-replay, TLS validation/callback precedence,
contexts, CA reload throttling/failure/replacement, certificate error caching,
connection rotation, generation-safe eviction, cache diagnostic events, canonical
HTTP identity/unwrapping, OAuth body ownership and legacy cancellation, lossless
token-time conversion, and GC worker cancellation/join. Testsupport performs only
local temp-file, in-memory pipe/body and GC effects.

No parser, typechecker, generated build, formatter, linter or test command was run
in this assignment, as required by the supervisor-only acceptance plan. These are
source implementations and unexecuted parity probes, not an accepted parity
receipt. The primary should run the focused native transport suite, generated-Go
build, and its integrated cross-domain/streaming acceptance checks after all
owners finish. GC/FFI behavior in particular requires execution, not source-level
inference. No production imports or delegation to `k8s.io/client-go` are used.
