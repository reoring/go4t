package storage

import (
	"context"
	"fmt"
	"strings"
	"time"

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
	conditions := existingConditions(approval)
	status := map[string]any{
		"observedGeneration": approval.GetGeneration(),
	}

	planName := firstNonEmptyNestedString(approval.Object, []string{"spec", "planRef", "name"})
	if planName == "" {
		return r.approvalStatus(status, &conditions, approval, "Rejected", metav1.ConditionFalse, "PlanReferenceMissing", "Storage approval has no referenced resize plan"), 0
	}
	plan := newStorageResizePlanObject()
	if err := r.Client.Get(ctx, client.ObjectKey{Namespace: approval.GetNamespace(), Name: planName}, plan); err != nil {
		if apierrors.IsNotFound(err) {
			return r.approvalStatus(status, &conditions, approval, "Rejected", metav1.ConditionFalse, "PlanNotFound", "Referenced storage resize plan was not found"), 0
		}
		return r.approvalStatus(status, &conditions, approval, "Pending", metav1.ConditionUnknown, "PlanReadFailed", "Referenced storage resize plan could not be read"), time.Minute
	}

	if phase := firstStatusString(plan, "status", "phase"); phase == "Expired" {
		return r.approvalStatus(status, &conditions, approval, "Expired", metav1.ConditionFalse, "PlanExpired", "Storage resize plan expired before approval acceptance"), 0
	} else if phase == "Superseded" {
		return r.approvalStatus(status, &conditions, approval, "Superseded", metav1.ConditionFalse, "PlanSuperseded", "Storage resize plan was superseded by newer evidence"), 0
	} else if phase != "ReadyForApproval" {
		return r.approvalStatus(status, &conditions, approval, "Rejected", metav1.ConditionFalse, "PlanNotReady", "Storage resize plan is not ready for approval"), 0
	}

	if ok, reason, message := storageApprovalBindingMatchesPlan(approval, plan); !ok {
		return r.approvalStatus(status, &conditions, approval, "Rejected", metav1.ConditionFalse, reason, message), 0
	}
	if ok, reason, message := storageApprovalPolicyGenerationMatches(ctx, r.Client, approval); !ok {
		return r.approvalStatus(status, &conditions, approval, "Rejected", metav1.ConditionFalse, reason, message), 0
	}
	if ok, reason, message := storageApprovalMaintenanceWindowAllows(ctx, r.Client, approval, plan); !ok {
		return r.approvalStatus(status, &conditions, approval, "Rejected", metav1.ConditionFalse, reason, message), 0
	}
	if ok, reason, message := storageApprovalPlanGatesPassing(plan); !ok {
		return r.approvalStatus(status, &conditions, approval, "Rejected", metav1.ConditionFalse, reason, message), 0
	}
	if ok, reason, message := storageApprovalNoConflictingOperation(ctx, r.Client, approval, plan); !ok {
		return r.approvalStatus(status, &conditions, approval, "Rejected", metav1.ConditionFalse, reason, message), 0
	}
	if ok, requeueAfter, reason, message := r.storageApprovalEvidenceFresh(plan); !ok {
		return r.approvalStatus(status, &conditions, approval, "Expired", metav1.ConditionFalse, reason, message), 0
	} else if requeueAfter > 0 {
		status = r.approvalStatus(status, &conditions, approval, "Accepted", metav1.ConditionTrue, "ApprovalAccepted", "Storage approval matches current resize plan evidence")
		return status, requeueAfter
	}

	return r.approvalStatus(status, &conditions, approval, "Accepted", metav1.ConditionTrue, "ApprovalAccepted", "Storage approval matches current resize plan evidence"), 0
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

func storageApprovalBindingMatchesPlan(approval, plan *unstructured.Unstructured) (bool, string, string) {
	if generation, ok := nestedInt64(approval.Object, "spec", "binding", "planGeneration"); !ok || generation != plan.GetGeneration() {
		return false, "PlanGenerationMismatch", "Approval plan generation does not match the current resize plan"
	}
	if expected := firstNonEmptyNestedString(approval.Object, []string{"spec", "binding", "evidenceHash"}); expected == "" || expected != firstStatusString(plan, "status", "evidenceHash") {
		return false, "EvidenceHashMismatch", "Approval evidence hash does not match the current resize plan"
	}
	if expected := firstNonEmptyNestedString(approval.Object, []string{"spec", "binding", "changeSetFingerprint"}); expected == "" || expected != firstStatusString(plan, "status", "changeSetFingerprint") {
		return false, "ChangeSetFingerprintMismatch", "Approval change-set fingerprint does not match the current resize plan"
	}
	if expected := strings.TrimSpace(firstNonEmptyNestedString(approval.Object, []string{"spec", "binding", "baseGitCommit"})); expected != "" {
		if got := firstNonEmptyNestedString(plan.Object, []string{"spec", "baseRevision", "gitCommit"}); got != expected {
			return false, "BaseRevisionMismatch", "Approval base revision does not match the current resize plan"
		}
	}
	return true, "", ""
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

func storageApprovalPolicyGenerationMatches(ctx context.Context, cl client.Client, approval *unstructured.Unstructured) (bool, string, string) {
	expected, ok := nestedInt64(approval.Object, "spec", "binding", "policyGeneration")
	if !ok || expected <= 0 {
		return true, "", ""
	}
	policyName := firstNonEmptyNestedString(approval.Object, []string{"spec", "policyRef", "name"})
	if policyName == "" {
		return false, "PolicyReferenceMissing", "Approval has no referenced storage policy"
	}
	policy := newStorageAutopilotPolicyObject()
	if err := cl.Get(ctx, client.ObjectKey{Namespace: approval.GetNamespace(), Name: policyName}, policy); err != nil {
		return false, "PolicyReadFailed", "Referenced storage policy could not be read"
	}
	if policy.GetGeneration() > 0 && policy.GetGeneration() != expected {
		return false, "PolicyGenerationMismatch", "Approval policy generation does not match the current storage policy"
	}
	return true, "", ""
}

func storageApprovalMaintenanceWindowAllows(ctx context.Context, cl client.Client, approval, plan *unstructured.Unstructured) (bool, string, string) {
	windowName := firstNonEmptyNestedString(
		approval.Object,
		[]string{"spec", "maintenanceWindowRef", "name"},
		[]string{"spec", "maintenanceWindow", "name"},
	)
	if windowName == "" {
		windowName = firstNonEmptyNestedString(plan.Object, []string{"spec", "maintenanceWindowRef", "name"})
	}
	if windowName == "" {
		return true, "", ""
	}
	window := newStorageMaintenanceWindowObject()
	if err := cl.Get(ctx, client.ObjectKey{Namespace: approval.GetNamespace(), Name: windowName}, window); err != nil {
		return false, "MaintenanceWindowReadFailed", "Referenced maintenance window could not be read"
	}
	if required, ok, _ := unstructured.NestedBool(window.Object, "spec", "approvalPolicy", "required"); ok && !required {
		return false, "MaintenanceWindowApprovalDisabled", "Maintenance window disables approvals for a data-risk storage workflow"
	}
	planType := firstNonEmptyNestedString(plan.Object, []string{"spec", "type"})
	if allowed, ok := firstNestedStringSlice(window.Object, []string{"spec", "allowedOperations"}); ok && len(allowed) > 0 {
		found := false
		for _, value := range allowed {
			if value == planType {
				found = true
				break
			}
		}
		if !found {
			return false, "MaintenanceWindowOperationNotAllowed", "Maintenance window does not allow this storage workflow type"
		}
	}
	if phase := firstStatusString(window, "status", "phase"); phase == "Open" {
		return true, "", ""
	}
	if open, ok, _ := unstructured.NestedBool(window.Object, "status", "currentWindow", "open"); ok && open {
		return true, "", ""
	}
	return false, "MaintenanceWindowClosed", "Maintenance window is not open for approval acceptance"
}

func storageApprovalPlanGatesPassing(plan *unstructured.Unstructured) (bool, string, string) {
	gates, ok, _ := unstructured.NestedSlice(plan.Object, "status", "safetyGates")
	if !ok || len(gates) == 0 {
		return false, "SafetyGatesMissing", "Resize plan has no approval safety gate evidence"
	}
	for _, raw := range gates {
		gate, ok := raw.(map[string]any)
		if !ok {
			return false, "SafetyGateInvalid", "Resize plan has invalid safety gate evidence"
		}
		state := strings.TrimSpace(mapStringDefault(gate, "state"))
		switch state {
		case "Pass", "Warn":
			continue
		default:
			name := strings.TrimSpace(mapStringDefault(gate, "name"))
			if name == "" {
				name = "unknown"
			}
			return false, "SafetyGateBlocked", "Resize plan safety gate is not passing: " + name
		}
	}
	return true, "", ""
}

func storageApprovalNoConflictingOperation(ctx context.Context, cl client.Client, approval, plan *unstructured.Unstructured) (bool, string, string) {
	operations := &unstructured.UnstructuredList{}
	operations.SetGroupVersionKind(storageResizeOperationListResourceGVK)
	if err := cl.List(ctx, operations, client.InNamespace(approval.GetNamespace())); err != nil {
		return false, "OperationReadFailed", "Storage resize operations could not be read"
	}
	capabilityName := firstNonEmptyNestedString(plan.Object, []string{"spec", "storageCapabilityRef", "name"})
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
		return false, "ConflictingOperationActive", "Another storage resize operation is active for this capability"
	}
	return true, "", ""
}

func (r *ApprovalReconciler) storageApprovalEvidenceFresh(plan *unstructured.Unstructured) (bool, time.Duration, string, string) {
	ttlValue := firstNonEmptyNestedString(plan.Object, []string{"spec", "evidenceTTL"})
	if ttlValue == "" {
		return true, 0, "", ""
	}
	ttl, err := time.ParseDuration(ttlValue)
	if err != nil || ttl <= 0 {
		return false, 0, "EvidenceTTLInvalid", "Resize plan evidence TTL is invalid"
	}
	observedAtValue := firstStatusString(plan, "status", "observedAt")
	if observedAtValue == "" {
		return false, 0, "EvidenceTimestampMissing", "Resize plan evidence timestamp is missing"
	}
	observedAt, err := time.Parse(time.RFC3339, observedAtValue)
	if err != nil {
		return false, 0, "EvidenceTimestampInvalid", "Resize plan evidence timestamp is invalid"
	}
	expiresAt := observedAt.UTC().Add(ttl)
	now := r.now()
	if !now.Before(expiresAt) {
		return false, 0, "EvidenceExpired", "Resize plan evidence expired before approval acceptance"
	}
	return true, expiresAt.Sub(now), "", ""
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
