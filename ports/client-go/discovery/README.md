# Native API discovery

Source: Kubernetes client-go **v0.37.2**, revision
`72c52dd0cc3b9099b36af11290b0304045dd1ada`, discovery and restmapper families.
Copyright The Kubernetes Authors; Apache-2.0.

## API

`DiscoveryInterface` is an explicit callback record. Its methods return `IO`
and shared `support/outcome.Outcome`; there is one contextual API rather than
parallel legacy/context wrapper hierarchies. `GroupsAndResources` retains
optional group/resource collections and a simultaneous error. All metadata,
versions, GVRs/GVKs and OpenAPI documents retain the canonical dependency types.
Group-version dictionary keys use the reversible canonical spelling (`v1`,
`apps/v1`), avoiding a second nominal identity or string-keyed resource table.

Construct with `newDiscoveryClient(rest.Interface)`, or use
`newDiscoveryClientForConfig(environment,config)` /
`newDiscoveryClientForConfigAndClient(environment,config,httpClient)`.
The supplied canonical HTTP client is never replaced or cloned. Discovery
sets the upstream 32-second default timeout, burst 300, empty API path/group
version and metadata decoder. Configured nonzero timeout/burst and user agent
survive. `newDiscoveryClientWithOptions` exposes the legacy prefix,
legacy-only format and no-peer negotiation switches.

`source-coverage.json` enumerates all **234 exported production declarations**
in the owned upstream sources, including methods on private implementations.
It records native equivalents, not verification results. Context conversions
are intentionally eliminated; the deprecated cached-package alias maps directly
to `cached/memory` rather than adding a shim.

## Discovery policy

* `/api` and `/apis` negotiate aggregated v2 ahead of JSON legacy; the no-peer
  profile precedes both. MIME parameters may arrive reordered.
* Legacy `/api` 404 is tolerated. `/apis` errors are not. Individual core `v1`
  404 returns an empty resource list. Empty group-version arguments fail with
  `groupVersion shouldn't be empty`.
* Aggregated results are used only when **both** roots return resources.
  Mixed responses fall back to version endpoints, fetched concurrently through
  parent-linked native tasks. Legacy output follows group/version discovery order.
* Stale aggregated versions are omitted from the group version list and retained
  in the failure map. The first non-stale version is preferred. Missing parent
  GVKs do not suppress a subresource with a valid response GVK; subresources
  inherit only scope/singular name, not parent categories/short names.
* Group failures retain all successful partial results and original cause
  identities. Error text is sorted. `isGroupDiscoveryFailedError` is a direct
  type predicate; `groupDiscoveryFailedErrorGroups` traverses wrapping.
  Errors compare by native structural tag, without treating contained failure
  causes as an unwrap chain. Discovery methods make the upstream two attempts
  on direct group failures only.
* Preferred resource selection gives a group's preferred version precedence,
  falls back per resource to another advertised version, ignores subresources,
  and preserves empty per-version output lists. Namespaced filtering removes
  empty lists. Helpers include resource enablement, exact server build matching,
  supported-version negotiation, resource sets and arbitrary verb predicates.

## Cache policy

`cached/memory.newMemCacheClient` returns `CachedDiscoveryInterface`; initial
freshness is false. A single mutex owns refresh, entries and OpenAPI cache state.
All advertised versions are fetched once. Permanent failures/misses are cached;
context/network reset/refusal, 5xx and 429 failures retry the requested entry.
Empty resource responses are cached errors. Aggregated partial failures remain
visible without discarding successes. Invalidation clears all state and the
cached OpenAPI client. `newMemCacheClientWithInvalidation` composes invalidation
when wrapping an already-cached native capability.

`cached/disk.newCachedDiscoveryClient` takes the delegate cached capability,
cache directory, TTL in nanoseconds and clock. The config constructor adds the
memory cache and optional HTTP disk transport. Group/resource files retain the
upstream `servergroups.json` / `<groupVersion>/serverresources.json` names.
Metadata is encoded with the v1 kind; directories are 0750 and files 0660.
Writes use same-directory temporary files, close, atomic rename and unconditional
temporary-file cleanup. No fsync is added. Read/codec/write failures fall back
to live discovery; errors/empty results are never cached. Expiry is strictly
`now > mtime + ttl`. Reading another process's file changes freshness to false.
After invalidation only files written by this client may be read; invalidation
sets freshness true and propagates to the delegate without deleting disk files.

The native `third_party/forked/httpcache` owns HTTP/ETag policy; disk merely
supplies its `Cache` callback record and a native transport layer. Keys are
SHA-256 hex. Values are a 32-byte SHA-256 followed by response bytes; truncated
or corrupt values are misses. Cancel, close and unwrap follow the original
canonical transport identity. Go files contain only codec/conversion,
structural-error transport, cryptography and filesystem effects.

## Fakes

`fake.newFakeDiscovery(testing.Fake)` consumes the testing aggregate's resource
list and records the upstream `get group`, `get resource`, and `get version`
actions. Reactor errors and partial resource results are retained. Group
versions are built from configured resources; version defaults come from
`pkg/version`. Use `newFakeDiscoveryState`, `setServerVersion`, `asDiscovery`
for the mutable version override. As in upstream, preferred-resource queries
are empty and the v2 schema is an empty document. Upstream's explicitly
unsupported fake OpenAPI-v3 query remains an IO failure, not a fake schema.
The fake's legacy capability is the same configured fake.

## Regression source and verification

`regression/*_test.gt` contains consumer-behavior cases for partial groups,
error wrapping/identity, stale aggregate versions, parent/subresource GVKs,
resource preference, permanent misses, retryable failures, invalidation,
deferred mapper reset, missing/ambiguous mappings, shortcuts/categories,
mixed aggregated/legacy wire negotiation, MIME parsing, disk freshness/TTL,
checksum corruption and deletion. Filesystem fixtures clean up with bracket.
These are permanent native tests, not source-text checks.

No parser check, typecheck, build, formatter, linter or test was run by this
slice. Main should run the focused native regression module and its integrated
production dependency check after all sibling owners land. Written tests and
source coverage do not constitute observed parity or acceptance evidence.

Collection mapping uses the explicitly imported `std/prelude.map` operation;
`std/list` supplies list-specific folds, filters and effectful traversals.
Host adapters that transport opaque values or `rt.IO` use the fully erased
all-`any` boundary required by SPEC §8.3; scalar-only helpers retain natural Go
signatures. This changes only boundary boxing, not discovery policy.
