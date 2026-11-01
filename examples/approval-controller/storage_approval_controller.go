package storage

import (
	"context"
	"fmt"
	"strings"
	"time"

	"approvalctl/decide"

	"gofx/rt"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

const StorageApprovalKind = "StorageApproval"

var storageApprovalResourceGVK = schema.GroupVersionKind{
	Group:   StorageCapabilityGroup,
	Version: StorageCapabilityVersion,
	Kind:    StorageApprovalKind,
}

type ApprovalReconciler struct {
	Client   client.Client
	Log      logr.Logger
	Recorder record.EventRecorder
	Now      func() time.Time
}

func (r *ApprovalReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx, "storageApproval", req.NamespacedName)
	if r.Log.GetSink() != nil {
		logger = r.Log.WithValues("storageApproval", req.NamespacedName)
	}

	approval := newStorageApprovalObject()
	if err := r.Client.Get(ctx, req.NamespacedName, approval); err != nil {
		if apierrors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, fmt.Errorf("get StorageApproval: %w", err)
	}
	if !approval.GetDeletionTimestamp().IsZero() {
		return ctrl.Result{}, nil
	}

	oldPhase := firstStatusString(approval, "status", "phase")
	status, requeueAfter := r.observe(ctx, approval)
	if err := patchUnstructuredStatus(ctx, r.Client, approval, status); err != nil {
		return ctrl.Result{}, err
	}
	if status["phase"] == "Accepted" {
		if err := r.ensureOperationForAcceptedApproval(ctx, approval); err != nil {
			return ctrl.Result{}, err
		}
	}
	if oldPhase != status["phase"] {
		eventType, reason, message := storageApprovalEvent(approval, status)
		recordStorageWorkflowEvent(r.Recorder, approval, eventType, reason, message)
	}
	logger.V(1).Info("published storage approval status")
	return ctrl.Result{RequeueAfter: requeueAfter}, nil
}

func (r *ApprovalReconciler) observe(ctx context.Context, approval *unstructured.Unstructured) (map[string]any, time.Duration) {
	inputs := r.observeInputs(ctx, approval)
	verdict := decide.Decide(inputs)
	phase, conditionStatus, reason, message, requeueAfter := publish(verdict)
	conditions := existingConditions(approval)
	status := map[string]any{
		"observedGeneration": approval.GetGeneration(),
	}
	return r.approvalStatus(status, &conditions, approval, phase, conditionStatus, reason, message), requeueAfter
}

