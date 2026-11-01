# Canonical external dependency bindings

This tooling binds **external** Go dependencies for the native client-go port. It never imports or implements `k8s.io/client-go`. Client-go policy remains in go4t. Go code here performs dependency conversion, typed field access, external calls, callbacks, and generic effects only.

## Inputs and regeneration

From `ports/client-go`:

```sh
python3 tools/binddeps/generate.py
```

For an isolated regeneration comparison, use a separate module-shaped output tree:

```sh
python3 tools/binddeps/generate.py \
  --contract tools/binddeps/contract.json \
  --target tools/binddeps/target.json \
  --output-root /tmp/client-go-binddeps-shadow
```

`--output-root` redirects every generated write and stale-generated-file deletion
to that tree. Contract, target, mode corrections, hand-written native declarations
and the frozen coverage baseline are still read from the existing inputs. It
rejects the live module root. Without this option, generation retains its normal
live-output behavior. Compare the shadow first; never replace integrated host
source merely to refresh metadata.

When only hand-written external declarations change, refresh the ledger and its
coverage mirror without rewriting any generated bindings:

```sh
python3 tools/binddeps/generate.py --manual-adapters-only
```

This mode and full generation use the same declaration extractor. It preserves
every other coverage field and also accepts `--output-root` for an isolated
two-file metadata shadow; full shadow generation remains the binding
reproducibility check.

The public-host cutover shadow comparison emitted 233 files: 232 were byte-identical
to the live outputs. The sole difference was 74 `coverage.manual_adapters` host
paths from the retired `internal/hostdeps` location to public `host/deps`.
The regenerated manual ledger matched the live `manual-adapters.json`; all
other coverage fields, native declarations and generated Go output matched.
Input and live-output hashes were unchanged by shadow generation. After the
approved application of only the generated `deps/coverage.json`, the exact
shadow/live byte comparison matched all 233 of 233 emitted files. This repairs
metadata provenance only; it is not a build or behavioral acceptance result.

After adding the generic tagged-error adapter, the manual ledger contains 75
operations. A fresh full `generate.py --output-root` shadow emitted 233 files;
all 233 were byte-identical to the live outputs, including the ledger and
coverage mirror. No live bindings were overwritten by that shadow generation.

Inputs and the generated adapter ledger are committed, not session artifacts:

- `contract.json`: the pinned, body-free external AST graph, exact module download records, source hashes and original authority. It includes the original client-go signature closure plus all exported types in reached API/apimachinery packages and `api/errors.APIStatus`, needed for dynamically decoded/status objects.
- `target.json`: Go build-constraint selection for linux/amd64 with Go 1.26.1. Other platform variants remain in the input graph; they are not silently selected by declaration order.
- `manual-adapters.json`: generated literal native signatures from the hand-written `deps/support`, `deps/codec` and `deps/objects` declarations. It is refreshed by the generator, not independently edited.
- `extract.go` and `bodyroots.go`: the reproducible Go AST extractor. They resolve imported package spellings from package clauses, not regular-expression guesses. `modules.json` records exact source roots, versions and Go module checksums. For an authorized signature input refresh, run `go run tools/binddeps/extract.go tools/binddeps/bodyroots.go PINNED_CLIENT_GO_ROOT tools/binddeps/modules.json`; this emits the body-free AST graph. Do not replace the accepted graph with that signature-only output during body-root completion. Updating the pinned input/authority is separate from ordinary binding regeneration.
- `select.go`: uses `go/build.MatchFile` rather than an ad hoc build-tag parser. `go run tools/binddeps/select.go tools/binddeps/contract.json` emits a target selection. Refreshing target metadata requires the exact downloaded source files recorded by the graph.

Run compiled extraction/selection and proofs under the repository's serialize-vm-tests wrapper. None of these commands is a test suite or formatter. Generator outputs are atomically replaced per file. Stale files bearing this generator's marker are removed; hand-written files are not deleted.

