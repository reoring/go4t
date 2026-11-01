# Native authentication plugins

Source: Kubernetes client-go **v0.37.2**, revision
`72c52dd0cc3b9099b36af11290b0304045dd1ada`. Copyright notices in the
implementations and pinned certificate fixture are retained under Apache-2.0.
This directory contains native authentication policy, not bindings to client-go.

## Composition

The application owns the lifetime of the explicit aggregates:

- `exec.newCache() IO[exec.Cache]` and `exec.newEnvironment() IO[exec.Environment]`.
- `oidc.newClientCache() IO[oidc.ClientCache]` and
  `oidc.newEnvironment(transport.Environment) IO[oidc.Environment]`.
- `rest.newAuthProviderRegistry() IO[rest.AuthProviderRegistry]`.

`auth.registerProviders(registry, oidcCache, oidcEnvironment)` registers oidc,
azure and gcp. `registerCommonProviders` registers oidc alone and
`registerCloudProviders` registers azure/gcp. Duplicate registration errors are
returned, not suppressed. Additional providers register through the same typed
Rest registry; no consumer edit is required.

`auth.authentication(registry, execCache, execEnvironment) rest.Authentication`
provides Rest's callback injection without an import cycle. The callback obtains
an authenticator and calls `exec.updateTransportConfig`. Kubeconfig records come
only from `tools/clientcmd/api`; credential records/codecs come only from
`pkg/apis/clientauthentication`; transport capabilities come only from
`transport`. Every fallible public domain operation returns the shared
`support/outcome.Outcome`. Cancellation/deadline failures remain IO failures.

## Exec contract

`getAuthenticator(Cache,Environment,api.ExecConfig,Option[auth.Cluster])`
returns `IO[Outcome[Option[Authenticator]]]`. The cache compares every config
and cluster field, including absent versus empty collections, ordered args/env,
plugin policy, stdin availability, install hint and external plugin config.
Cache keys retain deep-copied external object snapshots. Absent Object interfaces
decode to typed nil; present typed-nil objects stay present, and malformed Option
shapes remain errors. First insertion wins;
repeated configurations retain the same certificate and dial holder identities.
The authenticator separately retains the source configuration used for execution.

`updateTransportConfig(Authenticator,transport.Config)` returns
`IO[Outcome[transport.Config]]`. Explicit token/basic/certificate authentication
bypasses exec. Existing request Authorization bypasses exec. Otherwise native
wrappers set the returned token and install the canonical TLS certificate
callback. A conflicting certificate callback returns the pinned error. Native
callback types cannot represent Go's invalid nil dial-function state. Wrappers
use `transport.layer`, retaining transport lifetime and wrapper introspection
without incorrectly retaining a raw HTTP identity that would bypass auth.

`getCreds` serializes subprocess execution. Missing or zero-time expiration never
expires; other timestamps expire only when `now > expiration`, not at equality.
A credential generation identifies the cached instance. A 401 refreshes only
that generation and returns the original response without replay; another
request that already rotated the generation prevents duplicate refresh.
Refresh failure does not replace the cached credential. As upstream, a supplied
expiration is recorded before certificate parsing, even if parsing fails.
Both token and a complete certificate/key pair can be returned together.
Malformed output, unknown/mismatched versions, absent status, empty credentials,
and incomplete/invalid certificate pairs fail distinctly.

Interactive modes use source short-circuit ordering. `Never` does not inspect
stdin; `IfAvailable` inspects it only when available; `Always` first requires a
terminal, then availability. The native codec constructs `KUBERNETES_EXEC_INFO`
in the configured API version, including cluster information only when requested.
Parent environment, configured entries, and exec-info are appended in that
order. Command cleaning, argv, PATH resolution and allowlist matching retain
source ordering. The allowlist is checked before execution, and resolved
allowlist paths are retained. Install hints use `Sometimes`, with the source
10-per-hour default and an injectable clock.

