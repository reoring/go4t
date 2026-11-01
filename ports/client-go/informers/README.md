# Native shared informers

This domain ports the pinned Kubernetes client-go v0.37.2 informer factory and all 147 resource informers across 21 groups. `coverage.json` records all 3,325 exported upstream declarations with their native source location. The source tree, not the typed client descriptor alone, determines inclusion: typed client resources without an upstream informer are excluded. The generator rejects scope or GVR-routing disagreements with the pinned sources.

## Construction and access

`newSharedInformerFactoryWithOptions(featureRegistry, client, resyncNanos, options)` returns an effectful factory. The required first argument is the application's existing `features.FeatureRegistry`; standalone resource constructors also require it first. Options apply left-to-right. `withNamespace` defaults to all namespaces (`""`); cluster-scoped constructors never pass this namespace to clients. `withCustomResyncConfig` keys use fully qualified API object identities, such as `k8s.io/api/core/v1.Pod`. The first `informerFor` construction for a key wins and applies the global transform before publishing the aggregate.

Group/version access is native capability access: `core(factory).v1().pods()` yields a record with effectful `informer()`, `typedInformer()`, and `lister()` callbacks. Typed and raw access retain one underlying cache identity. `toTypedPodInformer` adapts the legacy capability record; `toPodIndexInformer` creates a typed cache view without another controller. Resource-specific handler constructors retain optional add/update/delete callbacks, detailed initial-list flags, tombstones and filtering transitions. Typed index functions are effectful and converted with the canonical object descriptor.

Every resource has independent and filtered constructors, typed equivalents, and WithOptions equivalents. Factory-created informers install the namespace index even for cluster-scoped resources, as upstream does. Standalone constructors retain exactly the explicitly supplied indexers. List/watch calls apply tweak callbacks on each request before invoking the typed client. List results retain partial values and original errors. Raw watches use `support/watch`; no foreign client-go execution is involved.

WatchListClient is read from that explicit registry during reflector construction at informer run time, matching upstream `tools/cache/reflector.go:361`. Registry replacements and overrides between allocation and start are observed; repeated start does not create a new reflector or reread the gate. The client's unsupported-WatchList capability remains a separate opt-out. No registry is created inside a factory and no feature decision is taken from a client Interface field.

Generated constructors use `cache.newSharedIndexInformerWithFeatures(featureRegistry, ...)`, capturing the same registry for AtomicFIFO storage projections as well as WatchListClient. AtomicFIFO is evaluated by the storage capability on each projection, independently of atomic event processing; no default registry is allocated. Custom `NewInformerFunc` callbacks own their allocated store: capture the application's registry and use that constructor, or `newSharedIndexInformerWithStoreOptions(..., threadSafeStoreOptionsWithFeatures(registry, metrics))` when supplying metrics. The factory installs WatchListClient for custom callbacks but cannot retroactively replace their storage options.

`forResource(factory, gvr)` performs the complete pinned GVR dispatch and returns a raw informer with a resource-aware generic lister. The GVR parameter does not shadow the `resource(factory)` group accessor used by resource.k8s.io routes. Unsupported resources return `no informer found for <group>/<version>, Resource=<resource>` rather than a dynamic fallback.

## Lifetime

`start` captures the caller's context. `startWithContext` explicitly supplies it. Both start only entries which have never been started, return immediately, and refuse new starts after shutdown begins. Creating an informer after start does not start it; call start again. A stopped informer is never restarted.

`waitForCacheSync` snapshots only started entries and waits on native cache completion channels, without polling. It returns the cache `SyncResult` with per-type keys. Cancellation is reported by the cache wait implementation. `shutdown` atomically refuses subsequent starts, joins all started tasks without cancelling their contexts, and releases the optional informer name only after joins complete. The caller must cancel the start context. Shutdown is uninterruptible and concurrent/repeated shutdown calls observe the same task set.

## Regeneration and regression fixtures

From the worktree root:

```sh
SERIALIZE_VM_TESTS_TARGET=local "${CODEX_HOME:-$HOME/.codex}/skills/serialize-vm-tests/scripts/serialize-vm-tests" --class short -- python3 tools/portgen/GeneratedInformers/generate.py --upstream /workspace/wip/gofx/.tmp/client-go-v0.37.2
```

For isolated regeneration, add `--output-root /absolute/path/to/empty-shadow`. Outputs are written only below that root's `ports/client-go/informers`; descriptor, inventory, canonical clientset and handwritten factory signatures still come from the live repository. Compare every generated shadow file against its corresponding live path before applying drift. The handwritten factory and fixtures are inputs/non-generated files, not copied into the shadow.

The generator consumes `tools/portgen/resources.json`, the pinned informer source and inventory; it never edits the shared descriptor. Its Go AST extractor resolves each upstream clientset `Interface` method's selector return type through the source imports, mapping the typed group/version import path to the exact getter. Existing lowerCamel naming converts that method to the native capability field (for example, `InternalV1alpha1` becomes `internalV1alpha1`, not a name inferred from the `apiserverinternal` directory). Missing or ambiguous mappings fail generation; every mapping is checked against the canonical native clientset field, import path and return type before outputs are written. Go execution requires the serialized runner shown above. Generated files have Apache-2.0 provenance. The handwritten `internalinterfaces/factory.gfx` and fixture files are preserved.

`internalinterfaces/factory_test.gfx` includes pure option ordering, effectful repeated tweak callbacks, and identity-preserving typed projection tests. Its `factoryLifecycleFixture(featureRegistry, client)` exercises real native cache tasks with controlled list gates: reuse, delayed initial synchronization, custom resync, default indexes, late creation, repeated start, shutdown join and restart refusal. Supply the application's registry and any constructed native clientset (for example the `.client` from `kubernetes/fake.newSimpleClientset(environment, Nil)`); the controlled lifecycle fixture does not invoke that client's HTTP callbacks. `watchListGateTimingFixture(client)` supplies an explicit test-owned registry and checks allocation/start timing, override observation, and no reread on repeated start. Run these fixtures under the integration harness's bounded context.

`resource_test.gfx` adds `generatedResourceDefaultsFixture(featureRegistry, client)`: actual generated Pod constructors share raw/typed/generic identity, install the namespace index, preserve the all-namespace default, and reject the client-only Binding resource. It does not start any network operations.

No product build, formatter, linter, or test was run by this owner. Integration acceptance belongs to the supervisor.