The API and apimachinery inputs are **v0.37.2**. API tag revision: `1bc0c529309723075aed339d50f16d4b311d908d`. Apimachinery tag revision: `97adf147c7378229d45db0fc7511af84d0c3c092`. Client-go inventory authority: `72c52dd0cc3b9099b36af11290b0304045dd1ada`. Each generated file identifies the source graph SHA-256. Kubernetes-derived declarations retain Apache-2.0 attribution.

### Approved body-root completion

The additive body pass preserves the accepted graph instead of replacing it with
the older signature-only extractor output:

```sh
go run tools/binddeps/extract.go tools/binddeps/bodyroots.go \
  /workspace/wip/go4t/.tmp/client-go-v0.37.2 \
  tools/binddeps/modules.json tools/binddeps/contract.json
go run tools/binddeps/select.go tools/binddeps/contract.json
```

The first command updates the contract in place; capture the second command's
JSON stdout as `target.json`, then run the generator. Both Go commands use the
serialization wrapper above. The pass records each imported exported
`SelectorExpr` in non-test function bodies and value initializers with file/line
anchors. Local identifiers resolved by the AST are excluded. No external bodies
are translated. Standard-library references bind actual Go operations; this is
not a port of the standard library.

Coverage keeps signature roots, original-body roots, the frozen 112 consumer
operations, and unused available closure distinct. Existing canonical identities,
modes, field types and call signatures are checked before any generated output
is replaced. A drift fails with `frozen dependency ABI changed: SECTION ID`.
New references do not expand the frozen operation set. In particular Backoff
remains a five-field snapshot.

`mode-corrections.json` records narrowly approved corrections to the frozen
classification, with source anchors and exact old/new metadata. The guard permits
only those listed transitions; nominal IDs, unrelated modes and signatures stay
frozen. Main approved `k8s.io/apimachinery/pkg/version.Info` from identity to
snapshot: `version/types.go:22-46` is a thirteen-string value DTO and both its
`String` and generated `OpenAPIModelName` methods have value receivers.
`newInfo()` and all `infoGet*`/`infoWith*` operations are pure; old identity
accessors are removed, not aliased. `infoCallString` and
`infoCallOpenAPIModelName` follow the existing snapshot-method convention,
returning `IO[(Info,String)]`. Native `pkg/version.getWith` remains pure.
The correction rationale and source anchors are also in coverage under
`approved_mode_corrections`.

The root pass exposes `spec3.OpenAPI`, `managedfields.NewTypeConverter`, and the
pinned import `k8s.io/apimachinery/pkg/util/httpstream/spdy.RoundTripperConfig`.
The latter is the actual pinned client-go spelling, not an invented
`k8s.io/streaming` alias. `deps/time.unix(Int,Int) Time` is the explicit pure
construction exception to effectful external calls; it delegates to `time.Unix`
and preserves nanosecond normalization.

## One owner for each type

`deps/types/types.gt` is the **only nominal declaration owner**. Its deterministic names contain the actual Go name and a qualified-identity hash. `deps/<Go import path>/values.gt` exports readable aliases and operations, for example:

```go4t
import core "github.com/reoring/gofx/ports/client-go/deps/k8s.io/api/core/v1"
import meta "github.com/reoring/gofx/ports/client-go/deps/k8s.io/apimachinery/pkg/apis/meta/v1"

func namedPod(name String) core.Pod {
 let metadata = meta.objectMetaWithName(meta.newObjectMeta(), name)
 core.podWithObjectMeta(core.newPod(), metadata)
}
```

Those aliases reference the same canonical type when imported by multiple consumers. A Go `type X Y` remains distinct; a real Go `type X = Y` shares its target. The generator does not decide aliases from explanatory comments. Anonymous interface/struct shapes receive structural canonical adapters; they are not mislabeled as named Go types. Prelude collisions use `Go` suffixes, such as `MapGo` and `ListGo`.

