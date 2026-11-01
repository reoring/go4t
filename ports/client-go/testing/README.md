# Native testing, reactors and object tracking

This directory implements the testing package from Kubernetes client-go **v0.37.2**, revision `72c52dd0cc3b9099b36af11290b0304045dd1ada`. Kubernetes-derived code is copyright The Kubernetes Authors and licensed under [Apache-2.0](https://www.apache.org/licenses/LICENSE-2.0).

`upstream-symbols.json` maps every one of the 176 exported production declarations in `actions.go`, `fake.go`, `fixture.go` and `interface.go` to its native equivalent. Repeated methods and Go action interfaces map to native tagged action records and their explicit accessors; they are not duplicate alias types. Internal list/watch regression behavior is covered in `tracker_test.gfx`.

## Actions

`Action` carries its actual action kind as well as verb/resource/namespace/subresource/name. This distinction matters: create and update have structurally similar fields but are not interchangeable in `objectReaction`.

All pinned root, namespaced, subresource, default-options and explicit-options constructors are provided with lowerCamel names. List, watch and delete-collection construction is effectful because selector parsing may fail. `newRootListActionWithOptions` intentionally discards non-selector list options; namespaced `newListActionWithOptions` preserves them. Patch subresource segments use Go path joining.

Action options are canonical Kubernetes values. `getListRestrictions` and `getWatchRestrictions` expose canonical label and field selectors. `matches` uses Unicode case folding; reactor wildcard/resource matching is case-sensitive. The proxy fields retain scheme, port, path and an immutable native parameter map.

`actionDeepCopy` clones only the active action's mutable object/option fields. Native strings and maps need no defensive allocation. `Action.value: Option[Payload]` is the genuinely generic upstream value slot: it carries arbitrary native scalars, records, references and functions, without weakening `Action.object: Option[runtime.Object]`. The former disconnected `GenericAction[A]` wrapper is removed.

Allocate a typed witness with `newPayloadKey[A]() IO[PayloadKey[A]]`, then use `newGenericAction(verb, resource, namespace, key, value) Action` or `withValue(action, key, value) Action`. `getValue(action, key) Option[A]` retrieves the payload from any reactor or recorded action. The leaf operations are `packPayload(key, value) Payload` and `payloadValue(key, payload) Option[A]`. Keys and boxes are opaque nominal handles following the existing generic-effect ABI. Retrieval requires the identical allocated key: a different key returns `None`, even when its type parameter is the same. There is no unchecked cast API, reflection-based type-name matching or closed dynamic-value encoding. Keep the typed key alongside the capability that creates and consumes its payload.

Both action copies retain the exact payload box. Mutations through a payload's reference or captured function therefore reach the caller, all reactors and the ledger. Replacing an action's fields or its payload box in a reactor still affects only that invocation's shared action, not the original or the recorded snapshot.

`runtimeObjectPayloadKey() PayloadKey[runtime.Object]` is a pure, stable built-in witness for existing runtime-object generic slots such as PodLogs. Producers box the canonical object with this key; callbacks retrieve it with `getValue(action, runtimeObjectPayloadKey())` before applying the canonical typed object view. This compatibility representation does not constrain caller-created payload keys or erase typed Kubernetes client fields.

## Fake and reactor chains

`newFake()` constructs an explicit aggregate with a mutex and state reference. `Reactor`, `WatchReactor` and `ProxyReactor` expose `handles` and `react`; their simple constructors capture verb/resource matching. Append/prepend operations, resource access, history access and invocation are all IO.

Every invocation deep-copies the original action separately for recording and for reactor execution. All reactors in that invocation receive the **same** mutable `sync.Ref[Action]`. An unhandled reactor's changes are therefore visible to the next reactor, but not to the original action or recorded snapshot. The fake lock covers recording and chain traversal, as upstream does.

- Object chains ignore unhandled results and fall back to the supplied default object.
- Watch chains fail if nobody handles the action.
- Proxy chains ignore either unhandled results or errors and return `None` if none succeeds.
- `actions` returns chronological history; `clearActions` clears only history.
- `FakeClient{fake,tracker}` composes the testing interface without hidden global state.

## Object trackers

`newObjectTracker(runtime.Scheme, runtime.Decoder)` returns `IO[ObjectTracker]`. `newObjectTrackerWithScheme(ObjectScheme, decoder)` supports custom new-object and object-kinds callbacks. `newFieldManagedObjectTracker(scheme, decoder, typeConverter)` wraps the same native base store with managed-field policy.

`ObjectTracker` exposes add/get/create/update/patch/apply/list/delete/watch callbacks. Other than add, operations accept a list of zero or one canonical options values. Two or more options return exactly `expected only one option argument but got N`.

The base store:

- Keeps per-GVR object maps and monotonically increasing internal versions; stored object metadata is not rewritten with those internal versions.
- Copies submitted/stored/returned data at ownership boundaries, propagates an empty namespace, rejects a conflicting namespace, and returns canonical Kubernetes NotFound/AlreadyExists/BadRequest errors.
- Adds all items from lists and every scheme-recognized kind, preserving the partial-metadata override and internal-version resource mapping.
- Lists by deterministic namespace/name order and sets ListMeta resourceVersion. As upstream, deleting an object does not increment the collection's version.
- Replays no existing objects when Watch receives no options. Passing options enables replay of objects newer than the requested internal version, which closes the list-to-watch gap.
- Uses the shared watch type, a 100-element buffer, serialized stop/enqueue, and ignores notifications after stop. Stop is idempotent and buffered events remain readable.
- Explicitly reports that watch-list semantics are unsupported.

`objectReaction` dispatches on actual action kind. It does not implement delete-collection, generic or proxy actions. Create on a subresource updates an existing same-type object; differing types return the submitted object without persisting it. Update-status remains the deliberately coarse upstream fake behavior, not simulated API-server status admission.

JSON patch, merge patch and strategic merge patch dispatch live in native source. Go adapters call only the external patch libraries/codecs and preserve their error identity. The unsupported-patch error is exactly `PatchType TYPE is not supported`. Legacy apply requires an existing object and uses strategic merge; field-managed apply supports creation, ownership conflicts, force takeover and normal update ownership. It reconstructs the mapper per operation so additions to the supplied scheme are visible. The explicit native object-defaulter callback retains the upstream choice not to run API-server defaulting.

## Boundaries and verification

- `actions.gfx`, `constructors.gfx`, `fake.gfx`, `tracker.gfx`, `managed.gfx`, `reaction.gfx` and `interface.gfx` contain client-go policy and state transitions.
- `internal/operations.gfx` contains typed external effect adapters.
- `host/operations.go` contains metadata/type conversion, dependency codec/patch calls and external managedfields invocation, never an original client-go import or tracker.
- `host/fixtures.go` constructs external dependency values and HTTP test responses only.
- `payload.gfx` and `host/payload.go` provide only typed witness allocation, opaque boxing and identity-checked retrieval through the generic effect ABI; no canonical dependency changes are needed.

`tracker_test.gfx` contains consumer-visible regressions for exact records, original-vs-reactor copy isolation, reactor order and partial results, options and root-list quirks, mutable generic payload aliasing and wrong-key rejection, CRUD errors, watch replay and stop, all three patch families, managed conflicts/force and scheme reload. `examples/generic-payload/main.gfx` is intentionally retained as an executable public-API regression, not a new application example: it passes a scalar, a native record, and a record holding a mutable reference and captured function through object, watch and proxy reactor chains and recorded-copy lookup, and asserts ordinary action namespace isolation.

Focused validation on 2026-11-02 used the current compiler with serialized `--class short` admission and a 180-second ceiling, from `ports/client-go`:

- `go test ./testing/host -run TestPayloadWitnessIdentityAndShallowValue`: passed.
- `go run ../../cmd/gofx test testing`: 12 passed, 0 failed, including original-object snapshot isolation and generic payload aliasing/wrong-key rejection.
- `go run ../../cmd/gofx run examples/generic-payload/main.gfx`: exit 0; all three payload scenarios completed. Each exercises six reactor callbacks and three recorded-copy lookups. The initial run exposed an invalid unit literal in this new consumer; replacing it with `unit()` resolved the parser error before the successful rerun.

These checks do not claim repo-wide or generated-client acceptance. No shared-runtime or manifest files were modified by this owner.
