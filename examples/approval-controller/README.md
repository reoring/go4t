# Approval controller extraction

Standalone Go module `approvalctl`, package `storage`, extracted from
`/workspace/platform/internal/controllers/storage/` at platform commit
`0a9a5de52b47d70f5ad8f5174d1f98efbedef3c3`.

`storage_approval_controller.go` started as a verbatim copy. Observation and
projection now live in `decide/observe.gt`, the pure decision in
`decide/decide.gt`, and publication and application in `decide/apply.gt`.
They emit into Go package `approvalctl/_decide_gen` (package name `decide`).
The Go shell owns the native client, request lookup, deletion guard, optional
test clock, event recorder, and requeue loop. It calls `decide.ObserveInputs`,
`decide.Decide`, and `decide.Apply`; it no longer constructs status, conditions,
or operations.

go4t performs dependency `Get`/`List`, status `MergeFrom` / `Status().Patch`,
and operation `Create` through generated controller-runtime bindings. Reads
still happen up front in plan, policy, window, operations order even when an
earlier decision gate will reject the approval. Apply patches status first,
ensures the operation only for Accepted, and returns ordered workflow events
for the shell to record. An unchanged status skips the patch; an existing
operation for the same approval skips creation.

The exported boundary preserves native handles, not copied client records:

```go
func ObserveInputs(c client.Client, approval *unstructured.Unstructured) func(context.Context) (Observed, error)
func Apply(c client.Client, approval *unstructured.Unstructured, verdict Verdict) func(context.Context) (Outcome, error)
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

`storage_approval_controller_test.go` and `gate_order_test.go` remain unchanged
correctness oracles. Their read-only `observe` entry point is test-only glue in
`observe_inputs_test.go`: go4t previews the proposed approval without mutating
the original. Production apply uses the original native approval handle and
does not allocate that preview copy.

## Generated bindings

Cross-package upcasts are now generated per
[ADR 0009](../../docs/adr/0009-cross-package-upcasts-and-lock-values.md):
`client.unstructuredAsObject` and `client.unstructuredListAsObjectList`
preserve native handles for `clientGet` and `clientList`. go4t constructs
unstructured objects with their GVK and uses `types.NamespacedName` for reads;
namespace-scoped lists pass `client.inNamespaceAsListOption(namespace)` (ADR 0010).
`io.tryIO` maps successful reads to `Found`,
`io.NotFound` to `Missing`, and other read failures to `ReadError`.

All bindings required by apply are available; there is no skipped apply piece.
`metav1.Condition` is a generated record. `meta.SetStatusCondition` is also
bound, but its `*[]Condition` argument crosses as a copy and its only result is
`Bool`, so it cannot return the changed list. go4t's `setCondition` instead
returns the updated immutable list with the same first-match and transition-time
semantics. Similarly, status updates use the native `unstructuredSetObject`
setter rather than mutating a copied `Map` through `SetNestedMap`.

The `metav1.Time` JSON-scalar binding reparses timestamps with `Time.UnmarshalJSON`,
which changes their location to `time.Local`. Existing condition timestamps are
therefore validated by `time.Parse` and rendered from their original wall time
and offset in go4t, without that lossy conversion. This preserves RFC3339
formatting, including fraction truncation and zero/invalid timestamps.

Read and patch error messages keep their original contextual prefixes.
`io.tryIO` exposes classified errors rather than Go's original error chain;
contextual failures therefore retain the message, not `%w` identity.

## What moved

Line counts below cover complete handwritten files, including imports and
comments, excluding generated code and tests.

| Area | Before this apply migration | After |
| --- | --- | --- |
| Go reconciler | 271 lines, including publication and operation helpers | 88 lines: request/client loop, observe glue, manager setup |
| Go helpers | 240 lines, including status, conditions and workflow events | 79 lines: native object constructors, GVKs and fixture string lookup |
| Observation and projection | 241-line `decide/observe.gt` | Unchanged, entirely go4t |
| Pure decision | 205-line `decide/decide.gt` | 205 lines; only the publication comment changed |
| Status and conditions | Go `publish`, `approvalStatus`, condition conversion and merge patch | go4t `publish`, `approvalStatus`, immutable `setCondition`, native status patch |
| Operations and events | Go ensure/create, snapshot/match/name helpers and event projection | 409-line `decide/apply.gt`, including publication and oracle preview |

**Export naming:** helpers use `_projectFetchedPlan`, `_projectTtl`, and
`_projectTimestamp`, not `plan`, `ttl`, or `timestamp`. This avoids first-rune
upper-casing collisions with `Plan`, `Ttl`, and `Timestamp` under SPEC §8.7;
the compiler is unchanged.

## Helpers

`helpers.go` contains:

- Native constructors for approval, resize plan, maintenance window,
  resize operation, and autopilot policy.
- Their GVKs and resource constants.
- `firstNonEmptyNestedString`, still used by the unchanged Go oracle.

All migrated Go status, condition, numeric-projection, operation, and event
helpers were deleted. The optional recorder consumes the events returned by
go4t; event classification, wording, audit correlation, and order live in go4t.

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

Apply-boundary tests additionally cover all verdict publications, stable
transition times with changed reason/message/generation, timezone and fractional
timestamp parity, condition filtering,
status replay without a second patch, operation replay without a second create,
native status persistence, optional operation fields and labels, approval
binding drift, event ordering, and patch/read/create failure boundaries.
Existing requeue values are preserved exactly, including the decision's
`Pending(PlanReadFailed, ..., 60000)` nanoseconds; the IO-boundary failure
fallback remains one minute.

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
