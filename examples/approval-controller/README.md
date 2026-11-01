# Approval controller extraction

Standalone Go module `approvalctl`, package `storage`, extracted from
`/workspace/platform/internal/controllers/storage/` at platform commit
`0a9a5de52b47d70f5ad8f5174d1f98efbedef3c3`.

`storage_approval_controller.go` and `storage_approval_controller_test.go` are
verbatim copies. Production helper declarations and test fixture functions are
copied verbatim from the same package, except for the fixture-client trimming
below. No controller decision logic or approval tests were changed.

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
GOFLAGS=-mod=mod go mod tidy
GOFLAGS=-mod=mod go vet ./...
GOFLAGS=-mod=mod go test ./... -count=1
```

The explicit `require gofx v0.0.0` and `replace gofx => ../..` reserve the local
module connection for later use; `go mod tidy` removes the unused requirement,
so retain it after tidying until gofx is imported. This nested module is not
added to the root module or a workspace, keeping root package discovery
unchanged (the same isolation pattern as `examples/deps/app`).
