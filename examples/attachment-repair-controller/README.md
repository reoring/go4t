# Attachment repair controller extraction

## Provenance

Standalone Go module `attachrepairctl`, package `storage`, extracted from
`/workspace/platform/internal/controllers/storage/` at platform commit
`0a9a5de52b47d70f5ad8f5174d1f98efbedef3c3`.

The initial extraction copied
`storage_attachment_repair_operation_controller.go` and
`storage_attachment_repair_operation_controller_test.go` verbatim; `cmp`
against each original exited 0. The operation workflow now delegates decisions
to go4t. The 22 original tests remain byte-unchanged as its oracle.

## Decision in go4t

`decide/decide.gt` owns the operation workflow: approval binding decisions,
expiry and phase interpretation, authorization, durable deletion commitment,
compare-and-swap outcomes, and convergence. Its generated boundary is
`decide.Decide(decide.Observed) decide.Action`.

Interpreter protocol:

1. Read dry-run/live mode and approval facts; initialize all three observation/result Options absent.
2. Call `Decide`; `ObserveInitial` runs the typed initial observation and fills its Option, retaining the result as `initialObs`.
3. `ObserveAuthorized` runs the typed authorized observation and fills its Option, retaining the result separately as `authorizedObs`.
4. `DeleteStale(identity)` requires an authorized observation and checks that `identity` equals `attachmentRepairObjectIdentityDigest("volume-attachment-v1", authorizedObs.staleAttachment)`. It then deletes that exact attachment with UID/resource-version preconditions and fills its result Option.
5. Call `Decide` again after each effect; `Publish` maps the final status to the existing builder and returns, using `authorizedObs` when available and otherwise `initialObs`.

Every effect guards its Option with `panic("decide asked twice")`. A missing
authorized observation, missing stale attachment, or mismatched deletion identity
panics with `decide named a stale attachment that was not observed` before deletion.
`AuthOkObs.StaleIdentity` contains the same identity digest, or the empty string
when the stale attachment is absent.

Observation chains produce the closed `decide.ObsReason` enum directly; their
optional `*obsBlock` carries that reason and the unchanged message (`nil` means
success). Authorized observations also produce `decide.Target` directly, without
a string-state conversion. No string reasons cross the go4t boundary:
`ObsBlocked`/`AuthBlocked` carry `ObsReason`, and `ReasonCode` is
`Decided(Reason)` or `FromObservation(ObsReason)`. Both reason variants are rendered
with `.String()` only when building Go status.

The original `observeInitialSafety` signature remains the oracle fixture's named
dependency, rendering the result of `observeInitialTyped` as strings. The
interpreter and approval reconciler call the typed chain directly. Observation
control flow, reason/message pairs, digests, and status builders are unchanged.
The shared `validateAttachmentRepairApprovalBinding` and
`attachmentRepairApprovalExpiresAt` functions remain verbatim because the
approval reconciler still calls them; only the operation's decisions moved.
`projectApproval` projects the same predicates and timestamp facts without
making the workflow decision.

Iteration counts include the final `Publish`:

| Path | Calls to `Decide` |
| --- | ---: |
| Approval rejection or authorized pre-observation gate/binding rejection | 1 |
| Dry-run; initial observation block; awaiting/rejected/expired approval; authorization | 2 |
| Authorized observation block; commitment; already deleting/absent convergence | 2 |
| Committed, present target: successful delete, CAS failure, or other delete failure | 3 |

`decision_protocol_test.go` observes representative 1/2/3-call traces, including
all three delete results and their identity payloads; maximum observed is **3**.
It checks typed observation block reasons and the deletion identity guard,
including mismatched identity and missing observation/attachment cases, and
preserves matched-approval safety evidence when a disabled feature gate blocks
authorization. There is no package-global iteration counter or shared
measurement state in production reconciliation.

The publisher passes the last observation (nil before any observation) and
the approval only after a successful live read. Thus dry-run has nil approval;
early approval/authorized-gate failures have nil observation; later failures
retain their observation. `AwaitingApproval` uses its original builder, which
does not take an approval. Both `Authorized` and a `Blocked` status carrying
`ApprovalMatches` set `safety["approvalMatchesObservation"]` before publishing:
the original controller set this evidence before checking the feature gate.

Decision-boundary Go line counts (function declarations through closing braces,
including internal blank lines, excluding imports and between-function blanks):

| Change | Lines |
| --- | ---: |
| Removed `reconcileOperation`, `reconcileAuthorized`, `readBoundApproval` bodies/functions | 130 |
| Added interpreter/initialization, identity guard, publisher, reason mapping, mode/approval/timestamp projection | 169 |
| Net boundary increase | 39 |
| Interpreter `for` loop itself, including its closing brace (previously 64) | 63 |
| Deletion identity guard helper | 6 |
| `publish` status mapping function | 22 |
| `reasonString` function | 10 |

These counts exclude generated Go, the observation chains and oracle-facing
observation helper, the retained shared validator/expiry helper, and test code.
The interpreter loop shrank by one line; its branches correspond to protocol
effects, not individual workflow outcomes.

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
extracted controller file.

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
go generate ./...
go generate ./... # generated Go must be byte-identical
go vet ./...
go test ./... -count=1
go test . -run '^TestAttachmentRepairDecisionProtocol$' -count=1 -v
git diff --stat -- storage_attachment_repair_operation_controller_test.go
```

Both generation runs exited 0, and all nine generated Go files were
byte-identical. Vet exited 0 without diagnostics. All 22 original top-level
tests (including the three bounded-time-window subtests), plus the new protocol
trace test, passed. Exact module test output, excluding the VM test-lock
wrapper's acquisition/release diagnostics:

```text
ok  	attachrepairctl	0.155s
?   	attachrepairctl/decide	[no test files]
```

The focused trace run reported `max decision iterations observed: 3` and:

```text
PASS
ok  	attachrepairctl	0.065s
```

The oracle's SHA-256 before and after the rewrite is
`ab5840c99a046b16f20c9fb8e66067a000cb4366c265aa1e9486ac8b1f6c0a14`.

The module uses the same Go line (`1.26.9`) and direct Kubernetes,
controller-runtime, and logr versions as `../approval-controller/go.mod`.
`require github.com/reoring/go4t v0.0.0` and `replace github.com/reoring/go4t => ../..` resolve the generated boundary's
runtime and the local generator. This is a nested module, not added to the root
module or a workspace.