func (r *ApprovalReconciler) observeInputs(ctx context.Context, approval *unstructured.Unstructured) decide.Observed {
	planGeneration, planGenerationPresent := nestedInt64(approval.Object, "spec", "binding", "planGeneration")
	policyGeneration, policyGenerationPresent := nestedInt64(approval.Object, "spec", "binding", "policyGeneration")
	inputs := decide.Observed{
		Binding: decide.Binding{
			PlanGeneration:       rt.Option[int64]{Value: planGeneration, Present: planGenerationPresent},
			EvidenceHash:         firstNonEmptyNestedString(approval.Object, []string{"spec", "binding", "evidenceHash"}),
			ChangeSetFingerprint: firstNonEmptyNestedString(approval.Object, []string{"spec", "binding", "changeSetFingerprint"}),
			BaseGitCommit:        strings.TrimSpace(firstNonEmptyNestedString(approval.Object, []string{"spec", "binding", "baseGitCommit"})),
			PolicyGeneration:     rt.Option[int64]{Value: policyGeneration, Present: policyGenerationPresent},
		},
		Plan:   decide.FetchedPlanNotReferenced{},
		Policy: decide.FetchedPolicyNotReferenced{},
		Window: decide.FetchedWindowNotReferenced{},
	}
	windowName := firstNonEmptyNestedString(
		approval.Object,
		[]string{"spec", "maintenanceWindowRef", "name"},
		[]string{"spec", "maintenanceWindow", "name"},
	)
	capabilityName := ""
	if planName := firstNonEmptyNestedString(approval.Object, []string{"spec", "planRef", "name"}); planName != "" {
		plan := newStorageResizePlanObject()
		if err := r.Client.Get(ctx, client.ObjectKey{Namespace: approval.GetNamespace(), Name: planName}, plan); err != nil {
			if apierrors.IsNotFound(err) {
				inputs.Plan = decide.FetchedPlanMissing{}
			} else {
				inputs.Plan = decide.FetchedPlanReadError{}
			}
		} else {
			observedPlan := decide.Plan{
				Generation:           plan.GetGeneration(),
				Phase:                firstStatusString(plan, "status", "phase"),
				EvidenceHash:         firstStatusString(plan, "status", "evidenceHash"),
				ChangeSetFingerprint: firstStatusString(plan, "status", "changeSetFingerprint"),
				BaseGitCommit:        firstNonEmptyNestedString(plan.Object, []string{"spec", "baseRevision", "gitCommit"}),
				PlanType:             firstNonEmptyNestedString(plan.Object, []string{"spec", "type"}),
			}
			gates, _, _ := unstructured.NestedSlice(plan.Object, "status", "safetyGates")
			observedPlan.Gates = make([]decide.GateEvidence, len(gates))
			for i, raw := range gates {
				gate, ok := raw.(map[string]any)
				if !ok {
					observedPlan.Gates[i] = decide.GateEvidenceInvalidGate{}
					continue
				}
				observedPlan.Gates[i] = decide.GateEvidenceGate{
					Name:  strings.TrimSpace(mapStringDefault(gate, "name")),
					State: strings.TrimSpace(mapStringDefault(gate, "state")),
				}
			}
			if ttlValue := firstNonEmptyNestedString(plan.Object, []string{"spec", "evidenceTTL"}); ttlValue != "" {
				observedPlan.EvidenceTtlMillis.Present = true
				if ttl, err := time.ParseDuration(ttlValue); err == nil && ttl > 0 {
					observedPlan.EvidenceTtlMillis.Value = ttl.Milliseconds()
				}
			}
			if observedAtValue := firstStatusString(plan, "status", "observedAt"); observedAtValue != "" {
				observedPlan.ObservedAtPresent = true
				if observedAt, err := time.Parse(time.RFC3339, observedAtValue); err == nil {
					observedPlan.ObservedAtMillis = rt.Option[int64]{Value: observedAt.UnixMilli(), Present: true}
				}
			}
			inputs.Plan = decide.FetchedPlanFound{F0: observedPlan}
			if windowName == "" {
				windowName = firstNonEmptyNestedString(plan.Object, []string{"spec", "maintenanceWindowRef", "name"})
			}
			capabilityName = firstNonEmptyNestedString(plan.Object, []string{"spec", "storageCapabilityRef", "name"})
		}
	}

	if policyName := firstNonEmptyNestedString(approval.Object, []string{"spec", "policyRef", "name"}); policyName != "" {
		policy := newStorageAutopilotPolicyObject()
		if err := r.Client.Get(ctx, client.ObjectKey{Namespace: approval.GetNamespace(), Name: policyName}, policy); err != nil {
			inputs.Policy = decide.FetchedPolicyReadError{}
		} else {
			inputs.Policy = decide.FetchedPolicyFound{F0: decide.Policy{Generation: policy.GetGeneration()}}
		}
	}
	if windowName != "" {
		window := newStorageMaintenanceWindowObject()
		if err := r.Client.Get(ctx, client.ObjectKey{Namespace: approval.GetNamespace(), Name: windowName}, window); err != nil {
			inputs.Window = decide.FetchedWindowReadError{}
		} else {
			required, requiredPresent, _ := unstructured.NestedBool(window.Object, "spec", "approvalPolicy", "required")
			allowed, _ := firstNestedStringSlice(window.Object, []string{"spec", "allowedOperations"})
			open, _, _ := unstructured.NestedBool(window.Object, "status", "currentWindow", "open")
			inputs.Window = decide.FetchedWindowFound{F0: decide.Window{
				ApprovalRequired:  rt.Option[bool]{Value: required, Present: requiredPresent},
				AllowedOperations: allowed,
				Phase:             firstStatusString(window, "status", "phase"),
				CurrentWindowOpen: open,
			}}
		}
	}

	operations := &unstructured.UnstructuredList{}
	operations.SetGroupVersionKind(storageResizeOperationListResourceGVK)
	inputs.OperationsReadable = r.Client.List(ctx, operations, client.InNamespace(approval.GetNamespace())) == nil
	if inputs.OperationsReadable {
		for i := range operations.Items {
			operation := &operations.Items[i]
			if operation.GetName() == "" {
				continue
			}
			if firstNonEmptyNestedString(operation.Object, []string{"spec", "approvalRef", "name"}) == approval.GetName() {
				continue
			}
			if firstNonEmptyNestedString(operation.Object, []string{"spec", "storageCapabilityRef", "name"}) != capabilityName {
				continue
			}
			if isTerminalStorageResizeOperationPhase(firstStatusString(operation, "status", "phase")) {
				continue
			}
			inputs.ConflictingOperation = true
			break
		}
	}
	inputs.NowMillis = r.now().UnixMilli()
	return inputs
}

