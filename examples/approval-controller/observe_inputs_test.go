package storage

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	decide "approvalctl/_decide_gen"

	"go4t/rt"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// Keep the original read-only oracle entry point without a Go implementation
// of publication or condition construction. Only this test preview copies the
// approval; production applies status to the original native handle.
func (r *ApprovalReconciler) observe(ctx context.Context, approval *unstructured.Unstructured) (map[string]any, time.Duration) {
	verdict := r.observeVerdict(ctx, approval)
	proposed, err := decide.PreviewApproval(approval, verdict)(context.WithoutCancel(ctx))
	if err != nil {
		panic(err)
	}
	return proposed.Object["status"].(map[string]any), time.Duration(decide.Publish(verdict).RequeueNanos)
}

func TestObserveInputsNativeClientProjection(t *testing.T) {
	observedAt := time.Date(2026, 6, 25, 1, 2, 3, 123456789, time.UTC)
	plan := newReadyApprovalPlan(observedAt)
	_ = unstructured.SetNestedField(plan.Object, "1ns", "spec", "evidenceTTL")
	_ = unstructured.SetNestedField(plan.Object, observedAt.Format(time.RFC3339Nano), "status", "observedAt")
	_ = unstructured.SetNestedSlice(plan.Object, []any{
		map[string]any{"name": " Frozen ", "state": " Warn "},
		"malformed",
	}, "status", "safetyGates")
	policy := newStorageAutopilotPolicy("default", "rook-ceph", "rook-ceph")
	policy.SetGeneration(7)
	window := newOpenStorageMaintenanceWindow("default", "window-a", "rook-ceph")
	_ = unstructured.SetNestedField(window.Object, false, "spec", "approvalPolicy", "required")
	_ = unstructured.SetNestedSlice(window.Object, []any{"OSDScaleIn", int64(3), "Other"}, "spec", "allowedOperations")
	approval := newStorageApproval("default", "approval-a")
	_ = unstructured.SetNestedField(approval.Object, float64(1.75), "spec", "binding", "planGeneration")
	_ = unstructured.SetNestedField(approval.Object, "7", "spec", "binding", "policyGeneration")
	_ = unstructured.SetNestedField(approval.Object, " abc123 ", "spec", "binding", "baseGitCommit")
	before := approval.DeepCopy()
	c := newAutopilotTestClient(t, plan, policy, window)

	observed, err := decide.ObserveInputs(c, approval)(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if observed.Binding.PlanGeneration != rt.Some(int64(1)) || observed.Binding.PolicyGeneration != rt.Some(int64(7)) || observed.Binding.BaseGitCommit != "abc123" {
		t.Fatalf("binding = %#v", observed.Binding)
	}
	p, ok := observed.Plan.(decide.FetchedPlanFound)
	if !ok {
		t.Fatalf("plan = %#v", observed.Plan)
	}
	if p.F0.Generation != 1 || p.F0.EvidenceTtl != (decide.TtlTtlNanos{F0: 1}) || p.F0.ObservedAt != (decide.TimestampAtNanos{F0: observedAt.UnixNano()}) {
		t.Fatalf("plan projection = %#v", p.F0)
	}
	if !reflect.DeepEqual(p.F0.Gates, []decide.GateEvidence{decide.GateEvidenceGate{Name: "Frozen", State: "Warn"}, decide.GateEvidenceInvalidGate{}}) {
		t.Fatalf("gates = %#v", p.F0.Gates)
	}
	w, ok := observed.Window.(decide.FetchedWindowFound)
	if !ok || w.F0.ApprovalRequired != rt.Some(false) || !reflect.DeepEqual(w.F0.AllowedOperations, []string{"OSDScaleIn", "Other"}) || w.F0.Phase != "Open" {
		t.Fatalf("window = %#v", observed.Window)
	}
	if observed.Policy != (decide.FetchedPolicyFound{F0: decide.Policy{Generation: 7}}) || !observed.OperationsReadable || observed.ConflictingOperation {
		t.Fatalf("observation = %#v", observed)
	}
	if !reflect.DeepEqual(approval.Object, before.Object) {
		t.Fatal("observation mutated the Go-owned approval")
	}
}

func TestObserveInputsReadOutcomesAndEagerReads(t *testing.T) {
	for _, readError := range []bool{false, true} {
		t.Run(map[bool]string{false: "not found", true: "read error"}[readError], func(t *testing.T) {
			approval := newStorageApproval("default", "approval-a")
			_ = unstructured.SetNestedField(approval.Object, "window-a", "spec", "maintenanceWindowRef", "name")
			c := &observationReadClient{Client: newApprovalTestClient(t), failGet: readError, failList: true}
			observed, err := decide.ObserveInputs(c, approval)(context.Background())
			if err != nil {
				t.Fatal(err)
			}
			var wantPlan decide.FetchedPlan = decide.FetchedPlanMissing{}
			if readError {
				wantPlan = decide.FetchedPlanReadError{}
			}
			if observed.Plan != wantPlan || observed.Policy != (decide.FetchedPolicyReadError{}) || observed.Window != (decide.FetchedWindowReadError{}) || observed.OperationsReadable || observed.ConflictingOperation {
				t.Fatalf("read outcomes = %#v", observed)
			}
			if !reflect.DeepEqual(c.reads, []string{"StorageResizePlan", "StorageAutopilotPolicy", "StorageMaintenanceWindow", "StorageResizeOperationList"}) {
				t.Fatalf("read order = %v", c.reads)
			}
		})
	}
}

func TestObserveInputsMissingReferencesAndCancellation(t *testing.T) {
	approval := newStorageApprovalObject()
	approval.SetNamespace("default")
	c := &observationReadClient{Client: newApprovalTestClient(t)}
	observed, err := decide.ObserveInputs(c, approval)(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if observed.Plan != (decide.FetchedPlanNotReferenced{}) || observed.Policy != (decide.FetchedPolicyNotReferenced{}) || observed.Window != (decide.FetchedWindowNotReferenced{}) || !observed.OperationsReadable {
		t.Fatalf("missing references = %#v", observed)
	}
	if !reflect.DeepEqual(c.reads, []string{"StorageResizeOperationList"}) {
		t.Fatalf("read order = %v", c.reads)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err = decide.ObserveInputs(c, approval)(ctx)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("cancellation = %v", err)
	}
}

type observationReadClient struct {
	client.Client
	failGet  bool
	failList bool
	reads    []string
}

func (c *observationReadClient) Get(ctx context.Context, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
	c.reads = append(c.reads, obj.GetObjectKind().GroupVersionKind().Kind)
	if c.failGet {
		return errors.New("read unavailable")
	}
	return c.Client.Get(ctx, key, obj, opts...)
}

func (c *observationReadClient) List(ctx context.Context, objects client.ObjectList, opts ...client.ListOption) error {
	c.reads = append(c.reads, objects.GetObjectKind().GroupVersionKind().Kind)
	if c.failList {
		return errors.New("list unavailable")
	}
	return c.Client.List(ctx, objects, opts...)
}

func TestPublishVerdictContract(t *testing.T) {
	tests := []struct {
		verdict                                decide.Verdict
		phase, conditionStatus, reason, message string
		requeue                                int64
	}{
		{decide.VerdictRejected{F0: decide.ReasonPlanNotFound, F1: "missing"}, "Rejected", "False", "PlanNotFound", "missing", 0},
		{decide.VerdictExpired{F0: decide.ReasonEvidenceExpired, F1: "stale"}, "Expired", "False", "EvidenceExpired", "stale", 0},
		{decide.VerdictSuperseded{F0: decide.ReasonPlanSuperseded, F1: "newer"}, "Superseded", "False", "PlanSuperseded", "newer", 0},
		{decide.VerdictPending{F0: decide.ReasonPlanReadFailed, F1: "unreadable", F2: 60000}, "Pending", "Unknown", "PlanReadFailed", "unreadable", 60000},
		{decide.VerdictAccepted{F0: 123456789}, "Accepted", "True", "ApprovalAccepted", "Storage approval matches current resize plan evidence", 123456789},
	}
	for _, tt := range tests {
		t.Run(tt.phase, func(t *testing.T) {
			got := decide.Publish(tt.verdict)
			if got.Phase != tt.phase || got.ConditionStatus != tt.conditionStatus || got.Reason != tt.reason || got.Message != tt.message || got.RequeueNanos != tt.requeue {
				t.Fatalf("publication = %#v, want %#v", got, tt)
			}
		})
	}
}

func TestApprovalConditionTimestampParity(t *testing.T) {
	for _, timestamp := range []string{
		"", "invalid", "0001-01-01T00:00:00Z",
		"2026-06-24T01:02:03Z", "2026-06-24T01:02:03.123456789Z",
		"2026-06-24T01:02:03,123+05:30", "2026-06-24T01:02:03-04:00",
		"2026-06-24T1:02:03+00:00", "2026-06-24T01:02:03-00:00",
		"2026-06-24T01:02:03+01:60",
	} {
		t.Run(timestamp, func(t *testing.T) {
			approval := newStorageApproval("default", "approval-a")
			approval.Object["status"] = map[string]any{"conditions": []any{
				map[string]any{"type": "Accepted", "status": "False", "reason": "OldReason", "lastTransitionTime": timestamp},
			}}
			before := approval.DeepCopy()
			proposed, err := decide.PreviewApproval(approval, decide.VerdictRejected{F0: decide.ReasonPlanNotFound, F1: "missing"})(context.Background())
			if err != nil {
				t.Fatal(err)
			}
			parsed, _ := time.Parse(time.RFC3339, timestamp)
			want := parsed.Format(time.RFC3339)
			conditions := proposed.Object["status"].(map[string]any)["conditions"].([]any)
			got := conditions[0].(map[string]any)["lastTransitionTime"]
			if got != want || !reflect.DeepEqual(approval.Object, before.Object) {
				t.Fatalf("timestamp = %q, want %q; approval unchanged = %t", got, want, reflect.DeepEqual(approval.Object, before.Object))
			}
		})
	}
}

func TestApplyStatusConditionsAndReplay(t *testing.T) {
	ctx := context.Background()
	approval := newStorageApproval("default", "approval-a")
	approval.SetGeneration(9)
	_ = unstructured.SetNestedField(approval.Object, " audit-123 ", "spec", "auditCorrelationID")
	previous := "2026-06-24T01:02:03Z"
	approval.Object["status"] = map[string]any{
		"phase": "Pending",
		"conditions": []any{
			"invalid",
			map[string]any{"type": "Incomplete", "status": "True"},
			map[string]any{"type": "Other", "status": "True", "reason": "Unrelated", "message": "keep", "observedGeneration": "4", "lastTransitionTime": previous},
			map[string]any{"type": "Accepted", "status": "False", "reason": "OldReason", "message": "old", "observedGeneration": int64(1), "lastTransitionTime": previous},
		},
	}
	c := &approvalApplyClient{Client: newApprovalTestClient(t, approval)}
	beforeSpec, _, _ := unstructured.NestedMap(approval.Object, "spec")
	verdict := decide.VerdictRejected{F0: decide.ReasonPlanNotFound, F1: "missing"}
	outcome, err := decide.Apply(c, approval, verdict)(ctx)
	if err != nil {
		t.Fatal(err)
	}
	status := approval.Object["status"].(map[string]any)
	conditions := status["conditions"].([]any)
	if len(conditions) != 2 || conditions[0].(map[string]any)["type"] != "Other" {
		t.Fatalf("conditions = %#v", conditions)
	}
	accepted := conditions[1].(map[string]any)
	if accepted["lastTransitionTime"] != previous || accepted["observedGeneration"] != int64(9) || accepted["reason"] != "PlanNotFound" || accepted["message"] != "missing" {
		t.Fatalf("accepted condition = %#v", accepted)
	}
	if status["observedGeneration"] != int64(9) || status["phase"] != "Rejected" || outcome.RequeueNanos != 0 {
		t.Fatalf("status = %#v, outcome = %#v", status, outcome)
	}
	wantEvents := []decide.WorkflowEvent{{EventType: "Warning", Reason: "StorageApprovalRejected", Message: "missing auditCorrelationID=audit-123"}}
	if !reflect.DeepEqual(outcome.Events, wantEvents) {
		t.Fatalf("events = %#v", outcome.Events)
	}
	if !reflect.DeepEqual(beforeSpec, approval.Object["spec"]) || c.patches != 1 || c.creates != 0 {
		t.Fatalf("spec or mutation counts changed: patches=%d creates=%d", c.patches, c.creates)
	}
	saved := newStorageApprovalObject()
	if err := c.Get(ctx, client.ObjectKeyFromObject(approval), saved); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(saved.Object["status"], status) {
		t.Fatalf("persisted status = %#v, want %#v", saved.Object["status"], status)
	}
	replay, err := decide.Apply(c, approval, verdict)(ctx)
	if err != nil || c.patches != 1 || len(replay.Events) != 0 {
		t.Fatalf("replay = %#v, error = %v, patches = %d", replay, err, c.patches)
	}
	pending, err := decide.Apply(c, approval, decide.VerdictPending{F0: decide.ReasonPlanReadFailed, F1: "unreadable", F2: 17})(ctx)
	if err != nil {
		t.Fatal(err)
	}
	conditions = approval.Object["status"].(map[string]any)["conditions"].([]any)
	accepted = conditions[1].(map[string]any)
	if accepted["status"] != "Unknown" || accepted["lastTransitionTime"] == previous || pending.RequeueNanos != 17 || len(pending.Events) != 0 {
		t.Fatalf("transition = %#v, outcome = %#v", accepted, pending)
	}
}

func TestApplyCreatesOperationWithBindingAndEvents(t *testing.T) {
	ctx := context.Background()
	approval := newStorageApproval("default", "approval-a")
	_ = unstructured.SetNestedField(approval.Object, "key-123", "spec", "idempotencyKey")
	_ = unstructured.SetNestedField(approval.Object, " audit-123 ", "spec", "auditCorrelationID")
	plan := newReadyApprovalPlan(time.Now())
	_ = unstructured.SetNestedField(plan.Object, "Reviewed", "status", "executionProfile", "profile")
	c := &approvalApplyClient{Client: newApprovalTestClient(t, approval, plan)}
	outcome, err := decide.Apply(c, approval, decide.VerdictAccepted{F0: 71})(ctx)
	if err != nil {
		t.Fatal(err)
	}
	operation := newStorageResizeOperationObject()
	if err := c.Get(ctx, client.ObjectKey{Namespace: "default", Name: "plan-a-operation"}, operation); err != nil {
		t.Fatal(err)
	}
	spec := operation.Object["spec"].(map[string]any)
	if spec["executionProfile"] != "Reviewed" || spec["rollbackPolicy"] != "StopAndRequireOperator" || spec["idempotencyKey"] != "key-123" || spec["auditCorrelationID"] != " audit-123 " {
		t.Fatalf("operation spec = %#v", spec)
	}
	wantLabels := map[string]string{
		"appthrust.io/managed-by":                  "storage-operator",
		"storage.appthrust.io/resize-plan":         "plan-a",
		"storage.appthrust.io/storage-approval":    "approval-a",
		"storage.appthrust.io/storage-capability":  "rook-ceph",
		"storage.appthrust.io/workflow-generation": "1",
	}
	if !reflect.DeepEqual(operation.GetLabels(), wantLabels) {
		t.Fatalf("operation labels = %#v", operation.GetLabels())
	}
	matches, err := decide.StorageOperationApprovalBindingMatches(operation, approval, plan)(ctx)
	if err != nil || !matches.F0 || matches.F1 != "" || matches.F2 != "" {
		t.Fatalf("binding match = %#v, error = %v", matches, err)
	}
	wantEvents := []decide.WorkflowEvent{
		{EventType: "Normal", Reason: "StorageResizeOperationCreated", Message: "Storage resize operation plan-a-operation was created after approval acceptance auditCorrelationID=audit-123"},
		{EventType: "Normal", Reason: "StorageApprovalAccepted", Message: "Storage approval matches current resize plan evidence auditCorrelationID=audit-123"},
	}
	if outcome.RequeueNanos != 71 || !reflect.DeepEqual(outcome.Events, wantEvents) {
		t.Fatalf("outcome = %#v", outcome)
	}
	replay, err := decide.Apply(c, approval, decide.VerdictAccepted{F0: 70})(ctx)
	if err != nil || c.creates != 1 || c.patches != 1 || len(replay.Events) != 0 || replay.RequeueNanos != 70 {
		t.Fatalf("replay = %#v, error = %v, patches=%d creates=%d", replay, err, c.patches, c.creates)
	}
	for _, tt := range []struct{ field, reason string }{
		{"planGeneration", "OperationPlanGenerationBindingDrift"},
		{"evidenceHash", "OperationEvidenceBindingDrift"},
		{"changeSetFingerprint", "OperationChangeSetBindingDrift"},
		{"baseGitCommit", "OperationBaseRevisionBindingDrift"},
		{"policyGeneration", "OperationPolicyGenerationBindingDrift"},
	} {
		t.Run(tt.field, func(t *testing.T) {
			drifted := operation.DeepCopy()
			unstructured.RemoveNestedField(drifted.Object, "spec", "approvalBinding", tt.field)
			got, err := decide.StorageOperationApprovalBindingMatches(drifted, approval, plan)(ctx)
			if err != nil || got.F0 || got.F1 != tt.reason || got.F2 == "" {
				t.Fatalf("binding drift = %#v, error = %v", got, err)
			}
		})
	}
}

func TestApplyFailureBoundaries(t *testing.T) {
	ctx := context.Background()
	for _, tt := range []struct {
		name, failGetKind, wantError string
		patchErr, createErr          error
		conflicting                 bool
		wantCreates                 int
	}{
		{name: "patch", patchErr: errors.New("status unavailable"), wantError: "patch StorageApproval status: status unavailable"},
		{name: "plan read", failGetKind: StorageResizePlanKind, wantError: "get accepted approval plan: read unavailable"},
		{name: "operation read", failGetKind: StorageResizeOperationKind, wantError: "get storage resize operation: read unavailable"},
		{name: "create", createErr: errors.New("create unavailable"), wantError: "create unavailable", wantCreates: 1},
		{name: "conflicting approval", conflicting: true, wantError: "storage resize operation default/plan-a-operation already exists for another approval"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			approval := newStorageApproval("default", "approval-a")
			plan := newReadyApprovalPlan(time.Now())
			objects := []client.Object{approval, plan}
			if tt.conflicting {
				operation := newStorageResizeOperation("default", "plan-a-operation", "Running", 0)
				_ = unstructured.SetNestedField(operation.Object, "other-approval", "spec", "approvalRef", "name")
				objects = append(objects, operation)
			}
			c := &approvalApplyClient{Client: newApprovalTestClient(t, objects...), patchErr: tt.patchErr, createErr: tt.createErr, failGetKind: tt.failGetKind}
			_, err := decide.Apply(c, approval, decide.VerdictAccepted{})(ctx)
			if err == nil || err.Error() != tt.wantError || c.creates != tt.wantCreates {
				t.Fatalf("error = %v, creates = %d; want %q, %d", err, c.creates, tt.wantError, tt.wantCreates)
			}
			if tt.patchErr != nil && len(c.reads) != 0 {
				t.Fatalf("operation reads after failed status patch: %v", c.reads)
			}
		})
	}
}

type approvalApplyClient struct {
	client.Client
	patches, creates   int
	patchErr, createErr error
	failGetKind        string
	reads              []string
}

func (c *approvalApplyClient) Get(ctx context.Context, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
	kind := obj.GetObjectKind().GroupVersionKind().Kind
	c.reads = append(c.reads, kind)
	if kind == c.failGetKind {
		return errors.New("read unavailable")
	}
	return c.Client.Get(ctx, key, obj, opts...)
}

func (c *approvalApplyClient) Create(ctx context.Context, obj client.Object, opts ...client.CreateOption) error {
	c.creates++
	if c.createErr != nil {
		return c.createErr
	}
	return c.Client.Create(ctx, obj, opts...)
}

func (c *approvalApplyClient) Status() client.SubResourceWriter {
	return &approvalStatusWriter{SubResourceWriter: c.Client.Status(), owner: c}
}

type approvalStatusWriter struct {
	client.SubResourceWriter
	owner *approvalApplyClient
}

func (w *approvalStatusWriter) Patch(ctx context.Context, obj client.Object, patch client.Patch, opts ...client.SubResourcePatchOption) error {
	w.owner.patches++
	if w.owner.patchErr != nil {
		return w.owner.patchErr
	}
	return w.SubResourceWriter.Patch(ctx, obj, patch, opts...)
}
