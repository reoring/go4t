# Canonical object references

Native implementation of client-go v0.37.2 `tools/reference/ref.go` (Apache-2.0).

- `GetReference` maps to `getReference(Option[runtime.Scheme], Option[runtime.Object]) IO[Outcome[Option[core.ObjectReference]]]`.
- `GetPartialReference` maps to `getPartialReference` with an additional fieldPath String.
- `ErrNilObject` maps to `errNilObject()` with exact message `can't reference a nil object`. Independent calls produce comparable canonical errors with native-owned domain `k8s.io/client-go/tools/reference` and key `ErrNilObject`; `support.errorIs` matches nil-reference and nil-partial-reference errors, including wrapping. A fresh equal-message error, another domain or another key does not match.

Existing references pass through unchanged. Object metadata is preferred; objects exposing only common list metadata receive kind/apiVersion/resourceVersion without object-name fields. A completely empty GVK falls back to the first Scheme ObjectKinds result; absent Scheme returns `scheme is required to look up gvk`. Partially populated GVK does not trigger fallback. Partial-reference updates preserve all other fields through canonical immutable setters.

`reference_test.gfx` covers nil input, absent scheme, populated GVK, namespace and field path, direct-reference passthrough, and sentinel identity. `regression/main.gfx` is an intentional permanent external public-API regression entrypoint: independent construction, both nil paths, wrapping, unchanged text, and equal-text/domain/key negative controls. Before the correction, `errorNew` constructed independent pointer errors, so the independent-call and nil-result identity assertions would fail; this before-state is source-derived, not an observed before-run result. The generic CommonAccessor conversion is in host/events; reference-selection policy remains native.

The released focused validation ran through the serialization wrapper:

- `go run ./cmd/gofx test ports/client-go/tools/reference`: five passed, zero failed, including `testNilReferenceSentinelIdentityTrace`.
- `go run ./cmd/gofx run ports/client-go/tools/reference/regression`: exit 0; all public identity, wrapping, unchanged-message and nonmatch assertions passed.

Both commands emitted existing warnings from flowcontrol and internal events;
neither emitted a reference failure. Run these commands from the repository root
through the repository's serialization wrapper when repeating this regression.