func publish(verdict decide.Verdict) (phase string, cond metav1.ConditionStatus, reason, message string, requeue time.Duration) {
	switch verdict := verdict.(type) {
	case decide.VerdictRejected:
		return "Rejected", metav1.ConditionFalse, verdict.F0.String(), verdict.F1, 0
	case decide.VerdictExpired:
		return "Expired", metav1.ConditionFalse, verdict.F0.String(), verdict.F1, 0
	case decide.VerdictSuperseded:
		return "Superseded", metav1.ConditionFalse, verdict.F0.String(), verdict.F1, 0
	case decide.VerdictPending:
		return "Pending", metav1.ConditionUnknown, verdict.F0.String(), verdict.F1, time.Duration(verdict.F2) * time.Millisecond
	case decide.VerdictAccepted:
		return "Accepted", metav1.ConditionTrue, decide.ReasonApprovalAccepted.String(), "Storage approval matches current resize plan evidence", time.Duration(verdict.F0) * time.Millisecond
	default:
		panic("invalid approval verdict")
	}
}

func (r *ApprovalReconciler) ensureOperationForAcceptedApproval(ctx context.Context, approval *unstructured.Unstructured) error {
	planName := firstNonEmptyNestedString(approval.Object, []string{"spec", "planRef", "name"})
	if planName == "" {
		return nil
	}
	plan := newStorageResizePlanObject()
	if err := r.Client.Get(ctx, client.ObjectKey{Namespace: approval.GetNamespace(), Name: planName}, plan); err != nil {
		return fmt.Errorf("get accepted approval plan: %w", err)
	}
	name := storageOperationNameForApproval(approval, plan)
	existing := newStorageResizeOperationObject()
	if err := r.Client.Get(ctx, client.ObjectKey{Namespace: approval.GetNamespace(), Name: name}, existing); err == nil {
		if firstNonEmptyNestedString(existing.Object, []string{"spec", "approvalRef", "name"}) == approval.GetName() {
			return nil
		}
		return fmt.Errorf("storage resize operation %s/%s already exists for another approval", approval.GetNamespace(), name)
	} else if !apierrors.IsNotFound(err) {
		return fmt.Errorf("get storage resize operation: %w", err)
	}
	operation := newStorageResizeOperationObject()
	operation.SetNamespace(approval.GetNamespace())
	operation.SetName(name)
	operation.SetLabels(map[string]string{
		"appthrust.io/managed-by":                  "storage-operator",
		"storage.appthrust.io/resize-plan":         plan.GetName(),
		"storage.appthrust.io/storage-approval":    approval.GetName(),
		"storage.appthrust.io/storage-capability":  firstNonEmptyNestedString(plan.Object, []string{"spec", "storageCapabilityRef", "name"}),
		"storage.appthrust.io/workflow-generation": fmt.Sprintf("%d", approval.GetGeneration()),
	})
	executionProfile := firstStatusString(plan, "status", "executionProfile", "profile")
	if executionProfile == "" {
		executionProfile = "ManualHold"
	}
	spec := map[string]any{
		"workflowChannel":      "W",
		"type":                 firstNonEmptyNestedString(plan.Object, []string{"spec", "type"}),
		"planRef":              map[string]any{"name": plan.GetName()},
		"storageCapabilityRef": map[string]any{"name": firstNonEmptyNestedString(plan.Object, []string{"spec", "storageCapabilityRef", "name"})},
		"approvalRef":          map[string]any{"name": approval.GetName()},
		"approvalBinding":      storageOperationApprovalBindingSnapshot(approval, plan),
		"executionProfile":     executionProfile,
		"rollbackPolicy":       "StopAndRequireOperator",
	}
	if idempotencyKey := firstNonEmptyNestedString(approval.Object, []string{"spec", "idempotencyKey"}); idempotencyKey != "" {
		spec["idempotencyKey"] = idempotencyKey
	}
	if auditCorrelationID := firstNonEmptyNestedString(approval.Object, []string{"spec", "auditCorrelationID"}); auditCorrelationID != "" {
		spec["auditCorrelationID"] = auditCorrelationID
	}
	operation.Object["spec"] = spec
	if err := r.Client.Create(ctx, operation); err != nil {
		return err
	}
	message := appendAuditCorrelation("Storage resize operation "+operation.GetName()+" was created after approval acceptance", firstNonEmptyNestedString(approval.Object, []string{"spec", "auditCorrelationID"}))
	recordStorageWorkflowEvent(r.Recorder, approval, corev1.EventTypeNormal, "StorageResizeOperationCreated", message)
	return nil
}

