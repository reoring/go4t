# Native private HTTP cache

Port of client-go v0.37.2 `third_party/forked/httpcache/httpcache.go`, itself a
fork of github.com/gregjones/httpcache. See the unmodified LICENSE.

Public mapping:

- `Cache.Get/Set/Delete` -> `Cache.get/set/delete`, all explicit IO.
- `MemoryCache` and `NewMemoryCache` -> `newMemoryCache`, a locked native map.
- `CachedResponse` -> `cachedResponse`, preserving HTTP decode errors.
- `Transport`/`NewTransport` -> `Transport`/`newTransport`; the native
  constructor explicitly accepts the base transport and clock rather than
  using process-wide defaults. `newDefaultTransport` supplies real defaults.
- `NewMemoryCacheTransport` -> `newMemoryCacheTransport(base,clock)`.
- `Transport.RoundTrip` -> `roundTrip`; `asRoundTripper` preserves wrapped
  transport cancellation, idle-close, and lifetime behavior.
- `Transport.Client` -> `client`, producing a canonical net/http.Client.
- `XFromCache` -> `xFromCache()`; `Date` -> `date`; `ErrNoDateHeader` ->
  `errNoDateHeader()` with stable error identity.
- Internal `cacheKey`, `varyMatches`, `getFreshness`, `canStaleOnError`,
  `getEndToEndHeaders`, `canStore`, `parseCacheControl`, and
  `headerAllCommaSepValues` remain native functions. Request cloning uses
  shared Transport effects. HTTP parse/dump and EOF notification are generic
  host effects, never cache decisions.

The upstream private-cache rules are intentionally retained, not modernized:
GET/HEAD without Range are cacheable; the key includes method except GET;
request no-cache is transparent, response no-cache stale, only-if-cached
allows existing cache entries and otherwise returns 504. Request max-age
replaces response lifetime; response max-age precedes Expires; strict age
comparison, min-fresh, max-stale, and stale-if-error follow the fork. Vary
records live in X-Varied-* headers. ETag/Last-Modified conditional checks use
the upstream header names, including its unusual tests against Etag and
Last-Modified rather than the If-* names. GET 304 merges only end-to-end
headers and serves the stored body; eligible transport/5xx errors can serve
stale. No-store deletes entries. GET storage occurs only when body EOF is
observed, never on early Close; HEAD is dumped immediately. Existing stale
entries are replaced/deleted according to the same fork's status rules.

All policies are in `cache.gt`. `host` owns only HTTP codecs, original-error
conversion, headers, time parsing, EOF IO callbacks and test fixture effects.
`cache_test.gt` supplies focused behavior regressions. No tests, builds,
linters or formatters have been run for this implementation.
