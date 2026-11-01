# Native typed listers

This directory ports the complete `listers` tree from Kubernetes client-go
v0.37.2, revision `72c52dd0cc3b9099b36af11290b0304045dd1ada`.
The generator produces 150 typed listers and their behavioral fixtures across
all 51 versioned packages. `coverage.json` records all 217 upstream Go sources,
their hashes, exported declarations, and each resource's native output.

## Construction and read-only identity

`newPodLister(indexer)` (and each corresponding resource constructor) accepts
`cache.Indexer[runtime.Object]`. It uses the canonical per-type `ObjectType`
descriptor, never dynamic values or structural lookalike casts. A wrong cached
concrete type raises `listers: cached object has unexpected concrete type`;
it is not silently skipped or reported as absent.

`newPodListerTyped(indexer, access)` accepts `cache.Indexer[core.Pod]` and
`cache.ObjectAccess[core.Pod]` without erasing its type. Both constructors expose
the same capability record. Its `list(selector)` returns
`IO[Outcome[List[core.Pod]]]`; `pods(namespace)` returns the namespace record
with `list(selector)` and `get(name) IO[Outcome[Option[core.Pod]]]`. Cluster
resources expose `get` directly. Results retain the original cache identity
and are read-only. `ResourceIndexer[A]`, `new`, `newNamespaced`, `list`, and
`get` are the native equivalents of the upstream generic helpers.

The empty namespace lists all namespaces, and its lookup uses a bare name.
Other lookups use `namespace/name`. Store errors are retained unchanged.
Absent keys return canonical structured Kubernetes NotFound errors using the
**singular resource from the upstream lister**, not the REST resource plural.
Listing, namespace-index fallback, label observation, and matching reuse the
Cache owner; there is no second cache implementation here.

## Expansions

Every upstream expansion interface has a native capability record (or a
nullary marker for an empty interface). Handwritten expansion methods are
included in the resource lister record and exposed as lowerCamel free functions.
These enumerate all potential owners, not just the controller-reference owner:

- ReplicationController, ReplicaSet, StatefulSet, and DaemonSet reject empty
  subject labels and empty owner selectors. Nil selectors match nothing.
- DaemonSet history methods preserve their version-specific ControllerRevision
  types, including apps/v1beta1 histories for extensions/v1beta1 DaemonSets.
- Job rejects empty pod labels, but permits an allocated empty selector to
  match. Its returned Jobs are shallow containing-value copies, preserving
  upstream `[]Job` versus the other expansions' read-only pointer semantics.
- policy/v1 PodDisruptionBudget permits an allocated empty selector to match
  every pod, including an unlabelled pod; v1beta1 rejects empty selectors.
- Invalid owner selectors are skipped. Namespace guards, upstream error text,
  and multiple simultaneous matches are retained.

Raw metadata and selector matching are observed through IO. Typed immutable
snapshot getters are used only within the IO observation callback. External
`LabelSelectorAsSelector` conversion and the canonical shallow-copy adapter
are dependencies, not client-go Go implementations.

## Regeneration and verification ownership

From the repository root:

```sh
python3 tools/portgen/Listers/generate.py
```

The generator consumes `tools/portgen/resources.json`, which is owned by
TypedClients, and the pinned upstream sources. The descriptor includes the
lister-only ImageReview resource. It never writes the shared descriptor.
All generated source and test files retain Kubernetes Apache-2.0 attribution.

For isolated reproduction, direct outputs to a separate repository-shaped root:

```sh
python3 tools/portgen/Listers/generate.py --output-root /tmp/clientgo-listers-shadow
```

Inputs remain the current shared descriptor and pinned upstream sources; only
the output root changes. Coverage paths remain repository-relative, so the
shadow files can be compared byte-for-byte without normalizing their contents.
The isolated reproduction proof compared all 301 generated files (150 listers,
150 fixture files, and `coverage.json`) with zero differences. Before/after
hashes confirmed the live lister tree was unchanged, including handwritten
algorithms and fixtures. The temporary shadow tree was removed afterward.

The generator has been executed successfully. No product build, typecheck,
formatter, linter, or test execution was performed in this owner slice.
Supervisor acceptance must execute the native fixture functions after shared
dependency integration. Fixtures cover every raw typed constructor and lookup,
namespace/all-namespace behavior, fallback indexing, structured NotFound,
identity, effectful matching, all handwritten expansions, ambiguous owners,
invalid selectors, nil/empty selectors, and unlabelled subjects.
