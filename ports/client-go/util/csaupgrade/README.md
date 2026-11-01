# Client-side to server-side apply migration

Native policy port of pinned client-go v0.37.2 `util/csaupgrade/upgrade.go` and `options.go`, with source hashes captured by the apply configuration generator.

- `findFieldsOwners(entries, operation, fields)` returns entries whose decoded fieldpath set contains the requested set; malformed entries are ignored, preserving input order.
- `upgradedManagedFields(entries, csa, ssa, subresource)` is the native entry-list operation. It preserves nil, finds the first SSA manager or converts the first matching CSA manager, unions only the first matching CSA entry with the same API version, removes all matching CSA update entries, and leaves other managers/subresources unchanged.
- `upgradeManagedFields(object, csaManagers, ssaManager, options)` returns `IO[Outcome[Option[runtime.Object]]]`. Unlike the mutating Go API, it returns an updated snapshot and leaves the original unchanged. Manager names are deduplicated; options use `subresource(name)`, with the last option winning.
- `upgradeManagedFieldsPatch(...)` returns `IO[Outcome[Option[String]]]`. `None` means no changes. The JSON patch replaces `/metadata/managedFields` and `/metadata/resourceVersion`, retaining the resource-version optimistic concurrency guard and the upstream replace rather than test operation.

Errors from fieldpath decoding/encoding are wrapped with the original messages `failed to convert fields to set` and `failed to encode field set`. Metadata-access failures retain the external error. All migration decisions and patch selection are native `.gt`; host code only provides fieldpath operations, snapshot metadata conversion, equality and JSON codecs.

Prepared regression cases cover union, API-version selection, subresource isolation, malformed sets, containment, nil/no-op paths, patch contents and immutable input. No tests/builds were run in the implementation slice; main integration owns verification.
