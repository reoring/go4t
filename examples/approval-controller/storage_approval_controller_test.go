package storage

import (
	"context"
	"testing"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	kruntime "k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestApprovalObserveAcceptsFreshMatchingPlan(t *testing.T) {
	now := time.Date(2026, 6, 25, 1, 2, 3, 0, time.UTC)
	plan := newReadyApprovalPlan(now.Add(-5 * time.Minute))
	policy := newStorageAutopilotPolicy("default", "rook-ceph", "rook-ceph")
	policy.SetGeneration(7)
	window := newOpenStorageMaintenanceWindow("default", "window-a", "rook-ceph")
	approval := newStorageApproval("default", "approval-a")
	reconciler := &ApprovalReconciler{
		Client: newAutopilotTestClient(t, plan, policy, window),
		Now:    func() time.Time { return now },
	}

	status, requeueAfter := reconciler.observe(context.Background(), approval)

	if status["phase"] != "Accepted" {
		t.Fatalf("status = %#v, want Accepted", status)
	}
	if requeueAfter <= 0 {
		t.Fatalf("requeueAfter = %v, want evidence TTL requeue", requeueAfter)
	}
	if !hasConditionStatus(status["conditions"].([]any), "Accepted", "True") {
		t.Fatalf("conditions = %#v, want Accepted=True", status["conditions"])
	}
}

func TestApprovalObserveExpiresStaleEvidence(t *testing.T) {
	now := time.Date(2026, 6, 25, 1, 2, 3, 0, time.UTC)
	plan := newReadyApprovalPlan(now.Add(-31 * time.Minute))
	policy := newStorageAutopilotPolicy("default", "rook-ceph", "rook-ceph")
	policy.SetGeneration(7)
	window := newOpenStorageMaintenanceWindow("default", "window-a", "rook-ceph")
	approval := newStorageApproval("default", "approval-a")
	reconciler := &ApprovalReconciler{
		Client: newAutopilotTestClient(t, plan, policy, window),
		Now:    func() time.Time { return now },
	}

	status, _ := reconciler.observe(context.Background(), approval)

	if status["phase"] != "Expired" {
		t.Fatalf("status = %#v, want Expired", status)
	}
	if !hasConditionReason(status["conditions"].([]any), "Accepted", "EvidenceExpired") {
		t.Fatalf("conditions = %#v, want EvidenceExpired", status["conditions"])
	}
}

func TestApprovalObserveRejectsStaleOrBlockedPlanBinding(t *testing.T) {
	now := time.Date(2026, 6, 25, 1, 2, 3, 0, time.UTC)
	tests := []struct {
		name   string
		mutate func(*unstructured.Unstructured, *unstructured.Unstructured, *unstructured.Unstructured)
		reason string
	}{
		{
			name: "evidence hash drift",
			mutate: func(plan, _, _ *unstructured.Unstructured) {
				_ = unstructured.SetNestedField(plan.Object, "sha256:new-evidence", "status", "evidenceHash")
			},
			reason: "EvidenceHashMismatch",
		},
		{
			name: "change-set fingerprint drift",
			mutate: func(plan, _, _ *unstructured.Unstructured) {
				_ = unstructured.SetNestedField(plan.Object, "sha256:new-change", "status", "changeSetFingerprint")
			},
			reason: "ChangeSetFingerprintMismatch",
		},
		{
			name: "base revision drift",
			mutate: func(plan, _, _ *unstructured.Unstructured) {
				_ = unstructured.SetNestedField(plan.Object, "def456", "spec", "baseRevision", "gitCommit")
			},
			reason: "BaseRevisionMismatch",
		},
		{
			name: "policy generation drift",
			mutate: func(_ *unstructured.Unstructured, policy, _ *unstructured.Unstructured) {
				policy.SetGeneration(8)
			},
			reason: "PolicyGenerationMismatch",
		},
		{
			name: "maintenance window closed",
			mutate: func(_, _, window *unstructured.Unstructured) {
				_ = unstructured.SetNestedField(window.Object, "Closed", "status", "phase")
				_ = unstructured.SetNestedField(window.Object, false, "status", "currentWindow", "open")
			},
			reason: "MaintenanceWindowClosed",
		},
		{
			name: "maintenance window approval disabled",
			mutate: func(_, _, window *unstructured.Unstructured) {
				_ = unstructured.SetNestedField(window.Object, false, "spec", "approvalPolicy", "required")
			},
			reason: "MaintenanceWindowApprovalDisabled",
		},
		{
			name: "blocking safety gate",
			mutate: func(plan, _, _ *unstructured.Unstructured) {
				_ = unstructured.SetNestedSlice(plan.Object, []any{
					map[string]any{"name": "ConsumerProvisioningFrozen", "state": "Unknown", "reason": "FreezeUnknown"},
				}, "status", "safetyGates")
			},
			reason: "SafetyGateBlocked",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			plan := newReadyApprovalPlan(now.Add(-5 * time.Minute))
			policy := newStorageAutopilotPolicy("default", "rook-ceph", "rook-ceph")
			policy.SetGeneration(7)
			window := newOpenStorageMaintenanceWindow("default", "window-a", "rook-ceph")
			approval := newStorageApproval("default", "approval-a")
			tt.mutate(plan, policy, window)
			reconciler := &ApprovalReconciler{
				Client: newAutopilotTestClient(t, plan, policy, window),
				Now:    func() time.Time { return now },
			}

			status, _ := reconciler.observe(context.Background(), approval)

			if status["phase"] != "Rejected" {
				t.Fatalf("status = %#v, want Rejected", status)
			}
			if !hasConditionReason(status["conditions"].([]any), "Accepted", tt.reason) {
				t.Fatalf("conditions = %#v, want %s", status["conditions"], tt.reason)
			}
		})
	}
}

