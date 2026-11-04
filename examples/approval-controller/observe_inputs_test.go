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
