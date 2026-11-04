# Approval controller extraction

Standalone Go module `approvalctl`, package `storage`, extracted from
`/workspace/platform/internal/controllers/storage/` at platform commit
`0a9a5de52b47d70f5ad8f5174d1f98efbedef3c3`.

`storage_approval_controller.go` started as a verbatim copy. Observation and
projection now live in `decide/observe.gt`, alongside the pure decision in
`decide/decide.gt`. Both emit into Go package `approvalctl/_decide_gen` (package
name `decide`). The Go shell owns the controller-runtime `client.Client`,
reconciler, status publishing, and operation creation. It calls
`decide.ObserveInputs(r.Client, approval)(ctx)`, then `decide.Decide`.

go4t performs `Get`/`List` through generated controller-runtime bindings,
selects the references, and projects every binding, plan, policy, window,
gate, TTL, timestamp, and operation-conflict predicate. Reads still happen
up front in plan, policy, window, operations order even when an earlier
decision gate will reject the approval. Operation creation and its immutable
approval-binding helpers remain unchanged.

The exported boundary preserves native handles, not copied client records:

```go
func ObserveInputs(c client.Client, approval *unstructured.Unstructured) func(context.Context) (Observed, error)
```

The generated signature names its parameters `p0`/`p1` and imports these
packages as `go0`/`go1`. The shell retains the `Now` test hook; absent that hook,
go4t samples `time.Now` after the reads, in nanoseconds. An IO-boundary failure
publishes Pending / PlanReadFailed with its error message; ordinary API read
failures remain the original `Fetched` and operation-read outcomes.

The shared `Fetched[A]` sum represents `Fetched[Plan]`, `Fetched[Policy]`, and
`Fetched[Window]` in `Observed`. Their Go mirrors are `FetchedPlan`,
`FetchedPolicy`, and `FetchedWindow`, with constructors such as
`FetchedPlanFound{F0: plan}`. Missing policies and windows retain the same
read-error verdicts as before.

`storage_approval_controller_test.go` is still a verbatim copy and its six
tests remain the correctness oracle. Production helper declarations and test
fixtures are copied from the same package, except for the fixture-client
trimming below.

## Generated bindings

Cross-package upcasts are now generated per
[ADR 0009](../../docs/adr/0009-cross-package-upcasts-and-lock-values.md):
`client.unstructuredAsObject` and `client.unstructuredListAsObjectList`
preserve native handles for `clientGet` and `clientList`. go4t constructs
unstructured objects with their GVK and uses `types.NamespacedName` for reads;
namespace-scoped lists use `newListOptions`, `listOptionsSetNamespace`, and
`listOptionsAsListOption`. `io.tryIO` maps successful reads to `Found`,
`io.NotFound` to `Missing`, and other read failures to `ReadError`.

## What moved

Line counts include signatures and bodies, excluding generated code.

| Area | Before | After |
| --- | --- | --- |
| Observation and projection | 123-line Go `observeInputs` | Deleted; 241-line `decide/observe.gt` added |
| API reads | Get/List inside the Go observer | Entirely go4t through generated bindings; 0 handwritten Go lines |
| Projection | Go constructs `Observed`, `Fetched`, gates and time sums | Entirely go4t; only the optional test clock override remains in the shell |
| Unused Go helpers | Terminal-phase/string-slice/map-string helpers, list GVK, default clock method | Removed: 45 lines from `helpers.go` and 7 clock-helper lines |

**Export naming:** helpers use `_projectFetchedPlan`, `_projectTtl`, and
`_projectTimestamp`, not `plan`, `ttl`, or `timestamp`. This avoids first-rune
upper-casing collisions with `Plan`, `Ttl`, and `Timestamp` under SPEC §8.7;
the compiler is unchanged.

## Helpers

`helpers.go` contains:

- `condition`, `setCondition`, `conditionsToUnstructured`, `existingConditions`,
  `nestedInt64`, and transitively required `numericToInt64`.
- `newStorageResizePlanObject`, `newStorageMaintenanceWindowObject`,
  `storageResizePlanResourceGVK`, `storageMaintenanceWindowResourceGVK`.