func TestApprovalObserveSupersedesWhenPlanSuperseded(t *testing.T) {
	now := time.Date(2026, 6, 25, 1, 2, 3, 0, time.UTC)
	plan := newReadyApprovalPlan(now.Add(-5 * time.Minute))
	_ = unstructured.SetNestedField(plan.Object, "Superseded", "status", "phase")
	policy := newStorageAutopilotPolicy("default", "rook-ceph", "rook-ceph")
	policy.SetGeneration(7)
	window := newOpenStorageMaintenanceWindow("default", "window-a", "rook-ceph")
	approval := newStorageApproval("default", "approval-a")
	reconciler := &ApprovalReconciler{
		Client: newAutopilotTestClient(t, plan, policy, window),
		Now:    func() time.Time { return now },
	}

	status, _ := reconciler.observe(context.Background(), approval)

	if status["phase"] != "Superseded" {
		t.Fatalf("status = %#v, want Superseded", status)
	}
	if !hasConditionReason(status["conditions"].([]any), "Accepted", "PlanSuperseded") {
		t.Fatalf("conditions = %#v, want PlanSuperseded", status["conditions"])
	}
}

func TestApprovalReconcileCreatesOperationOnce(t *testing.T) {
	now := time.Date(2026, 6, 25, 1, 2, 3, 0, time.UTC)
	plan := newReadyApprovalPlan(now.Add(-5 * time.Minute))
	policy := newStorageAutopilotPolicy("default", "rook-ceph", "rook-ceph")
	policy.SetGeneration(7)
	window := newOpenStorageMaintenanceWindow("default", "window-a", "rook-ceph")
	approval := newStorageApproval("default", "approval-a")
	reconciler := &ApprovalReconciler{
		Client: newApprovalTestClient(t, plan, policy, window, approval),
		Now:    func() time.Time { return now },
	}
	req := ctrl.Request{NamespacedName: types.NamespacedName{Namespace: "default", Name: "approval-a"}}

	if _, err := reconciler.Reconcile(context.Background(), req); err != nil {
		t.Fatalf("reconcile: %v", err)
	}
	if _, err := reconciler.Reconcile(context.Background(), req); err != nil {
		t.Fatalf("reconcile replay: %v", err)
	}

	operation := newStorageResizeOperationObject()
	if err := reconciler.Client.Get(context.Background(), types.NamespacedName{Namespace: "default", Name: "plan-a-operation"}, operation); err != nil {
		t.Fatalf("get operation: %v", err)
	}
	if got := firstNonEmptyNestedString(operation.Object, []string{"spec", "approvalRef", "name"}); got != "approval-a" {
		t.Fatalf("operation approvalRef = %q, want approval-a", got)
	}
	if got := firstNonEmptyNestedString(operation.Object, []string{"spec", "executionProfile"}); got != "ManualHold" {
		t.Fatalf("operation executionProfile = %q, want ManualHold", got)
	}
	binding, ok, _ := unstructured.NestedMap(operation.Object, "spec", "approvalBinding")
	if !ok {
		t.Fatalf("operation approvalBinding missing: %#v", operation.Object["spec"])
	}
	if binding["evidenceHash"] != "sha256:evidence" || binding["changeSetFingerprint"] != "sha256:change" || binding["baseGitCommit"] != "abc123" || binding["planGeneration"] != int64(1) || binding["policyGeneration"] != int64(7) {
		t.Fatalf("operation approvalBinding = %#v, want accepted approval snapshot", binding)
	}
}