No client-go object is replaced by universal JSON, strings or dyn. The `support.Any` handle appears only for actual Go `any`/empty-interface positions. Its presence in an external type does not erase the enclosing type.

### Typed runtime.Object descriptors

`deps/types` owns `ObjectType[A]`; `deps/objects` re-exports it and provides:

```go4t
toObject[A](descriptor ObjectType[A], value A) runtime.Object
fromObject[A](descriptor ObjectType[A], value Option[runtime.Object]) Option[A]
```

Every known concrete runtime.Object implementor has a cached descriptor, including `core.podObjectType()`, `core.podListObjectType()` and each versioned `scaleObjectType()`. `coverage.object_descriptors` records all 352 source identities and their entry points. A descriptor stores the exact Go `reflect.Type`, not a newly allocated empty object. Cast-back checks that type and rejects nil, typed-nil and other package/version types.

Upcasts and casts are views of the same owned storage. Unaddressable object values are boxed once when entering the canonical boundary, not copied on each conversion. Existing per-type `ToRuntimeObject`/`FromRuntimeObject` functions delegate to the same descriptor implementation.


## Values, presence and identity

| Go shape | Native boundary |
|---|---|
| Named model/enum/scalar | Its own canonical opaque typed value; scalar `Value`/`FromValue` operations preserve named identity |
| Signed integers | Exact signed-64 native `Int`; narrowing conversions reject overflow |
| `uint64`, `uint`, `uintptr` | `support.UInt64`; high/low words each checked in `0..4294967295`, never truncated |
| Pointer field | `Option[T]`; nested pointers retain nested presence |
| Slice | `Option[List[T]]`; absent and allocated-empty differ |
| `[]byte` | `Option[String]`, byte-preserving, with no text decoding |
| Array | Length-checked `List[T]` |
| Go map | `support.ForeignMap[K,V]`, retaining Go key equality, nil/empty and entry multiplicity; no invented native Ord |
| Unnamed pointer map key | `support.PointerKey[A]`, retaining pointer identity rather than lowering to a pointee value |
| Channel | Typed external channel capability with IO send/receive/close |
| Function | Typed IO callback, or a named callable value with a typed Invoke operation |
| Interface | Canonical capability, typed method operations and callback factories where Go permits implementation |

When a Go struct `T` is required by value, the conversion boundary accepts a
canonical nonnil `*T` only when its pointee has exactly the requested
`reflect.Type`. It supplies `Elem()` so the actual Go call or assignment performs
the required value copy, without an extra deep copy. This is needed for
identity-backed constructors consumed in value slices such as
`[]runtime.SerializerInfo`. Interface conversion still runs first and preserves
the original pointer identity. Pointer/Option conversion is unchanged; nil
pointers cannot become struct values, distinct named structs are not coerced,
and Option layers are not flattened.

The focused Go check
`go test ./host/deps -run '^TestIdentityStructValue' -count=1` passed
under the serialization wrapper. It verifies actual `reflect.Call`/`Set`
by-value copies, unchanged source fields, retained nested pointer identity,
unchanged direct interface/pointer conversion, and rejection of nil pointers,
distinct named structs, unrelated structs, nonstruct targets and Option wrapping.
The native callback proof
`go run ./cmd/go4t run ports/client-go/tools/binddeps/serializervalueproof`
also passed under the wrapper:
`serializer-value-proof: exact struct by-value callback; source isolation; nested identity PASS`.
It constructs the canonical identity-backed SerializerInfo, returns it through
`NegotiatedSerializer.SupportedMediaTypes`, verifies separate top-level value
storage, and verifies the nested StreamSerializer pointer remains shared.

`ForeignMap` exposes `mapIsNil`, `mapEmpty`, `mapLookup`, `mapEntries`, `mapWith` and `mapWithout`. It does not convert pointer keys to equal-looking values or normalize NaNs. Pointer-key creation is effectful; nil-pointer keys and pointer-key identity comparison are explicit. Iteration order is Go map order, not a promised sorted order.