- `newStorageResizeOperationObject`, `storageResizeOperationResourceGVK`.
- `patchUnstructuredStatus`, `firstStatusString`, `firstNonEmptyNestedString`,
  `newStorageAutopilotPolicyObject`, `storageAutopilotPolicyGVK`.
- `recordStorageWorkflowEvent`, `storageApprovalEvent`, `appendAuditCorrelation`.
- Constants `StorageCapabilityGroup`, `StorageCapabilityVersion`,
  `StorageResizePlanKind`, `StorageResizeOperationKind`,
  `StorageAutopilotPolicyKind`.

`helpers_test.go` contains `newStorageAutopilotPolicy`, `newAutopilotTestClient`,
`hasConditionStatus`, `newStorageResizePlan`, `newOpenStorageMaintenanceWindow`,
and the additionally required `newStorageResizeOperation` from
`storage_workflow_retention_controller_test.go` (discovered by the first vet).

The fixture client omits `withCurrentCephCommandEvidence` and the `corev1`,
`appsv1`, `batchv1`, and `storagev1` scheme registrations: these approval tests
use only unstructured workflow resources and need neither Ceph evidence
injection nor typed resource registrations. Existing maintenance-window and
host-pool registrations are retained. All six `TestApproval*` tests pass.

## Commands

Run inside this directory:

```sh
go generate ./...
go vet ./...
go test ./... -count=1
```

`generate.go` runs `go run ../../cmd/go4t build --emit-go _decide_gen --package
decide decide`. The source directory combines `decide.gt` and `observe.gt` into
one entry module and one exported API. **Run generation before building or
testing:** `_decide_gen/` and its `go4tbind/` packages are ignored, not committed.
Do not edit emitted Go files directly. The previously committed `decide/*.go`
are removed; `decide/` now contains only handwritten go4t.

The leading underscore excludes `_decide_gen/` from Go's `./...` test package
walk; the `*_gen/` rule separately excludes it from Git. Tests still compile
and execute the generated package through the shell import.

The `require go4t v0.0.0` and `replace go4t => ../..` connect the controller and
generated package to the local go4t runtime. This nested module is not added
to the root module or a workspace, keeping root package discovery unchanged
(the same isolation pattern as `examples/deps/app`).

## Behavioural parity

Gate evidence is projected element-by-element as `GateEvidence = Gate | InvalidGate`,
so the first malformed or non-passing element decides exactly as the original
loop did; `gate_order_test.go` passes against both the original Go controller
and this version. Generations cross as `Option[Int]` and are compared as
options (no sentinel); durations and timestamps use `Int` nanoseconds,
the same representation as `time.Duration` / `UnixNano`. Additional native
boundary tests cover read flags and ordering, missing references, mixed
allowed-operation lists, numeric binding formats, nanosecond precision,
cancellation, and preservation of the Go-owned approval.

## Projection rules learned here

Each readability fix in `decide/decide.gt` was the same correction: a state
that Go carried as `(value, ok)`, a flag or a sentinel became a constructor.

| First draft | Final | Effect |
| --- | --- | --- |
| `gatesValid: Bool` + `gates: List[Gate]` | `gates: List[GateEvidence]`, `GateEvidence = Gate \| InvalidGate` | first-failure order identical to the Go loop (`gate_order_test.go`) |
| `evidenceTtlMillis: Option[Int]` with "≤ 0 is invalid" | `Ttl = NoTtl \| InvalidTtl \| TtlNanos(Int)` | invalid case is a named arm |
| `observedAtPresent: Bool` + `observedAtMillis: Option[Int]` | `Timestamp = NoTimestamp \| InvalidTimestamp \| AtNanos(Int)` | `_evidenceFresh` is one tuple match, 20 → 11 lines |
| `fromOption(planGeneration, -1)` | `planGeneration != Some(plan.generation)` | no sentinel |
| milliseconds | `Int` nanoseconds | same representation as `time.Duration`, no rounding |

See [tutorial §12](../../docs/tutorial.md#12-go-shell-go4t-core-projecting-observed-state).
