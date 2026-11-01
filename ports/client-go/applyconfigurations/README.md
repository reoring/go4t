# Native apply configurations

Source authority is client-go v0.37.2, revision `72c52dd0cc3b9099b36af11290b0304045dd1ada`. Kubernetes-derived declarations and schema retain their Apache-2.0 notices. `coverage.json` records source hashes, every generated record and field, every source method/constructor, extraction entrypoints, and links to the shared resource descriptor.

Regenerate from the repository root:

```sh
python3 tools/portgen/ApplyConfigurations/generate.py
```

The generator consumes `tools/portgen/resources.json`, checks every descriptor apply type exists, and reads the full pinned apply configuration sources. It emits 952 native records containing 3,603 fields, typed builders/codecs and regression cases. All original `ForKind` branches become a closed `Configuration` sum; unknown GVKs return `None`. No resource subset is hand-maintained. The shipped schema is preserved byte-for-byte in `host/schema.yaml` (430,734 bytes).

Use `python3 tools/portgen/ApplyConfigurations/generate.py --metadata-only` to refresh descriptor provenance and export coverage without rewriting integrated source files. Explicit `exports` rows cover the handwritten parser, GVK/converter utilities, unstructured-extractor capability and CSA entrypoints. Each row carries the exact inventory `[source, receiver, symbol, kind]` key; extraction methods use receiver `*extractor`, not the public capability type name.

For isolated regeneration, pass `--output-root /path/to/empty-shadow-module`. The option redirects all generated outputs while retaining the repository's current descriptor, dependency bindings and inventory as read-only inputs. The isolated comparison covered all 110 generated files: 109 were byte-identical before the approved export-metadata correction; all 110 matched afterward. The 430,734-byte schema was unchanged. Temporary shadow output was removed after comparison.

## Public API

Upstream package boundaries are retained. For example, `core/v1` supplies:

- `PodApplyConfiguration`, an alias to a uniquely named immutable native record;
- `pod(name, namespace)`, `podSpec()`, `container()` and corresponding constructors;
- `podWithSpec(pod, Some(spec))`, `podSpecWithContainers(spec, [Some(container)])`;
- `podGetName(pod)`, `podGetNamespace`, `podGetKind`, `podGetAPIVersion`;
- `podMarshalJSON(pod)`, `podMarshalCBOR(pod)` returning `IO[Outcome[String]]` (String carries exact bytes, matching Rest);
- `podUnmarshalJSON(bytes)` returning `IO[Outcome[Option[PodApplyConfiguration]]]`;
- `podMarshal()` returning the shared `util/apply.Marshal[PodApplyConfiguration]`;
- `extractPodFrom(object, manager, subresource)` plus every upstream named wrapper, including `extractPod`, `extractPodStatus`, `extractPodResize`, and `extractPodEphemeralcontainers`.

Type names receive a package-derived prefix internally to prevent collisions in foreign-record registration; source-facing aliases remain unchanged. Functions use the lowercased first character of the upstream constructor name as their receiver prefix. Fields similarly preserve upstream spelling with the first character lowercased (`aPIVersion`).

Builders are native, immutable transformations. Pointer fields/arguments use `Option`, variadic arguments use `List`, and slices/maps preserve nil separately from empty. Repeated scalar calls overwrite, list calls append, and map calls merge with last-write wins. Nil pointer list elements raise the exact upstream `nil value passed to With<Field>` panic. Getters read immutable values without mutating metadata. Embedded metadata allocation happens in the native metadata builders. Only fields shipped upstream are present; omitted metadata fields are not reintroduced.

Native keywords are escaped with a trailing underscore (`Type` becomes `type_`); this affects record field access only, not JSON names or `WithType` builder names.

Maps with string keys use native `Map`. Maps keyed by external named Go types use canonical `deps/support.ForeignMap`; `cNew<Field>()` creates an empty typed map for such fields, and the shared dependency map operations populate it. Byte slices retain exact bytes rather than integer-to-Float conversion.

## External boundaries

`host/types.go` contains only generated field/tag mirrors. `host/codec.go` converts explicitly by generated record identity and field position, preserving pointer/slice/map states and canonical dependency values. JSON uses `encoding/json`; CBOR uses apimachinery's direct CBOR codec. Neither encoder passes integers through Float or a lossy dynamic JSON tree.

Extraction invokes external `managedfields.ExtractInto` against the complete shipped typed parser, then converts the extracted mirror back to the actual native record. Native wrappers restore name, namespace (where present upstream), kind and apiVersion through typed dependency getters. Field ownership, associative lists, manager choice, subresources and conflicts are decided by managedfields/structured-merge-diff, never by matching JSON keys.

`newTypeConverter(runtime.Scheme) IO[managedfields.TypeConverter]` uses the actual external scheme converter and shipped parser. `internal.parser()` exposes that parser. `meta/v1.newUnstructuredExtractor` consumes the native Discovery capability and maintains its parser cache in native code, with the original one-minute expiration rule and identity restoration. `newUnstructuredExtractorWithClock` permits deterministic cache tests.

## Prepared verification

No product build, formatter, linter or test suite was run by this owner. Generation completed successfully. Prepared tests include one typed JSON roundtrip plus CBOR encoding case per configuration, one exact GVK-variant test per registry branch, and focused cases for nil/empty/zero, append/merge/overwrite, nil-pointer errors, integers beyond Float precision, managed-field manager selection, named subresources and ownership exclusion. The main integrator must run native checks and these tests after the shared graph lands.