The `mode` of every type is in `deps/coverage.json`:

- Kubernetes API models, metadata/schema value models, `RawExtension`, `Unknown`, `VersionedObjects`, duration/scalar/container values, `time.Time`, structured-merge-diff schema models and **wait.Backoff** are snapshots.
- `runtime.Object` and related object/metadata/kind interfaces are data views, not HTTP-style identity services. Read-only access retains its view; a mutating interface operation returns a copied, updated snapshot and cannot mutate an older view.
- Backoff has five fields: Duration, Factor, Jitter, Steps, Cap. Pointer `Step` is a mutation of a returned copy, not a reason to turn configuration into shared identity.
- `k8s.io/apimachinery/pkg/version.Info` is an explicitly approved value snapshot; its thirteen string fields carry no mutable identity or resource lifecycle.
- Other state-bearing types, including transports, clients, readers/writers, schemes, codec services and synchronization objects, are identity capabilities. Allocation, field observation/mutation and methods are IO. Unknown dynamic named types are not copied speculatively.
- `schema.Map` has private lazy lookup caches. Snapshot updates rebuild those caches rather than copy `sync.Once` state.

A snapshot field getter reads the actual field directly; it does **not** serialize or deep-copy the whole object. `podGetObjectMeta`/`objectMetaGetName` illustrate the naming rule. `podWithObjectMeta` copies only its containing value; descendants remain immutable. Identity field operations have explicit `Get...Identity`/`Set...Identity` names and IO types.

An identity setter follows pointers only to reach the owning/intermediate
containers. It does not dereference the final field slot: nullable pointer fields
such as `http.Request.URL` support None-to-Some, replacement and clearing back to
None. Assigning an identity preserves the existing pointee rather than copying it.
Both owning pointers from constructors and addressable views produced by
`From(pointer)`/external Parse results can be mutated in place; unaddressable
values remain rejected with `hostdeps: identity field setter requires pointer for ID`.
Nil intermediate containers and private/inaccessible fields remain rejected with
`hostdeps: inaccessible identity field`; the setter does not allocate missing
intermediate containers.

The focused Go regression
`go test ./host/deps -run '^TestIdentityField' -count=1` passed, covering
pointer slots, parsed addressable views, intermediate pointers and private-field
rejection. The native
`go run ./cmd/go4t run ports/client-go/tools/binddeps/identityproof` also passed:
`identity-field-proof: nullable URL set/replace/clear; shared aliases; parsed pointer ingress PASS`.
Both ran under the serialization wrapper. The native proof uses the public
HTTP/URL bindings, including `url.callParse` and `http.callNewRequest`; no network
request or TLS bypass is involved.

Generic external handles have real witnessed operations: `zero<Type>Like[A](prototype ...)`, typed methods, and callable factories. A witness retains the actual existing Go instantiation. The generator does not pretend the erased runtime can invent arbitrary new named Go generic instantiations from only a native type variable.

## Calls, callbacks and mutations

All external function and method calls are IO. They preserve **every** Go result, including partial values accompanying an error. A trailing error is a canonical `Option[support.Error]`, not a flattened message and not an automatic exception.

Tuple rule:

- zero values: `Unit`;
- one value: the value;
- multiple values: right-nested pairs, `(A,(B,C))`.

Parameters capable of mutating caller-visible snapshot state are returned in original parameter order. `coverage.calls[id].updated_parameters` records both original Go and native parameter indices/names. Scalar/string/function arguments are not redundantly returned.

- Function with updates: `IO[(updatedInputs, results)]`.
- Snapshot method: `IO[(updatedSelf, results)]`, or `IO[(updatedSelf,(updatedInputs,results))]`.
- Identity method: `IO[results]`, or `IO[(updatedInputs,results)]`.

A single copy session preserves shared snapshot references across parameters and recursive pointer graphs. It does not clone transports/locks/codec services. Native callbacks follow the same update rule; updates are applied back to Go locations that callbacks can actually mutate. They cannot replace a caller's pointer variable or slice header by value.

