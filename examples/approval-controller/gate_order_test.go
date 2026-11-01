package storage

import (
	"context"
	"testing"
	"time"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

// Gate evidence is judged in order: the first element that is malformed or
// not Pass/Warn decides the reason. This case is reachable from the API but
// was not covered by the original controller tests.
func TestApprovalObserveGateEvidenceOrder(t *testing.T) {
	now := time.Date(2026, 6, 25, 1, 2, 3, 0, time.UTC)
	tests := []struct {
		name   string
		gates  []any
		reason string
	}{
		{
			name: "blocked before malformed",
			gates: []any{
				map[string]any{"name": "Frozen", "state": "Unknown"},
				"not-an-object",
			},
			reason: "SafetyGateBlocked",
		},
		{
			name: "malformed before blocked",
			gates: []any{
				"not-an-object",
				map[string]any{"name": "Frozen", "state": "Unknown"},
			},
			reason: "SafetyGateInvalid",
		},
		{
			name: "passing before malformed",
			gates: []any{
				map[string]any{"name": "Frozen", "state": "Warn"},
				"not-an-object",
			},
			reason: "SafetyGateInvalid",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			plan := newReadyApprovalPlan(now.Add(-5 * time.Minute))
			_ = unstructured.SetNestedSlice(plan.Object, tt.gates, "status", "safetyGates")
			policy := newStorageAutopilotPolicy("default", "rook-ceph", "rook-ceph")
			policy.SetGeneration(7)
			window := newOpenStorageMaintenanceWindow("default", "window-a", "rook-ceph")
			approval := newStorageApproval("default", "approval-a")
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