func (r *ApprovalReconciler) approvalStatus(status map[string]any, conditions *[]metav1.Condition, approval *unstructured.Unstructured, phase string, conditionStatus metav1.ConditionStatus, reason, message string) map[string]any {
	status["phase"] = phase
	status["message"] = message
	setCondition(conditions, condition(approval, "Accepted", conditionStatus, reason, message))
	status["conditions"] = conditionsToUnstructured(*conditions)
	return status
}

func storageOperationApprovalBindingSnapshot(approval, plan *unstructured.Unstructured) map[string]any {
	binding := map[string]any{}
	if generation, ok := nestedInt64(approval.Object, "spec", "binding", "planGeneration"); ok && generation > 0 {
		binding["planGeneration"] = generation
	} else if plan.GetGeneration() > 0 {
		binding["planGeneration"] = plan.GetGeneration()
	}
	if evidence := firstNonEmptyNestedString(approval.Object, []string{"spec", "binding", "evidenceHash"}); evidence != "" {
		binding["evidenceHash"] = evidence
	}
	if fingerprint := firstNonEmptyNestedString(approval.Object, []string{"spec", "binding", "changeSetFingerprint"}); fingerprint != "" {
		binding["changeSetFingerprint"] = fingerprint
	}
	if baseGitCommit := firstNonEmptyNestedString(approval.Object, []string{"spec", "binding", "baseGitCommit"}); baseGitCommit != "" {
		binding["baseGitCommit"] = baseGitCommit
	}
	if policyGeneration, ok := nestedInt64(approval.Object, "spec", "binding", "policyGeneration"); ok && policyGeneration > 0 {
		binding["policyGeneration"] = policyGeneration
	}
	return binding
}

