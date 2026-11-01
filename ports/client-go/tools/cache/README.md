# Native client-go cache

Source authority: client-go v0.37.2, revision `72c52dd0cc3b9099b36af11290b0304045dd1ada`, `tools/cache/**`. Implementation policy, queues, indexing, reflector decisions, batching, listeners, and test sources are gofx. The directory contains no Go implementation or call to original client-go.

## API and ownership

`ObjectAccess[A]` observes keys, resource versions, namespaces, labels, UIDs, deep copies and equality in **IO**. Native immutable callers lift their observations with `ioPure`; `rawObjectAccess()` supplies the default for canonical `runtime.Object`. `IndexFunc` and `TransformFunc` are also effectful. Comparators that inspect heap objects return `IO[Bool]`; map ordering only compares captured immutable keys.

`Store[A]` and `Indexer[A]` are native callback capabilities. `newStore`, `newIndexer`, and `newIndexerWithTransformer` allocate one indexed projection. `newStoreWithOptions`, `newIndexerWithOptions`, and `newThreadSafeStoreWithOptions` accept storage-owned metrics and an explicit AtomicFIFO gate. Store writes publish a complete immutable state under one mutex. Transactions retain successful original input indices. `Store.deleteByKey` and `DeleteKeyTransaction(key, optionalObject)` remove tombstones without manufacturing a typed object or a new resource version.

`ListWatcher[A]` carries a list callback returning `ListResult[A]{items,resourceVersion,continuation}` and a raw watch callback returning the shared `support/watch.Watch`. Both receive canonical `metav1.ListOptions`. `ListWatch` retains legacy/context callback preference. `restListWatchClient` connects this capability to the native REST interface using the canonical ParameterCodec. Unknown watch event strings, nil objects and API Status errors are not collapsed into typed resource events.

`newSharedIndexInformer(lw,access,decode,resync,indexers,clock)` creates the generic shared aggregate. `newSharedIndexInformerWithOptions` additionally selects watch-list, in-order/atomic/batched FIFO processing, metrics and mutation detection. `decode` is only a checked type conversion; external metadata observation remains IO. `getIndexer` and `getStore` return the existing projection.

`typedSharedInformer(raw,objects.ObjectType[A])` and `typedIndexer(rawIndexer,descriptor)` create **views**, not additional caches. They preserve the raw aggregate and handler-registration identity. Mismatched resource types produce errors rather than being discarded. Typed tombstones preserve absence.

All synchronization and clocks use `support/sync`. Queue close wakes readers but does not reject later FIFO writes. Heap close does reject writes. DeltaFIFO sync is a dequeue boundary; FIFO/RealFIFO sync is after processing. Informer sync waits for queue processing/publication, and a listener additionally waits for every initial Add callback to return. Late listeners replay while distribution is blocked, then start their sync observer. Each listener has an independent sequential runner and an unbounded pending pump. Removal stops future input without joining; shutdown waits for the executing callback. Closing input may discard pending notifications.

Reflector list/watch uses the last observed resource-version string, not a numerical maximum. Watch-list publishes only after an annotated `k8s.io/initial-events-end=true` bookmark and retains the watcher for continuation. Expired and too-large resource versions, 429, internal retry deadlines, resync failures, EOF, cancellation, malformed metadata and unsupported Table objects have separate paths.

### Legacy list lifetime

An upstream legacy List callback can ignore cancellation. Each native list attempt therefore owns a detached computation and a one-slot result mailbox. The attempt has no store/publication capability; a canceled observer returns without joining an uncooperative callback, and its late result cannot update the cache. This exception is intentional upstream parity, not an abandonment timeout. Resync, listener, mutation-detector and controller tasks use strict cleanup/join.

## Exported capability mapping

The names below account for the exported source surfaces, including exported methods on private implementation types. Native method equivalents are free functions or explicitly named callback fields; typed/untyped Go wrappers collapse into the same generic native implementation rather than compatibility shims.

