# Native scale client and conversion scheme

Port of client-go **v0.37.2**, revision `72c52dd0cc3b9099b36af11290b0304045dd1ada`; Kubernetes-derived code is Apache-2.0. `coverage.json` maps the exported production capabilities and source/test provenance, including all seven scale scheme directories.

## Client

`PreferredResourceMapper{resourceFor}` and `ScaleKindResolver{scaleForResource}` are native IO capability records using canonical GVR/GVK and Outcome. `newClient(restInterface, mapper, apiPathResolver, scaleKindResolver)` returns `IO[ScalesGetter]`; `newForConfig(environment, config, mapper, resolver, kindResolver)` returns `IO[Outcome[Option[ScalesGetter]]]`. The path resolver is `dynamic.APIPathResolverFunc`; `dynamic.legacyAPIPathResolverFunc` handles standard core/grouped paths.

`ScalesGetter.scales(namespace)` provides:

- `get(GroupResource, name, GetOptions)`
- `update(GroupResource, canonical autoscaling/v1.Scale, UpdateOptions)`
- `patch(GroupVersionResource, name, patchType, bytes, PatchOptions)`

All return `IO[Outcome[Option[autoscaling/v1.Scale]]]`. Get/update resolve the preferred GVR through the supplied mapper, not a static resource table. Patch uses the caller's explicit GVR. Namespace is attached only when nonempty. The `scale` subresource and options use the shared immutable REST request API. API failures preserve their canonical underlying status errors.

`newDiscoveryScaleKindResolver(discoveryInterface)` caches successful results by captured GVR values. Failed/missing lookups are not cached. The first matching `<resource>/scale` discovery entry wins, preserving upstream ordering even for ambiguous entries. Explicit discovery group/version is used only when **both** are present; otherwise the requested resource's group/version is retained. Fetch, missing subresource, mapper, conversion and encoding failures retain distinct message contexts.

## Native conversion model

The internal scale is native `scale/scheme.Scale{typeMeta, objectMeta, spec, status}`; `ScaleSpec{replicas}` and `ScaleStatus{replicas, selector:Option[metav1.LabelSelector]}` preserve exact counts and nil selectors. `deepCopy` and `deepCopyStatus` copy external metadata/selectors; ScaleSpec is an immutable scalar value.

`ScaleValue = InternalScale(Scale) | ExternalScale(runtime.Object)` makes the previously hidden internal/external boundary explicit. `Scheme{registrations, internalGroups}` is an immutable native registration aggregate, not a duplicate external runtime.Object type. Each scheme subpackage supplies group/version/resource metadata and `addToScheme`; each external version also exposes `registerConversions`, `fromScale/toScale`, spec conversions and status conversions. These are the native equivalent of SchemeBuilder plus generated/manual conversion registrations.

The four external versions are the exact canonical API types: autoscaling/v1, extensions/v1beta1, apps/v1beta1, apps/v1beta2. Internal autoscaling, extensions and apps registrations complete the upstream seven-version list. `newScaleConverter` composes them with the external codec factory; `convertToVersion` accepts/returns ScaleValue, preserving partial Outcome values on failures. `ScaleConverter.scheme` and `.codecs` expose the respective aggregates.

Selector conversion is native policy: external autoscaling string selectors are parsed to LabelSelector; beta versions prefer nonempty targetSelector, otherwise copy the optional legacy selector map. Outbound beta legacy maps are set only when no matchExpressions exist. The target string is generated with canonical apimachinery selector validation and formatting. Nil, empty, invalid selectors, and unknown target versions remain distinct. Go adapters only invoke external selector parsing/formatting and codecs; no client-go implementation is imported.

The selector host boundary uses the erased extern ABI and the canonical
`ErrorOptionFromGo` conversion. It preserves nil/error presence and the original
Go error rather than treating an error interface as an ordinary dependency value.

`scale/fake` records native testing actions. It intentionally ignores Get/Update/Patch options, uses a versionless GVR for get/update, preserves explicit patch GVR, and raises on an impossible response type as upstream type assertions do.

Native regression sources cover every external conversion version, selector errors, unknown versions, arbitrary discovered resources, ambiguous first-match behavior, and successful-only caching. **No parser/typecheck/build/linter/formatter/test command was run; integrated acceptance is supervisor-owned.**