func storageOperationApprovalBindingMatches(operation, approval, plan *unstructured.Unstructured) (bool, string, string) {
	binding, ok, _ := unstructured.NestedMap(operation.Object, "spec", "approvalBinding")
	if !ok || len(binding) == 0 {
		return false, "OperationApprovalBindingMissing", "Storage operation is missing immutable approval evidence binding"
	}
	if expected, ok := nestedInt64(approval.Object, "spec", "binding", "planGeneration"); ok && expected > 0 {
		got, gotOK := nestedInt64(operation.Object, "spec", "approvalBinding", "planGeneration")
		if !gotOK || got != expected {
			return false, "OperationPlanGenerationBindingDrift", "Storage operation approval binding no longer matches the accepted plan generation"
		}
		if plan.GetGeneration() > 0 && got != plan.GetGeneration() {
			return false, "OperationPlanGenerationBindingDrift", "Storage operation approval binding no longer matches the current reviewed plan generation"
		}
	}
	if expected := firstNonEmptyNestedString(approval.Object, []string{"spec", "binding", "evidenceHash"}); expected != "" {
		got := firstNonEmptyNestedString(operation.Object, []string{"spec", "approvalBinding", "evidenceHash"})
		if got != expected || (firstStatusString(plan, "status", "evidenceHash") != "" && got != firstStatusString(plan, "status", "evidenceHash")) {
			return false, "OperationEvidenceBindingDrift", "Storage operation approval binding no longer matches the accepted evidence hash"
		}
	}
	if expected := firstNonEmptyNestedString(approval.Object, []string{"spec", "binding", "changeSetFingerprint"}); expected != "" {
		got := firstNonEmptyNestedString(operation.Object, []string{"spec", "approvalBinding", "changeSetFingerprint"})
		if got != expected || (firstStatusString(plan, "status", "changeSetFingerprint") != "" && got != firstStatusString(plan, "status", "changeSetFingerprint")) {
			return false, "OperationChangeSetBindingDrift", "Storage operation approval binding no longer matches the accepted change-set fingerprint"
		}
	}
	if expected := strings.TrimSpace(firstNonEmptyNestedString(approval.Object, []string{"spec", "binding", "baseGitCommit"})); expected != "" {
		got := strings.TrimSpace(firstNonEmptyNestedString(operation.Object, []string{"spec", "approvalBinding", "baseGitCommit"}))
		if got != expected {
			return false, "OperationBaseRevisionBindingDrift", "Storage operation approval binding no longer matches the accepted base revision"
		}
		if planCommit := strings.TrimSpace(firstNonEmptyNestedString(plan.Object, []string{"spec", "baseRevision", "gitCommit"})); planCommit != "" && got != planCommit {
			return false, "OperationBaseRevisionBindingDrift", "Storage operation approval binding no longer matches the reviewed plan base revision"
		}
	}
	if expected, ok := nestedInt64(approval.Object, "spec", "binding", "policyGeneration"); ok && expected > 0 {
		got, gotOK := nestedInt64(operation.Object, "spec", "approvalBinding", "policyGeneration")
		if !gotOK || got != expected {
			return false, "OperationPolicyGenerationBindingDrift", "Storage operation approval binding no longer matches the accepted policy generation"
		}
	}
	if expected := firstNonEmptyNestedString(approval.Object, []string{"spec", "auditCorrelationID"}); expected != "" {
		got := firstNonEmptyNestedString(operation.Object, []string{"spec", "auditCorrelationID"})
		if got == "" {
			return false, "OperationAuditCorrelationMissing", "Storage operation is missing the accepted approval audit correlation"
		}
		if got != expected {
			return false, "OperationAuditCorrelationDrift", "Storage operation audit correlation no longer matches the accepted approval"
		}
	}
	return true, "", ""
}

func (r *ApprovalReconciler) now() time.Time {
	if r.Now != nil {
		return r.Now().UTC()
	}
	return time.Now().UTC()
}

func newStorageApprovalObject() *unstructured.Unstructured {
	obj := &unstructured.Unstructured{}
	obj.SetGroupVersionKind(storageApprovalResourceGVK)
	return obj
}

func storageOperationNameForApproval(approval, plan *unstructured.Unstructured) string {
	if plan.GetName() != "" {
		return plan.GetName() + "-operation"
	}
	return approval.GetName() + "-operation"
}

func (r *ApprovalReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(newStorageApprovalObject()).
		Complete(r)
}
