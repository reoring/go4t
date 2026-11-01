# OpenAPI v3 root

Native port of `openapi3/root.go`, Kubernetes client-go v0.37.2 at
`72c52dd0cc3b9099b36af11290b0304045dd1ada` (Apache-2.0).

- `Root`, `NewRoot` map to `Root`, `newRoot(openapi.Client)`.
- `GroupVersions` maps to `groupVersions(Root)`: fresh paths lookup, ignore
  unparsable entries, sort by canonical group/version string.
- `GVSpec` maps to `gvSpec(Root,canonical GroupVersion)` and returns the full
  canonical kube-openapi `spec3.OpenAPI`, including partial decode results.
- `GVSpecAsMap` maps to `gvSpecAsMap`, preserving the upstream explicitly
  unstructured representation as canonical `ForeignMap`/`Any`, not using that
  representation as a replacement for the typed schema API.
- `retrieveGVBytes`, `gvToAPIPath`, `pathToGroupVersion` retain the upstream
  helper behavior, including permissive trailing path segments.
- `GroupVersionNotFoundError` maps to `groupVersionNotFoundError` and
  `isGroupVersionNotFoundError`. The underlying typed Go error preserves the
  canonical GroupVersion and exact upstream message.

Every operation calls the provided client's paths capability. There is no
independent root cache or hidden global; a cached client can be passed
explicitly. Tests consume exact shipped schemas from `openapi/openapitest`.
No verification commands have been run in this slice.
