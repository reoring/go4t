# Native generic clients

This directory implements `gentype/type.go` and `gentype/fake.go` from Kubernetes client-go **v0.37.2**, revision `72c52dd0cc3b9099b36af11290b0304045dd1ada`. Kubernetes-derived source is copyright The Kubernetes Authors and licensed under [Apache-2.0](https://www.apache.org/licenses/LICENSE-2.0).

## Composition

The four real and four fake client capability records use explicit composition rather than embedded Go methods. A list/apply-capable record exposes its base as `.client` and its adapters as `.list` and `.apply`. Generated typed clients use these same constructors and operations; there is no dynamic-only alternative implementation.

- `ObjectAdapter[T]`: canonical `ObjectType[T]`, empty-object constructor, object name getter.
- `ListAdapter[T,L]`: canonical list descriptor, empty-list constructor, immutable list metadata copier, item getter and setter. Metadata copying takes `(destination, source)`. Item setters preserve upstream nil-list behavior by mapping `Nil` to an absent canonical item slice.
- `ApplyAdapter[C]`: optional name getter plus `util/apply.Marshal[C]` JSON and CBOR encoders. An absent configuration is represented by `Option[C]`.
- `ClientOption[T]`: immutable client transformation. `prefersProtobuf()` is the upstream `PrefersProtobuf` equivalent; `Option` remains the standard optional type.

Real constructors take resource, REST interface, parameter codec, namespace, object adapter, optional capability adapters, feature registry for apply-capable clients, and an explicit list of client options. Fake constructors take the shared `testing.Fake`, namespace, GVR, GVK and the same typed adapters.

`upstream-symbols.json` maps all 47 exported declarations in the two pinned production files to the native implementations. The map is source provenance, not a compilation or behavioral test result.

## Operations

Base operations are `get`, `create`, `update`, `updateStatus`, `delete`, `patch` and `watch`. `list` and `deleteCollection` operate on the list capability; `apply` and `applyStatus` take the apply adapter and feature registry. Fake equivalents have a `fake` prefix.

All network operations execute the native REST API. List/watch/deletion-collection preserve timeout seconds as integer nanoseconds, watch sets the canonical watch option, and status operations select the status subresource. Object and option bodies use canonical runtime objects and the supplied parameter codec. The base REST interface and namespace are available through `getClient` and `getNamespace`.

Ordinary object operations return `IO[Outcome[T]]`. Apply returns `IO[Outcome[Option[T]]]`, preserving nil validation results versus a freshly allocated request destination. Watches return the shared `support/watch.Watch`; no separate generic-client watcher exists. Both object data and an accompanying error survive partial results.
If the decoded runtime object cannot be recovered through the client's canonical descriptor, the empty destination is returned only with an error: the original decoding error when present, otherwise `decoded object does not match the client object type`. A cast failure can never become successful empty server data.

### Deliberate fake differences

- List filters only labels; field selectors are retained in recorded options but do not filter returned items.
- List metadata is copied from the reactor's result to a fresh list.
- A nil reactor result returns the empty object/list constructor result and retains its error.
- A non-nil result of the wrong concrete type is an IO failure, not a silently synthesized empty value.
- Fake apply marshals JSON before validating a missing name, whereas real apply validates the name before invoking the apply request encoder.
- Fake apply always records the YAML apply patch media type, even though its payload is JSON.
- Native immutable slice values use explicit `sync.Ref` cells in `toPointerSlice`/`fromPointerSlice`; absence and empty slices remain distinct. These helpers do not pretend immutable list slots can be mutated through an implicit Go pointer.

Validation strings preserve the upstream distinction:

| Operation | Error |
| --- | --- |
| Real apply, absent configuration | `object provided to Apply must not be nil` |
| Real apply, absent name | `obj.Name must be provided to Apply` |
| Fake apply, absent configuration | `configuration provided to Apply must not be nil` |
| Fake apply, absent name | `configuration.Name must be provided to Apply` |

## Regression sources and status

`gentype_test.gt` exercises actual native client/fake operations: exact CRUD/options/subresource records, nil and partial results, label-only filtering, metadata preservation, apply validation order, real REST request paths/bodies, empty-name rejection and explicit pointer cells.

**No parser check, typecheck, build, formatter, linter or test was run by this owner.** Integrated verification belongs to the supervisor. Tests written here are not evidence of passing tests.
