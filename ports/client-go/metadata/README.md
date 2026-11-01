# Native metadata client

Port of client-go **v0.37.2**, revision `72c52dd0cc3b9099b36af11290b0304045dd1ada`; Kubernetes-derived source is Apache-2.0. `coverage.json` provides exported-symbol and upstream test-file provenance.

## Surface

`newForConfig(environment, config)` and `newForConfigAndClient(environment, config, httpClient)` return `IO[Outcome[Option[Interface]]]`; `newForConfigOrDie` raises the canonical error. `configFor` copies configuration and uses the canonical metadata negotiated serializer. `fromRESTClient` supplies the codec adapters; `newClient(Client)` accepts the explicit REST/codec capabilities.

`Interface.resource(gvr).namespace(ns)` is immutable. `ResourceInterface` includes get, list, patch, delete, deleteCollection, and watch. Arguments preserve the canonical metav1 options; patch bytes/media type are lossless String values and subresources are `List[String]`. Results are `IO[Outcome[Option[PartialObjectMetadata]]]`, corresponding list/watch forms, or `Outcome[Unit]` for deletion. The upstream Getter and ResourceInterface capabilities are combined into this native record.

## Protocol distinctions

- Object and watch requests prefer `application/vnd.kubernetes.protobuf;as=PartialObjectMetadata;g=meta.k8s.io;v=v1`, then the JSON metadata representation, then ordinary JSON. List requests use the corresponding `PartialObjectMetadataList` negotiation, in the same order.
- Delete and delete-collection bodies are encoded as metav1 v1 **JSON**, regardless of the preferred protobuf content type, so CRD handlers accept them.
- Watch sets `watch=true` and applies `timeoutSeconds` to the request timeout in nanoseconds.
- Only a canonical unregistered-type decoding failure enters the ordinary-JSON metadata fallback. Missing kind/version, malformed serialization, and registered unexpected object types remain distinct errors. Fallback object metadata must contain UID, creation timestamp, name, or generateName; fallback outer TypeMeta is cleared. List item TypeMeta is retained.
- The metadata client intentionally preserves upstream's narrower name validation instead of adopting dynamic-client path checks.

## Fake and cache surfaces

`fake.newTestScheme`, `newSimpleMetadataClient`, `resource`, `namespace`, `createFake`, `updateFake`, `updateStatus`, and the read/delete/watch/patch operations use the native testing tracker and reactor chain. `fake.interface(client)` provides the ordinary metadata client capability and sets `unsupportedWatchList=true`. Fake create/update/get/patch intentionally discard legacy option values, while delete retains DeleteOptions. Fake list uses the registered synthetic metadata list kind, checks every returned item type, preserves list metadata, and filters labels only.

`newTestScheme` invokes the apimachinery `NewScheme` factory so registration
maps are initialized; a zero-valued Scheme is not a usable registration target.

`metadatalister` offers typed cache listers and runtime object projections. `metadatainformer` shares the generic factory lifecycle implemented by dynamicinformer. Every standalone/factory constructor takes the application's `features.FeatureRegistry` first, matching GeneratedInformers. Constructors support resync, namespace, IO list-option tweaks, and `newSharedInformerFactoryWithOptions(registry,client,resync,[withTransform(transform)],clock)`. WatchListClient is looked up through a deferred cache gate when the reflector is built during run; unsupported fake clients remain independently excluded. Transform applies before the informer can start; repeated GVR requests retain identity. Context is the executing IO context, including the upstream StartWithContext capability. Shutdown waits for all started workers and prevents subsequent starts.

Regression sources cover Accept ordering, legacy fallback and metadata plausibility checks, wrong registered types, JSON deletion, fake option quirks, typed lister isolation, factory identity, and transform installation.

**Verification has not been run.** The assignment explicitly reserves parser/typecheck/build/test acceptance for the main integration agent.
