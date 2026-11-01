# Native dynamic client

Port of client-go **v0.37.2**, revision `72c52dd0cc3b9099b36af11290b0304045dd1ada`. Kubernetes-derived source and preserved golden fixtures are Apache-2.0. `coverage.json` maps every exported production declaration, every owned upstream test file, and all 20 golden files to the native surface/source provenance.

## API

- `newForConfig(environment, config)`, `newForConfigAndClient(environment, config, httpClient)` return `IO[Outcome[Option[Interface]]]`; `newForConfigOrDie` raises the original error. A supplied HTTP client retains its identity and configuration.
- `configFor(config, featureRegistry)` copies config, applies JSON/CBOR feature-gated preferences, installs the native negotiated serializer, and fills only an empty user agent.
- `newClient(restInterface, parameterCodec, featureRegistry, objectMarshal)` is the injection constructor. `fromRESTClient` supplies canonical codecs. `Interface.resource(gvr)` returns a `ResourceInterface`; its `namespace(ns)` returns another immutable resource handle. `NamespaceableResourceInterface` is the same native record because it already contains namespace selection.
- Resource callbacks: `create(object, CreateOptions, subresources)`, `update(object, UpdateOptions, subresources)`, `updateStatus(object, UpdateOptions)`, `get(name, GetOptions, subresources)`, `list(ListOptions)`, `delete(name, DeleteOptions, subresources)`, `deleteCollection(DeleteOptions, ListOptions)`, `patch(name, patchType, bytes, PatchOptions, subresources)`, `apply(name, object, ApplyOptions, subresources)`, `applyStatus(name, object, ApplyOptions)`, and `watch(ListOptions)`.
- Object results are `IO[Outcome[Option[canonical Unstructured/UnstructuredList]]]`; delete operations return `Outcome[Unit]`. Watches use `support/watch.Watch`. Subresources are `List[String]`, bytes and media types use lossless String bytes, and all options use canonical apimachinery types.
- `APIPathResolverFunc` and `legacyAPIPathResolverFunc` provide the shared scale path-resolver contract.

## Preserved policies

Arbitrary caller-supplied GVRs select `/api/<version>` or `/apis/<group>/<version>`; there is no resource inventory lookup or CRD allowlist. Namespace/name validation occurs before request construction. An empty required name reports `name is required`; namespace validation wins over a nonempty invalid resource name. Create requires an object name only for subresources. Options are encoded specifically as metav1 v1. Apply rejects nonempty managed fields with the upstream message, including its original `UnstructructuredExtractor` spelling. Watch forces the watch query option.

`scheme.gt` owns unstructured creation/typing and permissive write typing. The external host only constructs apimachinery serializers and converts external values. Unknown resource GVKs are created as Unstructured; normal decode requires the upstream GVK constraints, while write encoding accepts missing kind/apiVersion. Stream serialization uses the basic metav1 scheme. There is no production import of original client-go.

## Fakes, caches and listers

`fake.newSimpleDynamicClient` and `newSimpleDynamicClientWithCustomListKinds` build native testing reactors/trackers. The fake exposes `fake`, `scheme`, `tracker`, and `listKinds`; `fake.interface(client)` projects the same resource capability record and marks watch-list semantics unsupported. Construction infers registered list kinds, preserves explicit overrides, converts typed initial objects, and registers absent unstructured types. List requires a registered mapping and filters labels only. Fake operations intentionally do **not** adopt real-client empty-name/path/managed-fields validation. Root list option normalization and error/nil distinctions follow upstream.

The private unstructured scheme is allocated through apimachinery `NewScheme`,
not a zero-value Scheme, so subsequent kind registration has initialized maps.

`dynamiclister` uses the canonical typed cache aggregate and offers native `newLister`, `list/get`, `namespace`, `namespaceList/namespaceGet`, and `newRuntimeObjectShim` projections. Projections preserve backing object identity.

`dynamicinformer.newSharedInformerFactory` / `newFilteredSharedInformerFactory` and `newFilteredInformer` create typed informers. Their first argument is the explicit application's `features.FeatureRegistry`, matching GeneratedInformers; WatchListClient is read through a deferred cache gate during run, not captured at allocation. The independent `unsupportedWatchList` capability blocks fake watch-list support. Dynamic sets the full GVR object description. Factories memoize by immutable captured GVR fields, start each requested informer once, propagate the calling IO context, and wait for actual tasks during uninterruptible shutdown. Requesting a new informer does not auto-start it; calling start after shutdown does nothing. `forResource`, `start`, `waitForCacheSync`, and `shutdown` are explicit functions. `GenericInformer` exposes `informer` and `lister`. The reusable generic factory is also used by metadata, rather than duplicating lifecycle policy.

The same registry also controls the independent AtomicFIFO store-RV projection.
Dynamic's custom shared configuration retains its description and other options
while installing the canonical feature-backed `atomicFIFOGate`; storage reads
observe subsequent registry replacement. Enabling WatchListClient alone does not
expose stored RVs when AtomicFIFO is disabled.

## Evidence

Native regression sources cover arbitrary CRD paths, scope immutability, status/multiple subresources, all mutation verbs, empty/invalid names, option queries, malformed list bodies, apply managed fields, raw watches, fake quirks, lister isolation, and factory identity/shutdown admission. The original golden bytes remain under `testdata/`.

The pinned original-Go Status/Into probe established that an HTTP 200 response
with either `Status: Success` or `Status: Failure` populates the supplied
Unstructured and returns no error through the dynamic client. This differs from
an alternate decoder returning a typed metav1 Status. `status_test.gt` asserts
the exact returned fields for both Status cases and an ordinary Widget control;
it does not assume an empty destination. Probe source and observed output are
preserved in the session's `clientgo-rest-status-into-evidence.md` and
`clientgo-rest-status-into-probe.go` artifacts.

`dynamic/regression` contains 17 separately discoverable native golden tests.
They consume all 20 preserved files: 11 full server-perceived HTTP requests
(including exact bodies), six semantic JSON outputs, and three serialized HTTP
response inputs. The host fixture only reads the allowlisted originals, performs
standard HTTP serialization/parsing, and compares output; it never updates the
goldens or imports original client-go. Request construction, object encoding,
response decoding and watch consumption execute the native dynamic client.
The fixture uses public `dynamic.newForConfigAndClient` with an explicit feature
registry, controlled environment and injected HTTP client. It does not duplicate
the unversioned REST defaults; the same constructor path as upstream selects the
canonical `meta.k8s.io/v1` negotiation target. Complete request comparisons expose
regressions that would inject unwanted TypeMeta into permissive write bodies.
Run this package through the normal `go4t test` harness; fixture lookup is
relative to the host source rather than the process working directory.

`host/codecs_test.go` fixes the erased serializer constructor ABI and conversion
contracts in regression tests: value GroupVersion arguments are not wrapped in
Option, interface and pointer handles retain their original identity, and a typed
nil interface remains present. Optional object-conversion errors use the canonical
dependency error bridge. No native API signatures are changed.

**No parser, compiler, build, linter, formatter or test was run for this assignment.** Source and fixture mappings are not acceptance evidence; integrated verification belongs to the supervisor.