Certificate parsing is an external crypto effect. Connection tracking,
custom/default dial wrapping, and close-all on certificate change use the
single Transport `ConnectionSet` implementation. Token-only changes do not
close connections. Native metrics callbacks report policy allowed/denied,
process exit/category, previous certificate age and earliest certificate expiry
across the cache; `None` means no metrics sink was installed.

The existing native `effects.dial(network,address)` owns exec's pinned 30-second
timeout and 30-second keepalive defaults. Its sole host extern receives both
durations explicitly in nanoseconds; the Go `net.Dialer` effect has no client-go
defaults. The Environment and authenticator dial callback signatures are unchanged.

## OIDC contract

`newOIDCAuthProvider(cache,environment,clusterAddress,config,persister)` returns
`IO[Outcome[Option[rest.AuthProvider]]]`. `newProvider` exposes the native aggregate
for callers needing `idToken(Provider)`. Cache identity is exactly cluster
address, issuer string and client ID; the first config/persister is retained.
Issuer URL and client ID are mandatory. CA-data base64 decoding and configured
CA-file/data are passed to canonical Transport. Deprecated extra-scopes remain
unused during refresh.

**This pinned plugin checks token freshness, not token authenticity.**
`idTokenExpired(nowNanos,token)` has the source's historical inverted name: true
means usable. It parses a three-part JWT payload and its `exp` (including numeric
exponent/fraction forms), applying a strict ten-second skew. It deliberately does
not verify the signature, audience, issuer or JWKS. Upstream `oidc_test.go:76,85`
explicitly supplies an unverified signature. Discovery consumes only
`token_endpoint`. TLS verification belongs to Transport, not JWT freshness.

A refresh holds the provider lock through discovery, exchange and persistence.
Discovery removes exactly one trailing slash, closes the response, checks HTTP
200 and limits the quoted error body to 80 bytes. OAuth2 exchange performs the
pinned Basic-first / body-credentials fallback, omits empty client credentials
and scopes from the body, accepts JSON/form token responses, requires an access
token, and separately requires string `id_token`. Token responses are read to
the pinned 1 MiB bound. A nonempty changed refresh token replaces the old token;
all unrelated configuration survives. Persistence must succeed before the
in-memory config changes. Network and persistence cancellation propagate and do
not commit tokens. `login()` intentionally returns **`not yet implemented`**:
this is the pinned upstream behavior, not an unfinished native implementation.

No authentication implementation logs token values, keys, environment or token
responses. Records containing credentials have no derived Show/Debug instances.
Process stdout is never included in exec-failure diagnostics. Provider-returned
HTTP error bodies remain part of upstream-compatible returned errors, not logs;
callers must not indiscriminately log such errors. The fixed RSA key in
`certificate_test.gfx` is the publicly distributed upstream test fixture.

## Exhaustive production exported-symbol map

The inventory has 13 exported production declarations under plugin (the larger
owner exported count also includes tests). Unexported receiver types do not make
their exported methods disappear from this map.

| Pinned source symbol | Native implementation |
|---|---|
| exec/exec.go:136 `(*sometimes).Do` | exec/limiter.gfx `Sometimes`, `newSometimes`, `doSometimes` |
| exec/exec.go:161 `GetAuthenticator` | exec/exec.gfx `getAuthenticator` with explicit cache/environment |
| exec/exec.go:263 `Authenticator` | exec/exec.gfx `Authenticator` / `Core` |
| exec/exec.go:313 `(*Authenticator).UpdateTransportConfig` | `updateTransportConfig` |
| exec/exec.go:356 `(*roundTripper).WrappedRoundTripper` | `wrapTransport` uses Transport layer introspection; `wrappedRoundTripper` returns supplied base |
| exec/exec.go:360 `(*roundTripper).RoundTrip` | `roundTrip` |
| exec/exec.go:661 `ValidatePluginPolicy` | `validatePluginPolicy` |
| oidc/oidc.go:178 `(*oidcAuthProvider).WrapTransport` | oidc/oidc.gfx `wrapTransport` |
| oidc/oidc.go:185 `(*oidcAuthProvider).Login` | `login` (same intentional error) |
| oidc/oidc.go:196 `(*roundTripper).RoundTrip` | `roundTrip` |
| oidc/oidc.go:219 `(*roundTripper).WrappedRoundTripper` | Transport layer introspection and `wrappedRoundTripper` |
| oidc/oidc.go:358 `(*jsonTime).UnmarshalJSON` | `unmarshalJSONTime`, native integer seconds plus generic JSON-number codec |
| oidc/oidc.go:378 `(jsonTime).MarshalJSON` | `marshalJSONTime` |

