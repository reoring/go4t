# Attachment repair controller extraction

Standalone Go module `attachrepairctl`, package `storage`, extracted from
`/workspace/platform/internal/controllers/storage/` at platform commit
`0a9a5de52b47d70f5ad8f5174d1f98efbedef3c3`.

This is a pure extraction with no logic changes. Both
`storage_attachment_repair_operation_controller.go` and
`storage_attachment_repair_operation_controller_test.go` are verbatim copies.
`cmp` against each original exited 0. The original tests are the oracle for a
later gofx rewrite; no rewrite is included here.

## Production helpers

`helpers.go` contains verbatim declarations from the same platform package,
with a local import block and only the required declarations selected:

- `storage_resource_controller.go`: `patchUnstructuredStatus`.
- `storage_autopilot_status.go`: `firstStatusString`,
  `firstNonEmptyNestedString`.
- `storage_workflow_events.go`: `recordStorageWorkflowEvent`,
  `storageAttachmentRepairOperationEvent`, `storageAttachmentRepairApprovalEvent`,
  and transitive `appendAuditCorrelation`.
- `storage_feature_gates.go`: the complete `StorageFeatureGates` type. The
  environment-loading functions are not referenced and are not copied.
- `storagecapability_controller.go`: `newStorageCapabilityObject`,
  `storageCapabilityGVK`, `condition`, `setCondition`,
  `conditionsToUnstructured`, `existingConditions`, `nestedInt64`, and
  `numericToInt64`.
- The same capability source also supplies the complete `parseSpec`,
  `capabilitySpec`, and `objectGatewayEndpoint`, plus transitively required
  `parseObjectGatewayEndpoints`, `parseStringMap`, `stringValue`, and
  `replicationSizeFromInt64`. Although the repair controller only consumes the
  parsed storage class, the parser is preserved rather than simplified.
- Constants from the capability source: `StorageCapabilityGroup`,
  `StorageCapabilityVersion`, `StorageCapabilityKind`,
  `storageCapabilityManagedProvider`, `storageCapabilityConvergedMode`,
  `defaultRookNamespace`, `defaultRookClusterName`, `defaultRookStorageClassName`,
  `defaultRookVolumeSnapshotClass`, `defaultRookBlockPoolName`,
  `defaultRookFilesystemName`, `defaultRookFilesystemStorageName`,
  `defaultRookObjectBucketStorage`, `defaultRookObjectGatewayPort`,
  `defaultRookMDSCommandImage`, `defaultRookMDSSessionAutocloseSeconds`,
  `defaultRookMDSReconnectTimeoutSeconds`, `defaultRookMDSBlocklistOnTimeout`,
  `defaultRookMDSBlocklistOnEvict`, `defaultRookObjectGatewaySvcType`, and
  `defaultRookFailureDomain`.

No extra whole production files are copied. The repair operation and approval
GVKs, reconcilers, digest helpers, and node/attachment logic already live in the
verbatim controller file.

## Test fixtures and trimming

`helpers_test.go` copies `newProviderRecoveryCapability` verbatim from
`storage_provider_recovery_operation_controller_test.go` and
`hasConditionReason` verbatim from `storage_approval_controller_test.go`.

No fixture bodies or scheme registrations are trimmed. Unlike the approval
example, these tests already have their own `newAttachmentRepairTestClient`,
node/attachment builders, PVC/PV/Pod construction, and reconciliation helpers
in the copied test file. Its `corev1`, `storagev1`, and unstructured capability,
operation, and approval registrations are retained unchanged.
`newAutopilotTestClient` and `hasConditionStatus` are not referenced and are
therefore not copied; no Ceph evidence injection is introduced.

## Verification

Run within this directory:

```sh
GOFLAGS=-mod=mod go mod tidy
# Restore require gofx v0.0.0 if tidy removes it; retain replace gofx => ../..
go vet ./...
go test ./... -count=1
cmp /workspace/platform/internal/controllers/storage/storage_attachment_repair_operation_controller.go storage_attachment_repair_operation_controller.go
cmp /workspace/platform/internal/controllers/storage/storage_attachment_repair_operation_controller_test.go storage_attachment_repair_operation_controller_test.go
```

Observed: tidy succeeded, vet succeeded without diagnostics, both comparisons
exited 0, and all 22 original top-level tests passed (including the three
bounded-time-window subtests). Exact test output, excluding the VM test-lock
wrapper's acquisition/release diagnostics:

```text
ok  	attachrepairctl	0.142s
```

The module uses the same Go line (`1.26.9`) and direct Kubernetes,
controller-runtime, and logr versions as `../approval-controller/go.mod`.
`require gofx v0.0.0` and `replace gofx => ../..` are retained even though this
pure extraction does not import gofx yet. This is a nested module, not added
to the root module or a workspace.
