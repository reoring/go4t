# Native feature gates

Ports the pinned client-go v0.37.2 feature registry and testing helpers. Create one `FeatureRegistry` with `newFeatureRegistry(processEnvironment())` and pass it to consumers; there is deliberately no hidden process-global registry. `featureGates(registry)` returns callback capabilities. `replaceFeatureGates` atomically replaces that capability and returns whether the default registry had already been consulted, also emitting the upstream initialization-order warning.

`newEnvVarFeatureGates(name, specs, environment)` makes the call-site name and environment injectable. `enabled` reads all known `KUBE_FEATURE_` variables once, including when Set overrides the requested feature. Override precedence is Set/SetForTesting, valid unlocked environment, then default. Go ParseBool's exact twelve spellings are accepted. Invalid and locked environment values are reported and ignored. Unknown reads fail in IO; unknown/locked writes return the original error text in `Result`. `setForTesting` bypasses locks. Shared state uses support/sync, not Go feature-policy code.

All ten upstream feature constants are typed lowerCamel zero-argument functions with unchanged string values: for example `watchListClient()`, `atomicFIFO()`, and `clientsAllowCBOR()`. Maturity values are `alpha()`, `beta()`, `ga()`, and `deprecated()`. `defaultVersionedKubernetesFeatureGates()` includes every pinned version history. `unversionedFeatureGates` selects the final entry and removes its Version, not a sorted/reinterpreted version.

`testing.newOverrides` and `setFeatureDuringTest` use a Test capability (name, cleanup registration, error reporting); callers must run cleanup actions in LIFO order. Sibling tests cannot concurrently override the same feature; same test and descendants may. Cleanup restores the captured value using the captured registry setter.

Prepared behavioral program: `features/regression`. It has not been built or run by this worker. Primary should run the gofx checker and program after canonical dependencies stabilize, plus `testing` nested/sibling cleanup and concurrent Enabled/Set coverage.