| Upstream source and symbols | Native implementation/equivalent |
|---|---|
| `store.go`: Store; Add, Update, Delete, List, ListKeys, Get, GetByKey, Replace, Resync, Bookmark, LastStoreSyncResourceVersion | `store.gfx`: Store callback fields and `newStore`; indexed state shared with Indexer |
| TransactionType, TransactionTypeAdd/Update/Delete, Transaction, TransactionStore, TransactionError, Error, Transaction | `TransactionType`, `Transaction`, `Store.transaction`, `storeTransaction`, `transactionErrorMessage`; `KeyedOperation`/`DeleteKeyTransaction` retain tombstone keys |
| KeyFunc, KeyError, Error, Unwrap, ExplicitKey, MetaNamespaceKeyFunc, ObjectToName, MetaObjectToName, SplitMetaNamespaceKey | `KeyFunc`, `KeyError`, `keyErrorMessage`, `keyErrorCause`, `Store.getByKey`, `metaNamespaceKeyFunc`, `objectToName`, `metaObjectToName`, `splitMetaNamespaceKey` |
| StoreOption, WithTransformer, WithStoreMetrics, NewStore, NewIndexer | `newStoreWithOptions`, `newIndexerWithOptions`, `newIndexerWithTransformer`, `ThreadSafeStoreOptions.metrics`, `newStore`, `newIndexer` |
| `thread_safe_store.go`: ThreadSafeStore, ThreadSafeStoreWithTransaction, ThreadSafeStoreTransaction, ThreadSafeStoreOption, NewThreadSafeStore, WithThreadSafeStoreMetrics; all CRUD/index/transaction/RV methods | `ThreadSafeStore`, `ThreadSafeStoreOptions`, `newThreadSafeStoreWithOptions`, `KeyedOperation`, `newThreadSafeStore`, `threadSafeTransaction`, `threadSafeUpdate/Delete/Get/List/ListKeys/Replace/Index/IndexKeys/ByIndex/IndexValues/AddIndexers/ResourceVersion/Bookmark`; metrics belong to the storage owner |
| `index.go`: Indexer, IndexFunc, Indexers, Indices, IndexFuncToKeyFuncAdapter, NamespaceIndex, MetaNamespaceIndexFunc | `Indexer`, effectful `IndexFunc`, `Indexers`, captured nested index maps, `indexFuncToKeyFuncAdapter`, literal `"namespace"`, `namespaceIndex` |
| TypedIndexer, TypedIndexers, TypedIndexFunc, TypedIndexersToIndexers, TypedIndexerFuncToIndexerFunc, TypedIndex, ByTypedIndex, AddTypedIndexers | Same generic Indexer/IndexFunc family; `typedIndexer` provides strict raw views; no untyped erasure conversion required |
| `fifo.go`: PopProcessFunc, ErrFIFOClosed, Queue, QueueWithBatch, FIFO, NewFIFO; Close/HasSynced/HasSyncedChecker/Name/Done/Add/Update/Delete/IsClosed/Pop/Replace/Resync | `PopProcessFunc`, `ProcessResult`, `errFIFOClosed`/`isFIFOClosed`, `Queue`, `FIFO`, `newFIFO`, `fifo*`, `queue*`, `queueDoneChecker`; batch capability is `realPopBatch`/`newRealBatchController` |
| `delta_fifo.go`: DeltaFIFOOptions, DeltaFIFO, TransformFunc, DeltaType, Added/Updated/Deleted/Replaced/Sync, Delta, Deltas, NewDeltaFIFO, NewDeltaFIFOWithOptions | Corresponding native types, `newDeltaFIFO`, `newDeltaFIFOWithOptions`, `deltaAdd/Update/Delete/Replace/Resync`, `queuePop`; adjacent deletion deduplication in `dedupDeltas` |
| ErrZeroLengthDeltasObject, KeyOf, Transformer, HasSynced, Close, IsClosed, Name, Done, HasSyncedChecker, KeyListerGetter/KeyLister/KeyGetter, Oldest/Newest, DeletedFinalStateUnknown | `deltaKeyOf`, `isZeroLengthDeltasError`, `transform` field, shared queue methods, explicit named DoneChecker, Store read callbacks, `oldest/newest`, `DeletedFinalStateUnknown` |
| `the_real_fifo.go`: RealFIFOOptions, RealFIFO, ReplacedAllInfo, SyncAllInfo, BookmarkInfo, NewRealFIFO/NewRealFIFOWithOptions; all queue/transform/bookmark/batch methods | `RealFIFOOptions`, `RealFIFO`, `RealEvent` variants, `newRealFIFOWithOptions`, `realAdd/Update/Delete/Replace/Resync/Bookmark/Pop/PopBatch`, queue lifecycle helpers; unique-key batch boundaries |
| `heap.go`: LessFunc, Heap, NewHeap, Add/BulkAdd/AddIfNotPresent/Update/Delete/Pop/List/ListKeys/Get/GetByKey/IsClosed/Close; private heapData methods | Effectful `LessFunc`, `Heap`, `newHeap`, `heap*`; `Get` is key observation plus `heapGetByKey`; indexed heap swap/up/down/remove logic is native |
| `expiration_cache.go`: ExpirationCache, ExpirationPolicy, TTLPolicy, TimestampedEntry, IsExpired, NewTTLStore, NewExpirationStore; Store methods | `ExpirationCache`, `ExpirationPolicy`, `ttlPolicy`, `TimestampedEntry`, `newTTLStore`, `newExpirationStore`, `expirationStore` callbacks; strict age > TTL and lazy locked deletion |
| `expiration_cache_fakes.go`: FakeExpirationPolicy, IsExpired, NewFakeExpirationStore, fake Delete | `FakeExpirationPolicy`, `fakeExpirationPolicy`, `newFakeExpirationStore`, explicit deletion callback |
| `fake_custom_store.go`: FakeCustomStore and all methods | `FakeCustomStore` optional callback record, `newFakeCustomStore`; deliberately empty omitted callbacks match this test helper's contract |
| `undelta_store.go`: UndeltaStore, NewUndeltaStore, Add/Update/Delete/Replace | `newUndeltaStore` returns a Store that pushes its full current snapshot after successful changes |
| `object-names.go`: ObjectName, NewObjectName, Parts, String, ParseObjectName, NamespacedNameAsObjectName, AsNamespacedName | `ObjectName`, `newObjectName`, `objectNameParts`, `objectNameString`, `parseObjectName`, canonical namespaced-name conversions |
| `listers.go`: AppendFunc, ListAll, ListAllByNamespace, GenericLister, GenericNamespaceLister, NewGenericLister, List/Get/ByNamespace | `AppendFunc`, `listAll`, `listAllByNamespace`, generic lister records, `newGenericLister`/`newResourceLister`, `listerList/Get`, `byNamespace`, `namespaceListerList/Get` |
| `listwatch.go`: Lister/Watcher and WithContext variants, ListFunc/WatchFunc variants, ListerWatcher variants, To*WithContext, ListWatch, Getter, constructors, List/Watch methods, watch-list capability | Native callbacks and ListWatcher; `toListerWatcherWithContext`, `listWithContext/watchWithContext`, legacy methods, `ListWatchClient`, `newListWatchFromClient`, `newFilteredListWatchFromClient`, `restListWatchClient`, `toListWatcherWithWatchListSemantics` |
| `reflector.go`: ReflectorStore, ReflectorBookmarkStore, TransformingStore, ResourceVersionUpdater, Reflector, ReflectorOptions, NewReflector/NewNamedReflector/NewReflectorWithOptions, NewNamespaceKeyedIndexerAndReflector, Name/TypeDescription, Run variants, ListAndWatch variants, LastSyncResourceVersion, DefaultWatchErrorHandler, error-handler types, VeryShortWatchError/Error | `ReflectorStore` optional bookmark/RV callbacks, explicit transform, `Reflector`, `ReflectorOptions`/`newReflector`/`defaultReflectorOptions`, `newNamespaceKeyedIndexerAndReflector`, explicit names/descriptions, `reflectorRun/ListAndWatch/ResourceVersion`, error callbacks, `defaultWatchErrorHandler`, `VeryShortWatchError`/message/predicate |
| `retry_with_deadline.go`: RetryWithDeadline, NewRetryWithDeadline, After, ShouldRetry | `RetryWithDeadline`, `newRetryWithDeadline`, `retryAfter`, `shouldRetry` with shared clock |
| `reflector_data_consistency_detector.go` | `checkWatchListDataConsistencyIfRequested`; optional `ReflectorOptions.checkWatchListConsistency` before publication |
| `controller.go`: Config, ShouldResyncFunc, ProcessFunc, ProcessBatchFunc, Controller, New, Run variants, HasSynced/HasSyncedChecker/LastSyncResourceVersion, InformerOptions, NewInformer/NewIndexerInformer/NewTransformingInformer/NewTransformingIndexerInformer/NewInformerWithOptions | Native `Controller`/constructor parameters, `newController`/`newRealBatchController`, run/sync helpers, `InformerOptions`/`newInformerWithOptions`; optional indexers/transformers configure the same implementation |
| ResourceEventHandler, TypedResourceEventHandler, Funcs/DetailedFuncs variants, all OnAdd/OnUpdate/OnDelete methods, Filtering/TypedFilteringResourceEventHandler | Generic `ResourceEventHandler`, optional callback constructors, `filteringResourceEventHandler`, `rawHandler` typed view adapter; initial flag and all four filter transitions retained |
| Object, DeletedObject and GetObjectName/GetName/GetNamespace/GetKey, DeletionHandlingMetaNamespaceKeyFunc/DeletionHandlingObjectToName | ObjectAccess, `Deleted[A]`, `deletedObjectName`, structural name projections, `deletionKey`, deletion-handling helpers; absent objects remain Option |
| `shared_informer.go`: SharedInformer/SharedIndexInformer/TypedSharedIndexInformer, options and all constructors, typed index/handler methods | `SharedInformer[A]`, direct generic aliases, `SharedIndexInformerOptions`, constructors, `TypedInformerView`, strict typed views and handler/index adapters |
| ResourceEventHandlerRegistration, HandlerOptions, AddEventHandler/AddEventHandlerWithResyncPeriod/AddEventHandlerWithOptions, RemoveEventHandler, ShutDownEventHandler, registration HasSynced/HasSyncedChecker/ShutdownChan | Native registration identity, options, corresponding functions, `handlerHasSynced`/`handlerDone`; listener finished channel and shutdown join |
| SetWatchErrorHandler variants, SetTransform, Run variants, HasStarted/HasSynced/HasSyncedChecker/IsStopped, GetStore/GetIndexer/AddIndexers/GetController, LastSyncResourceVersion, OnAdd/OnUpdate/OnDelete | Native setters, lifecycle functions, existing store/indexer projection, read-only `getControllerStatus` (no legacy dummy Run), RV and distribution handlers |
| InformerSynced, WaitForNamedCacheSync variants, WaitForCacheSync, WaitFor, DoneChecker, IsDone, SyncResult/AsError | `InformerSynced`, native wait functions, named reporting callback, `DoneChecker`, `isDone`, `SyncResult` and `syncResultAsError`; completion waits do not poll |
| `identity.go`: InformerName, NewInformerName, WithResource, Release, Name, InformerNameAndResource, Reserved, GroupVersionResource, ResetInformerNamesForTesting | Explicit `InformerNameRegistry`, `newInformerName`, `withResource`, `releaseInformerName`, name/resource fields, `informerReserved`, reset helper; no implicit process global |
| `informer_metrics.go` and `reflector_metrics.go`: all provider/metric types, constructors and Set*MetricsProvider | Native callback records, explicit first-set-wins MetricsRegistry, `withStoreMetrics`, `configureQueueMetrics`; nanosecond timing and last-15-digit RV gauge conversion |
| `mutation_cache.go`: MutationCache, MutationCacheOptions, ResourceVersionComparator, integer-cache constructors, GetByKey/ByIndex/Mutation/OnAddOrUpdate/OnDelete/ObjectResourceVersion/CompareResourceVersion | Generic MutationCache constructor parameters and methods, exact decimal uint64 comparisons; LRU expiration, captured tombstone UID/RV; private tombstone fake-object mutators become an inaccessible native entry variant rather than panic stubs |
| `mutation_detector.go`: MutationDetector, NewCacheMutationDetector, AddObject/Run/CompareObjects and disabled implementation | Native detector, explicit enabled setting, effectful copy/equality and failure callback, two retained generations; disabled detector deliberately has no effects |
| `synctrack/synctrack.go`: AsyncTracker/SingleFileTracker, constructors, Start/Finished/UpstreamHasSynced/HasSynced/Done/Name | Native trackers and corresponding functions/fields; deduplicated keys versus counted sequential work, latched completion and negative-count failure |
| `synctrack/lazy.go`: Lazy, Evaluate, Notify, Get | Native Lazy callback/entry identities, `newLazy`, `notify`, `get`; successful values cached, errors retried, old generations cannot overwrite new ones |
| `testing/fake_controller_source.go`: source/PV/PVC source constructors/types, ResetWatch/Add/Modify/Delete/*DropWatch/Change/List/Watch/Shutdown | Generic FakeControllerSource instantiated for the resource type, usable raw default, effectful version setter/copy, list-result projection, history replay, reset expiry and subscription shutdown; the Go-only list wrapper types are replaced by typed ListResult |

## Retained regression sources

Native tests cover store CRUD/RV, partial transaction indices, transformer key ordering, replacement failure, late indices, FIFO close/write distinctions, FIFO/DeltaFIFO sync boundaries, adjacent deletion deduplication, queue ordering and missing-key resync, heap updates, expiration boundary/refresh, mutation UID/RV/LRU behavior, uint64 precision, RealFIFO atomic/batch boundaries, filters, identity release, raw reflector events/pagination/watch-list cancellation, independent sequential listeners, replay sync barriers, pending-discard/join, legacy list cancellation, sync trackers, lazy evaluation and source history/reset.

The released reconciliation gate ran the current compiler from source through the VM serializer (`--class short`) and a 180-second bound per command. `go run ./cmd/gofx test ports/client-go/tools/cache` passed 44 tests; the separate `synctrack` and `testing` directories passed 5 and 3 tests respectively. Imported package tests are not included in the root directory's count. No formatter, linter, live cluster, or repository-wide suite was run.

## Explicit application feature registry

Source-facing standalone construction uses `newSharedIndexInformerWithFeatures`, `newSharedInformerWithFeatures`, `newRawSharedIndexInformerWithFeatures`, or `newReflectorWithFeatures` with the application's `features.FeatureRegistry`. No constructor creates a hidden registry.

Generated resource constructors use the same application registry for WatchList
and AtomicFIFO through `newSharedIndexInformerWithFeatures`. Factories may also
install `setWatchListGate(informer, func() IO[Bool])` before starting an informer.
The setter rejects an already-started informer and is evaluated in
`buildSharedController` at reflector construction. `ListWatcher.unsupportedWatchList`
remains an independent opt-out.

A custom factory constructor owns the storage it creates. The factory does not
retrospectively replace that storage or its AtomicFIFO gate. Such constructors
must capture the application registry and call a `WithFeatures` constructor, or
pass `threadSafeStoreOptionsWithFeatures(registry, metrics)` to
`newSharedIndexInformerWithStoreOptions`. Custom `SharedConfiguration` users set
`atomicFIFOGate` to that adapter's `atomicFIFO` callback while retaining their
other options.

The no-registry constructors remain fixed-policy building blocks: WatchList is
explicitly disabled and AtomicFIFO is enabled (the pinned AtomicFIFO default).
They are not the source-facing mapping for constructors that consult application
feature gates. Installing only the deferred WatchList gate does not configure AtomicFIFO.

Informer resource reservations keep `(group, version, resource)` as their key. A private lexicographic String comparator is passed to the existing `mapMemberBy` and `mapInsertBy` operations; no `Ord[Tuple3]` instance, delimiter encoding, or alternate GVR type is required. The identity regression distinguishes changes to each of the three components and verifies duplicate reservation and release behavior.

## Final capability reconciliation

- `ReflectorOptions.typeDescription` is independent of `name`. Empty descriptions
  derive from `expectedType`: absent is `<unspecified>`, a typed object uses its
  Go type name, and a nonempty unstructured GVK uses `group/version, Kind=kind`.
  `reflectorTypeDescription` exposes the resolved value. Failed-list errors use
  that description. The existing canonical object-type and GVK effects supply
  only observations; selection and formatting remain native policy.
- `ReflectorOptions.onError` and `setWatchErrorHandler` accept
  `func(Reflector[A], errors.Error) IO[Unit]`. Both run-loop errors and store-write
  errors pass the actual reflector, retaining access to state, options and RV.
- `Store.transformer` and `ReflectorStore.transformer` declare transformation
  discovery. Store/DeltaFIFO/RealFIFO adapters and typed views retain it.
  Watch-list temporary storage applies the discovered transform before
  consistency checking and final replacement. Its private bookmark observation
  stays available independently of the public store's gated RV projection.
- `ThreadSafeStoreOptions` contains `atomicFIFO: func() IO[Bool]` and
  `metrics: Option[(InformerNameAndResource, InformerMetricsProvider)]`.
  `threadSafeStoreOptionsWithFeatures(registry, metrics)` consults the supplied
  registry on each RV read; no global registry is created. The explicit
  no-registry default is enabled, matching the pinned AtomicFIFO default.
  `SharedConfiguration.atomicFIFOGate` is separate from queue `atomicEvents`.
  A disabled gate returns `""` from both direct storage and Store views without
  discarding the stored RV. Metadata-free deletes preserve that stored value.
- Metrics are initialized and emitted by storage, not a Store decorator.
  Update/delete-with-object/replace/bookmark/transaction observe committed RVs
  under the storage lock. Missing metadata does not emit; invalid numeric RVs
  remain stored but do not emit. Metrics truncate to the last 15 digits and stop
  after identity release, independently of whether RV projection is enabled.
- `ResourceVersionComparator[A]` is
  `func(A,A) IO[Outcome[Ordering]]`. `newMutationCache` accepts it and every
  mutation/backing/tombstone comparison uses it. The integer constructor delegates
  through `compareIntegerResourceVersion`. Upstream exposes the comparator
  interface but only installs its integer implementation through its public
  constructor; the native constructor makes the declared capability usable
  rather than retaining an inaccessible private field.

`capability_test.gfx` adds nine focused proofs for these contracts, including
run-loop, watch-store and controller error callbacks, disabled/enabled projection
and registry replacement, direct metrics, metadata-free tombstones, noninteger
comparators and custom-store transform discovery. Existing cancellation/lifecycle
regressions remain unchanged.

`consumer/main.gfx` is a permanent separate public-import regression consumer.
`go run ./cmd/gofx run ports/client-go/tools/cache/consumer` passed and observed
direct metrics `[12.0]` while RV projection was empty, Store projection changing
from empty to `13`, discovered-transform sequence `11`, custom-comparator sequence
`50`, and a callback modifying the actual reflector RV to `from-handler`.
The root test gate took 13.16 seconds with 1,848,900 KiB peak RSS; the consumer
took 12.46 seconds with 1,945,196 KiB peak RSS. These are focused local observations,
not a claim of repository-wide or live-cluster parity.

