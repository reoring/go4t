# Discovery REST mapping

Native implementation of client-go v0.37.2 `restmapper`, pinned revision
`72c52dd0cc3b9099b36af11290b0304045dd1ada`. Copyright The Kubernetes Authors,
Apache-2.0. The exhaustive public declaration mapping lives in
`../discovery/source-coverage.json`.

## Surface

`APIGroupResources{group:metav1.APIGroup,versionedResources:Map[String,List[APIResource]]}`
is the immutable discovery snapshot. `getAPIGroupResources` retains successful
partial groups/resources: if both collections exist, a group failure does not
prevent constructing mappings, matching upstream behavior.

`newDiscoveryRESTMapper` returns a native `RESTMapper` capability record:
`kindFor`, `kindsFor`, `resourceFor`, `resourcesFor`, `restMapping`,
`restMappings`, `resourceSingularizer`, and `reset`. Each query returns explicit
`IO[Outcome[...]]`; reset is `IO[Unit]`. All resource and kind identities are
canonical `runtime/schema` types. `RESTMapping` is a native immutable projection
with `resource`, `groupVersionKind`, and `scope` (`namespace` or `root`). Missing
and ambiguous errors are constructed as **actual apimachinery meta error types**,
not string approximations; `isNoMatchError` uses the canonical predicate.

## Policy

* Core v1 has first priority, followed by each group's preferred available
  version and its other advertised versions in discovery order. Resources
  absent from the preferred version remain discoverable in older versions.
* Subresource names containing `/` are excluded from REST mapping. Both the
  advertised kind and its lower-case spelling resolve. Explicit singular names
  win; absent singular names use the upstream unsafe kind inflection, including
  the endpoints exception. Guessed kind-list and generic List mappings are
  retained for upstream parity, not substituted from a built-in resource table.
* Resource lookup accepts partial group/version, exact resource/singular names,
  and group prefixes after exact-group lookup misses. Single-result queries
  narrow by version priority; unresolved ties produce typed ambiguity errors.
* `newDeferredDiscoveryRESTMapper` performs no discovery during construction.
  First query initializes once under the mutex. Query failure/empty multi-result
  with a non-fresh discovery cache invalidates and repeats; a fresh miss does not.
  Explicit reset invalidates the client and clears the mapper. The optional
  state constructor exposes `deferredString`, which lazily describes resolved
  mappings or the initialization error.
* Shortcut expansion reads current discovery on every call. Exact full or
  singular resource names win over short names. The first advertised shortcut
  wins; later distinct resources emit a single warning each, not per version.
  A successful exact shortcut clears the requested version as upstream does;
  group-prefix expansion retains it. `kindFor` re-expands after a typed no-match
  so newly discovered CRDs can resolve following deferred invalidation. Other
  resource operations expand once; kind mapping operations and reset delegate.
* Categories use partial discovery results even when discovery reports an error.
  Simple, discovery and union expanders retain a separate found flag and
  deduplicate group/resource identities without a fixed category fallback.

Legacy and `WithContext` overloads intentionally collapse into the same explicit
IO capability. No compatibility aliases or original client-go calls remain.
All mapping, priority, invalidation, shortcut and category policies are `.gfx`;
Go only converts external canonical error types.

Regression source is in `../discovery/regression`. No product checks were run
by this slice; main owns integrated verification.