`newDecoder` and other externally implementable interface factories are real Go proxy implementations around typed native callbacks. Sealed interfaces and protobuf's private implementation markers cannot be implemented outside their Go owner; this limitation is explicit in coverage, never a fake proxy. Their existing host implementations can still be consumed through the canonical capability and typed methods.

Data-interface factories without the standard `runtime.Object.DeepCopyObject` contract require an explicit trailing `copy func() IO[Option[I]]` callback. For example, `schema.newObjectKind(getKind, setKind, copy)` uses that callback before a native mutating method invocation. Registered data models use the snapshot copier; unknown runtime.Objects use their external DeepCopyObject contract. Other unknown data-interface receivers without a lawful copy provider fail with `hostdeps: data interface receiver has no snapshot copy contract`. Nil copies and retained input identities are rejected. These extension requirements are explicit in `coverage.data_interface_copy_contracts`, not advertised as generic lock-copy support.

## Codecs, objects and errors

`deps/codec` supplies `DecodeOutcome{object, gvk, error}`. Decode allocates/copies its destination before external decoding and preserves object, GVK and original error independently, including strict-decoding partial results.

`codec.standardParameterCodec()` returns the canonical external metav1 parameter codec as an IO identity capability. `objects.listOptionsToObject()` uses the same zero-copy typed object descriptor for ListOptions; consumers retain the canonical runtime.Object and ParameterCodec identities used by the REST boundary.

`deps/objects` supplies external-only adapters for:

- object/RawExtension deep copy;
- original-object `reflect.DeepEqual` observation;
- exact RawExtension conversion functions (including nil `*Object` versus pointer-to-nil `Object`, and the prior output that the upstream conversion may inspect);
- metadata and list access/extraction, list construction, narrow resource-version capability and GVK;
- exact status conversion/error construction and list-option conversion/validation.

`objectResourceVersionValue` requires only `GetResourceVersion() string`; it intentionally does not require the complete metav1.Object interface. `objectMetadata` uses the external Accessor and AsPartialObjectMetadata. `objectListItems` preserves nil slices and nil interface elements. Its allocation flag selects the external ExtractList versus ExtractListWithAlloc implementation. `validateListOptions` exposes conversion error separately from validation success; its boolean is the external **isWatchListFeatureEnabled**, not the AllowWatchBookmarks field. Watch-list eligibility, flags and retry decisions remain native policy.

`objectSetResourceVersion` deep-copies the external object, applies the external metadata accessor's setter, and returns the updated object plus any original accessor error. It never mutates the input snapshot.

`support.Error` retains the original Go error. Error construction/wrapping, errors.Is and API/status predicates do not stringify the error chain. `errorWrap(prefix,cause)` uses exactly `fmt.Errorf("%s: %w", prefix, cause)`. `raiseError[A]` returns that same original error from IO. `objects.objectGoString` formats the original Go value with `%#v` in IO, because user implementations can provide formatter callbacks.

`support.errorTagged(domain,key,message)` constructs an immutable comparable
canonical error. `errors.Is` matches independent constructions with exactly the
same domain, key and message, including through wrapping. Equal-message ordinary
errors or tagged errors from different domains/keys do not match. Native policy
owns all three strings; the Go primitive has no domain-specific constants,
mutable intern cache, or message-only matcher.

### Go host error boundaries

Domain Go adapters use these helpers; the native `deps/support.Error` API is unchanged:

```go
hostdeps.ErrorFromGo(err)       // bare canonical Error; err must not be nil
hostdeps.ErrorOptionFromGo(err) // canonical None or exactly Some(Error)
hostdeps.ErrorToGo(value)       // bare canonical Error -> original Go error
```

`ErrorFromGo(nil)` panics with `hostdeps: cannot box nil error`. A typed-nil error
interface is not absent: its dynamic type and pointer are retained, and the
optional helper returns Some. Pointer/value errors, wrapping chains and APIStatus
retain their original Go identity and `errors.Is`/`errors.As` behavior.

