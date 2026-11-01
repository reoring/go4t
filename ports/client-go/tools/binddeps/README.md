# Canonical external dependency bindings

This tooling binds **external** Go dependencies for the native client-go port. It never imports or implements `k8s.io/client-go`. Client-go policy remains in gofx. Go code here performs dependency conversion, typed field access, external calls, callbacks, and generic effects only.

## Inputs and regeneration

From `ports/client-go`:

```sh
python3 tools/binddeps/generate.py
```

Inputs and the generated adapter ledger are committed, not session artifacts:

- `contract.json`: the pinned, body-free external AST graph, exact module download records, source hashes and original authority. It includes the original client-go signature closure plus all exported types in reached API/apimachinery packages and `api/errors.APIStatus`, needed for dynamically decoded/status objects.
- `target.json`: Go build-constraint selection for linux/amd64 with Go 1.26.1. Other platform variants remain in the input graph; they are not silently selected by declaration order.
- `manual-adapters.json`: generated literal native signatures from the hand-written `deps/support`, `deps/codec` and `deps/objects` declarations. It is refreshed by the generator, not independently edited.
- `extract.go`: the reproducible Go AST extractor. It resolves imported package spellings from package clauses, not regular-expression guesses. `modules.json` records exact source roots, versions and Go module checksums. For an authorized input refresh, run `go run tools/binddeps/extract.go PINNED_CLIENT_GO_ROOT tools/binddeps/modules.json`; this emits the body-free AST graph. Updating the pinned input/authority is separate from ordinary binding regeneration.
- `select.go`: uses `go/build.MatchFile` rather than an ad hoc build-tag parser. `go run tools/binddeps/select.go tools/binddeps/contract.json` emits a target selection. Refreshing target metadata requires the exact downloaded source files recorded by the graph.

Run compiled extraction/selection and proofs under the repository's serialize-vm-tests wrapper. None of these commands is a test suite or formatter. Generator outputs are atomically replaced per file. Stale files bearing this generator's marker are removed; hand-written files are not deleted.

The API and apimachinery inputs are **v0.37.2**. API tag revision: `1bc0c529309723075aed339d50f16d4b311d908d`. Apimachinery tag revision: `97adf147c7378229d45db0fc7511af84d0c3c092`. Client-go inventory authority: `72c52dd0cc3b9099b36af11290b0304045dd1ada`. Each generated file identifies the source graph SHA-256. Kubernetes-derived declarations retain Apache-2.0 attribution.

## One owner for each type

`deps/types/types.gfx` is the **only nominal declaration owner**. Its deterministic names contain the actual Go name and a qualified-identity hash. `deps/<Go import path>/values.gfx` exports readable aliases and operations, for example:

```gofx
import core "github.com/reoring/gofx/ports/client-go/deps/k8s.io/api/core/v1"
import meta "github.com/reoring/gofx/ports/client-go/deps/k8s.io/apimachinery/pkg/apis/meta/v1"

func namedPod(name String) core.Pod {
 let metadata = meta.objectMetaWithName(meta.newObjectMeta(), name)
 core.podWithObjectMeta(core.newPod(), metadata)
}
```

Those aliases reference the same canonical type when imported by multiple consumers. A Go `type X Y` remains distinct; a real Go `type X = Y` shares its target. The generator does not decide aliases from explanatory comments. Anonymous interface/struct shapes receive structural canonical adapters; they are not mislabeled as named Go types. Prelude collisions use `Go` suffixes, such as `MapGo` and `ListGo`.

No client-go object is replaced by universal JSON, strings or dyn. The `support.Any` handle appears only for actual Go `any`/empty-interface positions. Its presence in an external type does not erase the enclosing type.

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

`ForeignMap` exposes `mapIsNil`, `mapEmpty`, `mapLookup`, `mapEntries`, `mapWith` and `mapWithout`. It does not convert pointer keys to equal-looking values or normalize NaNs. Pointer-key creation is effectful; nil-pointer keys and pointer-key identity comparison are explicit. Iteration order is Go map order, not a promised sorted order.

The `mode` of every type is in `deps/coverage.json`:

- Kubernetes API models, metadata/schema value models, `RawExtension`, `Unknown`, `VersionedObjects`, duration/scalar/container values, `time.Time`, structured-merge-diff schema models and **wait.Backoff** are snapshots.
- Backoff has five fields: Duration, Factor, Jitter, Steps, Cap. Pointer `Step` is a mutation of a returned copy, not a reason to turn configuration into shared identity.
- Other state-bearing types, including transports, clients, readers/writers, schemes, codec services and synchronization objects, are identity capabilities. Allocation, field observation/mutation and methods are IO. Unknown dynamic named types are not copied speculatively.
- `schema.Map` has private lazy lookup caches. Snapshot updates rebuild those caches rather than copy `sync.Once` state.

