# Approval controller extraction

Standalone Go module `approvalctl`, package `storage`, extracted from
`/workspace/platform/internal/controllers/storage/` at platform commit
`0a9a5de52b47d70f5ad8f5174d1f98efbedef3c3`.

`storage_approval_controller.go` started as a verbatim copy. Its decision logic
now lives in `decide/decide.gfx`, emitted as Go package `approvalctl/decide`.
The Go shell reads the referenced plan, policy, maintenance window, and
operations into `decide.Observed`, calls `decide.Decide`, then publishes the
verdict through the original status helper. Reads happen up front, even when
an earlier decision gate will reject the approval. Operation creation and its
immutable approval-binding helpers remain unchanged.

The shared `Fetched[A]` sum represents `Fetched[Plan]`, `Fetched[Policy]`, and
`Fetched[Window]` in `Observed`. Their Go mirrors are `FetchedPlan`,
`FetchedPolicy`, and `FetchedWindow`, with constructors such as
`FetchedPlanFound{F0: plan}`. Missing policies and windows retain the same
read-error verdicts as before.

`storage_approval_controller_test.go` is still a verbatim copy and its six
tests remain the correctness oracle. Production helper declarations and test
fixtures are copied from the same package, except for the fixture-client
trimming below.

## Helpers

`helpers.go` contains:

- `condition`, `setCondition`, `conditionsToUnstructured`, `existingConditions`,
  `nestedInt64`, and transitively required `numericToInt64`.
- `newStorageResizePlanObject`, `newStorageMaintenanceWindowObject`,
  `storageResizePlanResourceGVK`, `storageMaintenanceWindowResourceGVK`.
- `isTerminalStorageResizeOperationPhase`, `newStorageResizeOperationObject`,
  `storageResizeOperationResourceGVK`, `storageResizeOperationListResourceGVK`.
- `patchUnstructuredStatus`, `firstStatusString`, `firstNonEmptyNestedString`,
  `firstNestedStringSlice`, `newStorageAutopilotPolicyObject`,
  `storageAutopilotPolicyGVK`, `mapStringDefault`.
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

`generate.go` runs `go run ../../cmd/gofx build --emit-go decide --package decide
decide/decide.gfx`. The command regenerates `decide/*.go` from the gofx source;
do not edit emitted Go files directly.

The `require gofx v0.0.0` and `replace gofx => ../..` connect the controller and
generated package to the local gofx runtime. This nested module is not added
to the root module or a workspace, keeping root package discovery unchanged
(the same isolation pattern as `examples/deps/app`).

## Known behavioural differences

Gate evidence is projected element-by-element as `GateEvidence = Gate | InvalidGate`,
so the first malformed or non-passing element decides exactly as the original
loop did; `gate_order_test.go` passes against both the original Go controller
and this version.

- A missing approval plan generation is compared using a `-1` sentinel in
  gofx. If a plan itself has generation `-1`, this gate passes instead of
  reporting `PlanGenerationMismatch`. Negative plan generations are not valid
  Kubernetes metadata, and the unchanged oracle tests do not cover them.
- TTLs, timestamps, and requeue intervals cross the gofx boundary in
  milliseconds instead of Go's nanoseconds. Fractional milliseconds are
  truncated; a positive TTL shorter than one millisecond becomes zero and is
  reported as `EvidenceTTLInvalid`. Invalid or non-positive TTLs also map to
  zero, while an absent TTL remains absent.