`ErrorToGo` is deliberately strict: it does not accept None, Some, nested Some,
strings or arbitrary values. An adapter receiving `Option[Error]` must first
unwrap that **one declared Option layer** with `rt.OptionValue` and call
`ErrorToGo` only when present. Do not construct error values using
`From(reflect.ValueOf(&err).Elem())` plus guessed wrapping: `From` already
adds Option for Go interfaces, which causes nested Some or Option-as-Error bugs.
The helpers reuse the canonical error storage directly; boxing a bare error
does not allocate Some just to unwrap it.

The focused Go proof is
`go test ./host/deps -run '^TestErrorBridge' -count=1` from
`ports/client-go`, under the serialization wrapper. It covers pointer and value
errors, typed nil, wrapped `os.ErrNotExist`, wrapped APIStatus, and rejection of
malformed Option/text shapes. It passed after installing the same scoped runtime
Option-shape fixture pattern used by existing runtime tests.

`support.errorIsAlreadyExists(Error) Bool` preserves wrapped API error predicates;
`support.errorNotFound(group,resource,name) Error` constructs the actual API
NotFound error. The existing `codec.newNotFound(GroupResource,String)` retains
its distinct structured-resource signature.

`objects.shallowCopy[A](ObjectType[A],A) IO[A]` implements Go value-copy semantics:
a fresh outer object with immutable descendants shared, not a DeepCopy. Exact
descriptor mismatch fails with `hostdeps: object descriptor type mismatch`.
`objects.labelSelectorAsSelector(Option[meta.LabelSelector])` returns
`IO[(Option[labels.Selector],Option[Error])]`, preserving nil input as Nothing,
empty input as Everything, and invalid input as nil selector plus original error.
It does not change the input's nil maps or slices.

`codec.encodeJSON(Object)` and `codec.encodeCBOR(Object)` return
`IO[(Option[String],Option[Error])]`. The String is raw bytes, not decoded text.
They call the pinned apimachinery JSON and direct CBOR marshalers on the actual
object, honoring custom marshalers and returning both original output presence
and error. No JSON/Float intermediate is introduced.

`codec.decodeJSON[A](ObjectType[A],String)` returns
`IO[(Option[A],Option[Error])]` using a fresh exact-typed destination and
apimachinery's case-sensitive, integer-preserving JSON unmarshal. It honors
custom unmarshalers (including Unstructured), retaining the partial destination
even on error. JSON `null` retains the fresh zero destination, matching
unmarshal into a nonnil concrete pointer; it does not invent an absent object.
`objects.objectTypeName(Option[Object]) IO[String]` uses exact Go `%T`:
nil is `<nil>`, while a typed-nil Pod pointer is `*v1.Pod`.
`support.errorIsNotRegistered(Error) Bool` delegates exactly to
`runtime.IsNotRegisteredError`; unlike the API-status predicates, that upstream
predicate does **not** unwrap a wrapped registration error.

The too-large-resource-version retry predicate is **not** implemented in Go. Consumers compose the external timeout and status-cause primitives in go4t. Likewise, random-bound selection is native policy; `support.randomInt` is only the generic bounded random effect.

## Coverage and exclusions

The output separates:

1. the complete canonical data/type graph;
2. the frozen required external-operation set and manual adapters;
3. the additionally available method closure, **not claimed to be additional client-go scope**;
4. explicit source/platform/constructibility exclusions.

At the body-root generation, coverage records 2,545 canonical named/synthetic type entries, 7,491 field entries, 19,979 callable adapters and the unchanged 112 frozen required operations, plus the manual declarations inventoried from the native files. Counts are inventories, not percentages of client-go implementation.