A snapshot field getter reads the actual field directly; it does **not** serialize or deep-copy the whole object. `podGetObjectMeta`/`objectMetaGetName` illustrate the naming rule. `podWithObjectMeta` copies only its containing value; descendants remain immutable. Identity field operations have explicit `Get...Identity`/`Set...Identity` names and IO types.

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

## Codecs, objects and errors

`deps/codec` supplies `DecodeOutcome{object, gvk, error}`. Decode allocates/copies its destination before external decoding and preserves object, GVK and original error independently, including strict-decoding partial results.

`codec.standardParameterCodec()` returns the canonical external metav1 parameter codec as an IO identity capability. `objects.listOptionsToObject()` copies and upcasts typed ListOptions for parameter encoding; consumers retain the same runtime.Object and ParameterCodec identities used by the REST boundary.

`deps/objects` supplies external-only adapters for:

- object/RawExtension deep copy;
- original-object `reflect.DeepEqual` observation;
- exact RawExtension conversion functions (including nil `*Object` versus pointer-to-nil `Object`, and the prior output that the upstream conversion may inspect);
- metadata and list access/extraction, list construction, narrow resource-version capability and GVK;
- exact status conversion/error construction and list-option conversion/validation.

`objectResourceVersionValue` requires only `GetResourceVersion() string`; it intentionally does not require the complete metav1.Object interface. `objectMetadata` uses the external Accessor and AsPartialObjectMetadata. `objectListItems` preserves nil slices and nil interface elements. Its allocation flag selects the external ExtractList versus ExtractListWithAlloc implementation. `validateListOptions` exposes conversion error separately from validation success; its boolean is the external **isWatchListFeatureEnabled**, not the AllowWatchBookmarks field. Watch-list eligibility, flags and retry decisions remain native policy.

`objectSetResourceVersion` deep-copies the external object, applies the external metadata accessor's setter, and returns the updated object plus any original accessor error. It never mutates the input snapshot.

`support.Error` retains the original Go error. Error construction/wrapping, errors.Is and API/status predicates do not stringify the error chain. `errorWrap(prefix,cause)` uses exactly `fmt.Errorf("%s: %w", prefix, cause)`. `raiseError[A]` returns that same original error from IO. `objects.objectGoString` formats the original Go value with `%#v` in IO, because user implementations can provide formatter callbacks.

The too-large-resource-version retry predicate is **not** implemented in Go. Consumers compose the external timeout and status-cause primitives in gofx. Likewise, random-bound selection is native policy; `support.randomInt` is only the generic bounded random effect.

## Coverage and exclusions

The output separates:

1. the complete canonical data/type graph;
2. the frozen required external-operation set and manual adapters;
3. the additionally available method closure, **not claimed to be additional client-go scope**;
4. explicit source/platform/constructibility exclusions.

At the delivered generation, coverage records 2,403 canonical named/synthetic type entries, 7,138 field entries, 19,174 callable adapters and 112 operations in the frozen required-operation set, plus the manual declarations inventoried from the current native files. Counts are inventories, not percentages of client-go implementation.

The remaining exclusions are 213 private/internal or otherwise externally unconstructible zero constructors, 25 protobuf private-marker proxy shapes, four sealed interface proxy shapes and one private protobuf method-table proxy. There are no omitted public Kubernetes model fields hidden behind those counts. These exclusions do not claim that every transitive Go-private type needs a new constructor. Existing external instances and public aliases retain their real capabilities. Go `unsafe.ArbitraryType`/`IntegerType` documentation pseudo-types are separately identified as not real exported runtime types.

`initial_extraction_counts`/the original evidence count block describes the earlier signature-only inventory. The expanded definitions additionally include public types required for dynamic status/metadata decoding; `deps/coverage.json` is the binding-output count authority.

## Assigned proof and observed result

From the gofx worktree root, the only executed acceptance command was the assigned narrow proof:

```sh
SERIALIZE_VM_TESTS_TARGET=local \
  /home/vagrant/.codex/skills/serialize-vm-tests/scripts/serialize-vm-tests \
  --class short -- go run ./cmd/gofx run ports/client-go/tools/binddeps/proof
```

It exited 0 with:

```text
dependency-proof: typed Pod/PodList/ListOptions; presence; int64/uint64; recursive schema; canonical identity; decode object+GVK+structured-error PASS
```

The native proof exercises typed objects/lists/options; absent versus false; nil versus allocated-empty collections; signed integers above 2^53; full uint64 words and invalid-word rejection; recursive and actually cyclic schema values; metadata passed through two independently importing modules; shared-pointer mutation through the external Scheme.Convert path; a native Decoder callback implementation; real external strict JSON decoding with simultaneous object/GVK/error; pointer-key map equality; RawExtension pointer-presence conversions; original-object deep-copy equality; wrapped-error identity; metadata RV and typed Status projection.

No repository-wide suite, formatter, linter, live-cluster command, commit or push was run. The proof is not a claim that all optional transitive operations were exercised. Domain owners and the primary retain their own integration acceptance. Proof-generated `_gen` output is disposable; the gofx proof and its small external fixtures are retained under this directory.