Other production source files are mapped completely by their policy purpose:
`plugins.go` / `plugins_providers.go` initialize the explicit registration entry
points; `azure_stub.go` / `gcp_stub.go` map to their respective factory/register
functions with the exact removal errors; `exec/metrics.go` maps to native
`Metrics`, `_callsMetric`, `_policyMetric`, `_rotationMetric`, `_expirationMetric`.
Exec cache, expiration, allowlist, command-error and credential lifecycle helpers
are native in exec.gfx. OIDC cache, JSON time, discovery and refresh/persistence
helpers are native in oidc.gfx.

## Staged regression evidence

No builds, checks, tests, live credential commands or live HTTP requests were run
by the implementation owner. The tests below were staged for the supervisor;
the mapping is not passing evidence.

- Exec `TestCacheKey` and `TestExecTLSCache`: presence-sensitive keys, cluster
  distinctions, shared certificate/dial identities and independent configs.
- `TestRefreshCreds`: cached/expired/equality transitions, native exec info and
  environment ordering, malformed output/version/status/pair failures, actual
  fixed `/bin/sh -c 'exit 7'` fixture, cancellation and secret-free failure text.
- `TestPluginPolicy`: nil versus empty, path normalization and deny-before-run.
- `TestRoundTripper`: original 401 returned, no replay, generation deduplication,
  request Authorization precedence and caller header immutability.
- `TestAuthorizationHeaderPresentCancelsExecAction`: token/basic/cert transport
  bypass and explicit request credentials.
- `TestTLSCredentials`: pinned certificate/key parsing, simultaneous token/cert,
  repeated certificate equality and certificate-to-token rotation. TLS handshake
  execution remains part of supervisor integration verification.
- `TestConcurrentUpdateTransportConfig`: immutable config composition and shared
  holders; credentials themselves are serialized by the aggregate mutex.
- `TestInstallHintRateLimit`: fake-clock threshold and exact window reset.
- Exec metrics tests: certificate expiry removal/rotation callbacks; the metric
  category/policy branches are exposed as native callbacks for integration sinks.
- OIDC `TestJSONTime`, `TestExpired`, `TestClientCache`: fractional/exponent time,
  strict skew boundary, malformed token and first-provider cache behavior.
- Additional OIDC tests cover simultaneous refresh, persistence failure/retry,
  missing refresh/discovery/id-token, OAuth credential-style fallback, omitted
  scopes, and cancellation without persistence or memory commit.
- Provider registry tests cover extension registration, duplicate rejection and
  exact azure/gcp removal messages.

All host functions are implemented under `plugin/host`: external process/path,
HTTP client/body lifetime, crypto, JSON/base64/form codecs and canonical object
conversion only. They neither import client-go nor implement provider cache,
registration, expiration, refresh or credential policy. No adapter is left as a
placeholder. Shared dependencies and the final integrated module still need the
supervisor's compilation and behavioral verification; no acceptance is claimed.

### Nil-interface integration repair

The supervisor's exec-suite execution reached 12 named passes, but failed the
cached-credential test and later aborted on `interface is nil, not runtime.Object`.
The generic host extractor asserted a nil interface returned by canonical
`hostdeps.To` for `Option[Object]=None`. It now returns the typed zero only for
that converted nil; canonical conversion still validates the input shape and
preserves the dynamic type of present typed-nil objects. Error boundaries use
`ErrorFromGo`, `ErrorOptionFromGo` and `ErrorToGo`, rather than general reflection.
`plugin/host/effects_test.go` adds focused absence, typed-nil, malformed-Option
and error-layer regressions. No successful rerun of this repair is claimed here.