Private/internal constructors and sealed/protobuf interface proxy shapes remain explicitly excluded. Generic package functions require an actual instantiation witness; no arbitrary erased generic instantiation is invented. The original-body generic references not emitted as package calls are `sets.KeySet`, `sets.List`, `sets.New`, `ptr.Deref`, `ptr.To`, `runtime.AddCleanup`, `slices.All`, `slices.Sort`, and `weak.Make`. These are recorded exclusions, not a claim that consumers have implemented their equivalents. Transitive cgo pseudo-types remain unresolved (`C.GO_EC_KEY`, `C.GO_EC_POINT`, `C.struct_rusage`, `C.struct_timeval`); no private C representation is fabricated. `unsafe.ArbitraryType`/`IntegerType` documentation pseudo-types are separately identified as not real exported runtime types.

`initial_extraction_counts`/the original evidence count block describes the earlier signature-only inventory. The expanded definitions additionally include public types required for dynamic status/metadata decoding; `deps/coverage.json` is the binding-output count authority.

## Assigned proof and observed result

From the go4t worktree root, the only executed acceptance command was the assigned narrow proof:

```sh
SERIALIZE_VM_TESTS_TARGET=local \
  /home/vagrant/.codex/skills/serialize-vm-tests/scripts/serialize-vm-tests \
  --class short -- go run ./cmd/go4t run ports/client-go/tools/binddeps/proof
```

It exited 0 with:

```text
dependency-proof: typed Pod/PodList/ListOptions; presence; int64/uint64; recursive schema; canonical identity; decode object+GVK+structured-error PASS
```

The native proof exercises typed objects/lists/options; absent versus false; nil versus allocated-empty collections; signed integers above 2^53; full uint64 words and invalid-word rejection; recursive and actually cyclic schema values; metadata passed through two independently importing modules; shared-pointer mutation through the external Scheme.Convert path; a native Decoder callback implementation; real external strict JSON decoding with simultaneous object/GVK/error; pointer-key map equality; RawExtension pointer-presence conversions; original-object deep-copy equality; wrapped-error identity; metadata RV and typed Status projection. The descriptor/isolation extension additionally proves cached descriptor identity, zero-copy upcast/cast storage, wrong-type and cross-version Scale rejection, nil/typed-nil rejection, ObjectKind and metadata setter isolation through upcast views, and a custom ObjectKind factory's explicit copy callback.

The body-root/additive proof passed with the same command and output. It adds
outer-object shallow-copy isolation; AlreadyExists/NotFound distinction and wrapped
API errors; pure Unix nanosecond normalization; nil/empty/invalid label selectors
without nil-map mutation; exact nil/typed-nil `%T`; the upstream non-unwrapping
registration predicate; JSON and CBOR round-trips of 9007199254740993 through an
actual Unstructured; custom JSON marshaling; and partial JSON destination plus
original error. Before that pass, the probe exposed and fixed an invalid generic
interface factory (now explicitly excluded, with typed witnessed methods retained),
proof-fixture pointer/Option mismatch, and missing Kind on the CBOR fixture.
Runtime's owning worker normalized the module requirements after the probe
reported the required Go module update; the dependency owner did not edit manifests.

Generic interface factories without a concrete Go instantiation are excluded,
including `cached.Value[T]`. Their existing instances retain generic typed method
bindings; the generator does not emit an invalid uninstantiated Go proxy.

The approved Info correction was separately verified with the same wrapper using
`go run ./cmd/go4t run ports/client-go/tools/binddeps/versionproof`. It exited 0:
`version-binding-proof: pure getWith; thirteen fields; immutable snapshots; value methods PASS`.
The existing dependency proof was then rerun and also exited 0. The narrow version
proof imports the real `pkg/version.getWith`, observes all thirteen fields,
checks original-snapshot preservation and both value methods. No clientcmd
regression or broader core checks were rerun by the dependency owner.

No repository-wide suite, formatter, linter, live-cluster command, commit or push was run. The proof is not a claim that all optional transitive operations were exercised. Domain owners and the primary retain their own integration acceptance. Proof-generated `_gen` output is disposable; the go4t proof and its small external fixtures are retained under this directory.