func TestApprovalObserveRejectsConflictingActiveOperation(t *testing.T) {
	now := time.Date(2026, 6, 25, 1, 2, 3, 0, time.UTC)
	plan := newReadyApprovalPlan(now.Add(-5 * time.Minute))
	policy := newStorageAutopilotPolicy("default", "rook-ceph", "rook-ceph")
	policy.SetGeneration(7)
	window := newOpenStorageMaintenanceWindow("default", "window-a", "rook-ceph")
	approval := newStorageApproval("default", "approval-a")
	operation := newStorageResizeOperation("default", "other-operation", "Running", 1)
	reconciler := &ApprovalReconciler{
		Client: newAutopilotTestClient(t, plan, policy, window, approval, operation),
		Now:    func() time.Time { return now },
	}

	status, _ := reconciler.observe(context.Background(), approval)

	if status["phase"] != "Rejected" {
		t.Fatalf("status = %#v, want Rejected", status)
	}
	if !hasConditionReason(status["conditions"].([]any), "Accepted", "ConflictingOperationActive") {
		t.Fatalf("conditions = %#v, want ConflictingOperationActive", status["conditions"])
	}
}

func newReadyApprovalPlan(observedAt time.Time) *unstructured.Unstructured {
	plan := newStorageResizePlan("default", "plan-a", "rook-ceph", "rook-ceph")
	_ = unstructured.SetNestedField(plan.Object, "30m", "spec", "evidenceTTL")
	plan.Object["status"] = map[string]any{
		"phase":                "ReadyForApproval",
		"decision":             "RequireApproval",
		"observedAt":           observedAt.UTC().Format(time.RFC3339),
		"evidenceHash":         "sha256:evidence",
		"changeSetFingerprint": "sha256:change",
		"safetyGates": []any{
			map[string]any{"name": "SafeToDestroyEvidenceReady", "state": "Pass", "reason": "Safe"},
			map[string]any{"name": "ConsumerProvisioningFrozen", "state": "Pass", "reason": "Frozen"},
			map[string]any{"name": "DataProtectionRecentEnough", "state": "Pass", "reason": "Protected"},
		},
	}
	return plan
}

func newStorageApproval(namespace, name string) *unstructured.Unstructured {
	approval := newStorageApprovalObject()
	approval.SetNamespace(namespace)
	approval.SetName(name)
	approval.SetGeneration(1)
	approval.SetCreationTimestamp(metav1.NewTime(time.Date(2026, 6, 25, 0, 0, 0, 0, time.UTC)))
	approval.Object["spec"] = map[string]any{
		"workflowChannel": "W",
		"planRef":         map[string]any{"name": "plan-a"},
		"policyRef":       map[string]any{"name": "rook-ceph"},
		"binding": map[string]any{
			"planGeneration":       int64(1),
			"evidenceHash":         "sha256:evidence",
			"changeSetFingerprint": "sha256:change",
			"baseGitCommit":        "abc123",
			"policyGeneration":     int64(7),
		},
		"approvedBy": "reoring",
		"approvedAt": metav1.NewTime(time.Date(2026, 6, 25, 1, 0, 0, 0, time.UTC)).Format(time.RFC3339),
		"reason":     "approved after evidence review",
	}
	return approval
}

func hasConditionReason(conditions []any, conditionType, reason string) bool {
	for _, item := range conditions {
		condition, ok := item.(map[string]any)
		if !ok {
			continue
		}
		if condition["type"] == conditionType && condition["reason"] == reason {
			return true
		}
	}
	return false
}

func newApprovalTestClient(t *testing.T, objects ...client.Object) client.Client {
	t.Helper()
	scheme := kruntime.NewScheme()
	for _, gvk := range []schema.GroupVersionKind{
		storageResizePlanResourceGVK,
		storageApprovalResourceGVK,
		storageResizeOperationResourceGVK,
		storageMaintenanceWindowResourceGVK,
		storageAutopilotPolicyGVK,
	} {
		scheme.AddKnownTypeWithName(gvk, &unstructured.Unstructured{})
		scheme.AddKnownTypeWithName(gvk.GroupVersion().WithKind(gvk.Kind+"List"), &unstructured.UnstructuredList{})
	}
	return fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(objects...).
		WithStatusSubresource(newStorageApprovalObject()).
		Build()
}
