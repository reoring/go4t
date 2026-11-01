package storage

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	storagev1 "k8s.io/api/storage/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	kruntime "k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

var attachmentRepairTestNow = time.Date(2026, 7, 15, 9, 0, 0, 0, time.UTC)

type attachmentRepairDeleteRecordingClient struct {
	client.Client
	deleteCalls int
	deleteOpts  []client.DeleteOption
}

func (c *attachmentRepairDeleteRecordingClient) Delete(ctx context.Context, obj client.Object, opts ...client.DeleteOption) error {
	if _, ok := obj.(*storagev1.VolumeAttachment); ok {
		c.deleteCalls++
		c.deleteOpts = append(c.deleteOpts, opts...)
	}
	return c.Client.Delete(ctx, obj, opts...)
}

func TestAttachmentRepairDryRunPublishesRedactedObservation(t *testing.T) {
	fixture := newAttachmentRepairFixture(true)
	cl := newAttachmentRepairTestClient(fixture.objects...)
	reconciler := fixture.operationReconciler(cl, false)

	result := reconcileAttachmentRepairOperation(t, reconciler, fixture.operation)
	if result.RequeueAfter != 0 {
		t.Fatalf("requeueAfter = %s, want zero", result.RequeueAfter)
	}
	status := attachmentRepairObjectStatus(t, cl, fixture.operation, newStorageAttachmentRepairOperationObject())
	if status["phase"] != "Succeeded" || status["currentStage"] != "DryRunComplete" || !validAttachmentRepairDigest(fmt.Sprint(status["observationDigest"])) {
		t.Fatalf("status = %#v, want successful dry-run observation", status)
	}
	assertAttachmentRepairStatusRedacted(t, status, fixture)
	if _, found := fixture.operation.Object["spec"].(map[string]any)["preconditions"]; found {
		t.Fatal("operation spec unexpectedly contains raw preconditions")
	}
}

func TestAttachmentRepairNonDryRunAwaitsApprovalWithoutRawIdentityInput(t *testing.T) {
	fixture := newAttachmentRepairFixture(false)
	cl := newAttachmentRepairTestClient(fixture.objects...)
	reconciler := fixture.operationReconciler(cl, true)

	result := reconcileAttachmentRepairOperation(t, reconciler, fixture.operation)
	status := attachmentRepairObjectStatus(t, cl, fixture.operation, newStorageAttachmentRepairOperationObject())
	if result.RequeueAfter != attachmentRepairRequeueInterval || status["phase"] != "AwaitingApproval" || status["currentStage"] != "AwaitApproval" {
		t.Fatalf("result=%#v status=%#v, want AwaitingApproval", result, status)
	}
	if !validAttachmentRepairDigest(fmt.Sprint(status["observationDigest"])) {
		t.Fatalf("observationDigest = %#v", status["observationDigest"])
	}
	spec := fixture.operation.Object["spec"].(map[string]any)
	for _, forbidden := range []string{"preconditions", "executionApproval", "persistentVolume", "volumeAttachment", "resourceVersion"} {
		if _, found := spec[forbidden]; found {
			t.Fatalf("operation spec exposes %s: %#v", forbidden, spec)
		}
	}
}

func TestAttachmentRepairApprovalAcceptsExactOperationObservation(t *testing.T) {
	fixture := newAttachmentRepairFixture(false)
	fixture.addExactApproval(t)
	cl := newAttachmentRepairTestClient(fixture.objects...)
	reconciler := fixture.approvalReconciler(cl)

	reconcileAttachmentRepairApproval(t, reconciler, fixture.approval)
	status := attachmentRepairObjectStatus(t, cl, fixture.approval, newStorageAttachmentRepairApprovalObject())
	if status["phase"] != "Accepted" || !hasConditionReason(status["conditions"].([]any), "Ready", "ApprovalAccepted") {
		t.Fatalf("status = %#v, want Accepted", status)
	}
	assertApprovalStatusRedacted(t, status, fixture)
}

func TestAttachmentRepairApprovalRejectsTargetBindingDrift(t *testing.T) {
	fixture := newAttachmentRepairFixture(false)
	fixture.addExactApproval(t)
	approvalSpec := fixture.approval.Object["spec"].(map[string]any)
	approvalSpec["targetBinding"] = map[string]any{
		"productCluster": map[string]any{
			"group": "runtime.appthrust.io", "kind": "ClusterDeployment", "namespace": "mgmt",
			"name": "wlc-next", "uid": "replacement-deployment-uid", "generation": int64(1),
		},
	}
	cl := newAttachmentRepairTestClient(fixture.objects...)

	reconcileAttachmentRepairApproval(t, fixture.approvalReconciler(cl), fixture.approval)
	status := attachmentRepairObjectStatus(t, cl, fixture.approval, newStorageAttachmentRepairApprovalObject())
	if status["phase"] != "Rejected" || !hasConditionReason(status["conditions"].([]any), "Ready", "ApprovalTargetBindingMismatch") {
		t.Fatalf("status = %#v, want target-binding rejection", status)
	}
}

func TestAttachmentRepairDeclarativeApprovalExecutesCASAndConverges(t *testing.T) {
	fixture := newAttachmentRepairFixture(false)
	fixture.addExactApproval(t)
	baseClient := newAttachmentRepairTestClient(fixture.objects...)
	recording := &attachmentRepairDeleteRecordingClient{Client: baseClient}
	operationReconciler := fixture.operationReconciler(recording, true)
	approvalReconciler := fixture.approvalReconciler(recording)

	reconcileAttachmentRepairApproval(t, approvalReconciler, fixture.approval)
	reconcileAttachmentRepairOperation(t, operationReconciler, fixture.operation)
	status := attachmentRepairObjectStatus(t, recording, fixture.operation, newStorageAttachmentRepairOperationObject())
	if status["phase"] != "Running" || status["currentStage"] != "AuthorizeExactDeletion" || recording.deleteCalls != 0 {
		t.Fatalf("status=%#v deleteCalls=%d, want durable authorization", status, recording.deleteCalls)
	}

	reconcileAttachmentRepairOperation(t, operationReconciler, fixture.operation)
	status = attachmentRepairObjectStatus(t, recording, fixture.operation, newStorageAttachmentRepairOperationObject())
	if status["phase"] != "Running" || status["currentStage"] != "CommitExactDeletion" || recording.deleteCalls != 0 {
		t.Fatalf("second status=%#v deleteCalls=%d, want durable deletion commitment", status, recording.deleteCalls)
	}

	reconcileAttachmentRepairOperation(t, operationReconciler, fixture.operation)
	status = attachmentRepairObjectStatus(t, recording, fixture.operation, newStorageAttachmentRepairOperationObject())
	if status["phase"] != "Running" || status["currentStage"] != "WaitForConvergence" || recording.deleteCalls != 1 {
		t.Fatalf("third status=%#v deleteCalls=%d, want exact delete", status, recording.deleteCalls)
	}
	deleteOptions := &client.DeleteOptions{}
	for _, option := range recording.deleteOpts {
		option.ApplyToDelete(deleteOptions)
	}
	if deleteOptions.Preconditions == nil || deleteOptions.Preconditions.UID == nil || *deleteOptions.Preconditions.UID != fixture.stale.UID || deleteOptions.Preconditions.ResourceVersion == nil || *deleteOptions.Preconditions.ResourceVersion != fixture.stale.ResourceVersion {
		t.Fatalf("delete preconditions = %#v, want live stale UID/RV", deleteOptions.Preconditions)
	}

	current := &storagev1.VolumeAttachment{}
	mustGetAttachmentRepairObject(t, recording, fixture.current, current)
	current.Status.Attached = true
	if err := recording.Status().Update(context.Background(), current); err != nil {
		t.Fatalf("attach current: %v", err)
	}
	reconcileAttachmentRepairOperation(t, operationReconciler, fixture.operation)
	status = attachmentRepairObjectStatus(t, recording, fixture.operation, newStorageAttachmentRepairOperationObject())
	if status["phase"] != "Succeeded" || status["currentStage"] != "Complete" {
		t.Fatalf("status = %#v, want converged success", status)
	}
	assertAttachmentRepairStatusRedacted(t, status, fixture)
}

func TestAttachmentRepairApprovalSupersedesDriftedObservation(t *testing.T) {
	fixture := newAttachmentRepairFixture(false)
	fixture.addExactApproval(t)
	fixture.pod.ResourceVersion = "2"
	cl := newAttachmentRepairTestClient(fixture.objects...)

	reconcileAttachmentRepairApproval(t, fixture.approvalReconciler(cl), fixture.approval)
	status := attachmentRepairObjectStatus(t, cl, fixture.approval, newStorageAttachmentRepairApprovalObject())
	if status["phase"] != "Superseded" || !hasConditionReason(status["conditions"].([]any), "Ready", "ObservationDigestChanged") {
		t.Fatalf("status = %#v, want ObservationDigestChanged", status)
	}
}

func TestAttachmentRepairApprovalRejectsWrongOperationUID(t *testing.T) {
	fixture := newAttachmentRepairFixture(false)
	fixture.addExactApproval(t)
	fixture.approval.Object["spec"].(map[string]any)["operationRef"].(map[string]any)["uid"] = "wrong-operation-uid"
	cl := newAttachmentRepairTestClient(fixture.objects...)

	reconcileAttachmentRepairApproval(t, fixture.approvalReconciler(cl), fixture.approval)
	status := attachmentRepairObjectStatus(t, cl, fixture.approval, newStorageAttachmentRepairApprovalObject())
	if status["phase"] != "Rejected" || !hasConditionReason(status["conditions"].([]any), "Ready", "ApprovalOperationIdentityMismatch") {
		t.Fatalf("status = %#v, want rejected exact identity", status)
	}
}

func TestAttachmentRepairApprovalRejectsMissingServerDerivedActor(t *testing.T) {
	fixture := newAttachmentRepairFixture(false)
	fixture.addExactApproval(t)
	delete(fixture.approval.Object["spec"].(map[string]any), "approvedBy")
	cl := newAttachmentRepairTestClient(fixture.objects...)

	reconcileAttachmentRepairApproval(t, fixture.approvalReconciler(cl), fixture.approval)
	status := attachmentRepairObjectStatus(t, cl, fixture.approval, newStorageAttachmentRepairApprovalObject())
	if status["phase"] != "Rejected" || !hasConditionReason(status["conditions"].([]any), "Ready", "ApprovalEvidenceIncomplete") {
		t.Fatalf("status = %#v, want incomplete approval rejection", status)
	}
}

func TestAttachmentRepairApprovalEnforcesBoundedTimeWindow(t *testing.T) {
	tests := []struct {
		name       string
		approvedAt time.Time
		expiresAt  time.Time
		phase      string
		reason     string
	}{
		{
			name:       "future approval time",
			approvedAt: attachmentRepairTestNow.Add(time.Minute),
			expiresAt:  attachmentRepairTestNow.Add(30 * time.Minute),
			phase:      "Rejected",
			reason:     "ApprovalTimeInvalid",
		},
		{
			name:       "expired approval",
			approvedAt: attachmentRepairTestNow.Add(-30 * time.Minute),
			expiresAt:  attachmentRepairTestNow,
			phase:      "Expired",
			reason:     "ApprovalExpired",
		},
		{
			name:       "window longer than one hour",
			approvedAt: attachmentRepairTestNow.Add(-time.Minute),
			expiresAt:  attachmentRepairTestNow.Add(60 * time.Minute),
			phase:      "Rejected",
			reason:     "ApprovalWindowInvalid",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fixture := newAttachmentRepairFixture(false)
			fixture.addExactApproval(t)
			spec := fixture.approval.Object["spec"].(map[string]any)
			spec["approvedAt"] = tt.approvedAt.Format(time.RFC3339)
			spec["expiresAt"] = tt.expiresAt.Format(time.RFC3339)
			cl := newAttachmentRepairTestClient(fixture.objects...)

			reconcileAttachmentRepairApproval(t, fixture.approvalReconciler(cl), fixture.approval)
			status := attachmentRepairObjectStatus(t, cl, fixture.approval, newStorageAttachmentRepairApprovalObject())
			if status["phase"] != tt.phase || !hasConditionReason(status["conditions"].([]any), "Ready", tt.reason) {
				t.Fatalf("status = %#v, want phase=%s reason=%s", status, tt.phase, tt.reason)
			}
		})
	}
}

func TestAttachmentRepairExpiryBeforeCommitBlocksMutation(t *testing.T) {
	fixture := newAttachmentRepairFixture(false)
	fixture.addExactApproval(t)
	baseClient := newAttachmentRepairTestClient(fixture.objects...)
	recording := &attachmentRepairDeleteRecordingClient{Client: baseClient}
	now := attachmentRepairTestNow
	nowFn := func() time.Time { return now }
	operationReconciler := fixture.operationReconcilerAt(recording, true, nowFn)
	approvalReconciler := fixture.approvalReconcilerAt(recording, nowFn)

	reconcileAttachmentRepairApproval(t, approvalReconciler, fixture.approval)
	reconcileAttachmentRepairOperation(t, operationReconciler, fixture.operation)
	now = attachmentRepairTestNow.Add(30 * time.Minute)
	reconcileAttachmentRepairApproval(t, approvalReconciler, fixture.approval)
	reconcileAttachmentRepairOperation(t, operationReconciler, fixture.operation)

	approvalStatus := attachmentRepairObjectStatus(t, recording, fixture.approval, newStorageAttachmentRepairApprovalObject())
	operationStatus := attachmentRepairObjectStatus(t, recording, fixture.operation, newStorageAttachmentRepairOperationObject())
	if approvalStatus["phase"] != "Expired" || operationStatus["phase"] != "Blocked" || !hasConditionReason(operationStatus["conditions"].([]any), "Ready", "ApprovalExpired") || recording.deleteCalls != 0 {
		t.Fatalf("approval=%#v operation=%#v deleteCalls=%d, want pre-commit expiry block", approvalStatus, operationStatus, recording.deleteCalls)
	}
}

func TestAttachmentRepairExpiryAfterCommitContinuesCASAndConvergence(t *testing.T) {
	fixture := newAttachmentRepairFixture(false)
	fixture.addExactApproval(t)
	baseClient := newAttachmentRepairTestClient(fixture.objects...)
	recording := &attachmentRepairDeleteRecordingClient{Client: baseClient}
	now := attachmentRepairTestNow
	nowFn := func() time.Time { return now }
	operationReconciler := fixture.operationReconcilerAt(recording, true, nowFn)
	approvalReconciler := fixture.approvalReconcilerAt(recording, nowFn)

	reconcileAttachmentRepairApproval(t, approvalReconciler, fixture.approval)
	reconcileAttachmentRepairOperation(t, operationReconciler, fixture.operation)
	reconcileAttachmentRepairOperation(t, operationReconciler, fixture.operation)
	now = attachmentRepairTestNow.Add(30 * time.Minute)
	reconcileAttachmentRepairApproval(t, approvalReconciler, fixture.approval)
	reconcileAttachmentRepairOperation(t, operationReconciler, fixture.operation)

	approvalStatus := attachmentRepairObjectStatus(t, recording, fixture.approval, newStorageAttachmentRepairApprovalObject())
	operationStatus := attachmentRepairObjectStatus(t, recording, fixture.operation, newStorageAttachmentRepairOperationObject())
	if approvalStatus["phase"] != "Expired" || operationStatus["phase"] != "Running" || operationStatus["currentStage"] != "WaitForConvergence" || recording.deleteCalls != 1 {
		t.Fatalf("approval=%#v operation=%#v deleteCalls=%d, want committed CAS continuation", approvalStatus, operationStatus, recording.deleteCalls)
	}
	deleteOptions := &client.DeleteOptions{}
	for _, option := range recording.deleteOpts {
		option.ApplyToDelete(deleteOptions)
	}
	if deleteOptions.Preconditions == nil || deleteOptions.Preconditions.UID == nil || *deleteOptions.Preconditions.UID != fixture.stale.UID || deleteOptions.Preconditions.ResourceVersion == nil || *deleteOptions.Preconditions.ResourceVersion != fixture.stale.ResourceVersion {
		t.Fatalf("delete preconditions = %#v, want committed exact UID/RV", deleteOptions.Preconditions)
	}

	current := &storagev1.VolumeAttachment{}
	mustGetAttachmentRepairObject(t, recording, fixture.current, current)
	current.Status.Attached = true
	if err := recording.Status().Update(context.Background(), current); err != nil {
		t.Fatalf("attach current: %v", err)
	}
	reconcileAttachmentRepairOperation(t, operationReconciler, fixture.operation)
	operationStatus = attachmentRepairObjectStatus(t, recording, fixture.operation, newStorageAttachmentRepairOperationObject())
	if operationStatus["phase"] != "Succeeded" {
		t.Fatalf("operation status = %#v, want post-expiry convergence", operationStatus)
	}
}

func TestAttachmentRepairExecutionStillRequiresFeatureGate(t *testing.T) {
	fixture := newAttachmentRepairFixture(false)
	fixture.addExactApproval(t)
	baseClient := newAttachmentRepairTestClient(fixture.objects...)
	recording := &attachmentRepairDeleteRecordingClient{Client: baseClient}
	reconcileAttachmentRepairApproval(t, fixture.approvalReconciler(recording), fixture.approval)
	reconcileAttachmentRepairOperation(t, fixture.operationReconciler(recording, false), fixture.operation)
	status := attachmentRepairObjectStatus(t, recording, fixture.operation, newStorageAttachmentRepairOperationObject())
	if status["phase"] != "Blocked" || !hasConditionReason(status["conditions"].([]any), "Ready", "FeatureGateDisabled") || recording.deleteCalls != 0 {
		t.Fatalf("status=%#v deleteCalls=%d, want feature-gate block", status, recording.deleteCalls)
	}
}

func TestAttachmentRepairAllowsExplicitStaleNodeAbsence(t *testing.T) {
	fixture := newAttachmentRepairFixture(false)
	fixture.removeObject(fixture.staleNode)
	fixture.addExactApproval(t)
	cl := newAttachmentRepairTestClient(fixture.objects...)
	reconcileAttachmentRepairApproval(t, fixture.approvalReconciler(cl), fixture.approval)
	reconcileAttachmentRepairOperation(t, fixture.operationReconciler(cl, true), fixture.operation)
	status := attachmentRepairObjectStatus(t, cl, fixture.operation, newStorageAttachmentRepairOperationObject())
	safety := status["safety"].(map[string]any)
	if status["phase"] != "Running" || safety["staleNodeAbsent"] != true || safety["repairAdmitted"] != true {
		t.Fatalf("status = %#v, want authorized stale-node-absent evidence", status)
	}
}

func TestAttachmentRepairConvergesWhenStaleNodeDisappearsAfterDelete(t *testing.T) {
	fixture := newAttachmentRepairFixture(false)
	fixture.addExactApproval(t)
	cl := newAttachmentRepairTestClient(fixture.objects...)
	operationReconciler := fixture.operationReconciler(cl, true)
	reconcileAttachmentRepairApproval(t, fixture.approvalReconciler(cl), fixture.approval)
	reconcileAttachmentRepairOperation(t, operationReconciler, fixture.operation)
	reconcileAttachmentRepairOperation(t, operationReconciler, fixture.operation)
	reconcileAttachmentRepairOperation(t, operationReconciler, fixture.operation)

	staleNode := &corev1.Node{}
	mustGetAttachmentRepairObject(t, cl, fixture.staleNode, staleNode)
	if err := cl.Delete(context.Background(), staleNode); err != nil {
		t.Fatalf("delete stale node: %v", err)
	}
	current := &storagev1.VolumeAttachment{}
	mustGetAttachmentRepairObject(t, cl, fixture.current, current)
	current.Status.Attached = true
	if err := cl.Status().Update(context.Background(), current); err != nil {
		t.Fatalf("attach current: %v", err)
	}
	reconcileAttachmentRepairOperation(t, operationReconciler, fixture.operation)
	status := attachmentRepairObjectStatus(t, cl, fixture.operation, newStorageAttachmentRepairOperationObject())
	if status["phase"] != "Succeeded" {
		t.Fatalf("status = %#v, want success after stale node disappearance", status)
	}
}

func TestAttachmentRepairTreatsTerminatingPodAsUnsafe(t *testing.T) {
	fixture := newAttachmentRepairFixture(false)
	fixture.pod.Finalizers = []string{"test.appthrust.io/hold"}
	fixture.pod.DeletionTimestamp = &metav1.Time{Time: attachmentRepairTestNow.Add(-time.Minute)}
	cl := newAttachmentRepairTestClient(fixture.objects...)
	reconcileAttachmentRepairOperation(t, fixture.operationReconciler(cl, true), fixture.operation)
	status := attachmentRepairObjectStatus(t, cl, fixture.operation, newStorageAttachmentRepairOperationObject())
	if status["phase"] != "Blocked" || !hasConditionReason(status["conditions"].([]any), "Ready", "TerminatingPodUsesClaim") {
		t.Fatalf("status = %#v, want terminating Pod block", status)
	}
}

func TestAttachmentRepairBlocksPreexistingAttachmentDeletion(t *testing.T) {
	fixture := newAttachmentRepairFixture(false)
	fixture.stale.Finalizers = []string{"storage.csi.example.test/external-attacher"}
	fixture.stale.DeletionTimestamp = &metav1.Time{Time: attachmentRepairTestNow.Add(-time.Minute)}
	cl := newAttachmentRepairTestClient(fixture.objects...)
	reconcileAttachmentRepairOperation(t, fixture.operationReconciler(cl, true), fixture.operation)
	status := attachmentRepairObjectStatus(t, cl, fixture.operation, newStorageAttachmentRepairOperationObject())
	if status["phase"] != "Blocked" || !hasConditionReason(status["conditions"].([]any), "Ready", "AttachmentDeletionInProgress") {
		t.Fatalf("status = %#v, want preexisting deletion block", status)
	}
}

func TestAttachmentRepairBlocksMultipleAttachments(t *testing.T) {
	fixture := newAttachmentRepairFixture(false)
	fixture.objects = append(fixture.objects, attachmentRepairVolumeAttachment("va-extra-private", "va-extra-uid", fixture.pv.Name, "other-node-private", true))
	cl := newAttachmentRepairTestClient(fixture.objects...)
	reconcileAttachmentRepairOperation(t, fixture.operationReconciler(cl, true), fixture.operation)
	status := attachmentRepairObjectStatus(t, cl, fixture.operation, newStorageAttachmentRepairOperationObject())
	if status["phase"] != "Blocked" || !hasConditionReason(status["conditions"].([]any), "Ready", "VolumeAttachmentSetNotUnique") {
		t.Fatalf("status = %#v, want unique attachment block", status)
	}
}

func TestAttachmentRepairBlocksRecreatedAttachmentAfterAuthorization(t *testing.T) {
	fixture := newAttachmentRepairFixture(false)
	fixture.addExactApproval(t)
	baseClient := newAttachmentRepairTestClient(fixture.objects...)
	recording := &attachmentRepairDeleteRecordingClient{Client: baseClient}
	reconcileAttachmentRepairApproval(t, fixture.approvalReconciler(recording), fixture.approval)
	reconcileAttachmentRepairOperation(t, fixture.operationReconciler(recording, true), fixture.operation)

	stale := &storagev1.VolumeAttachment{}
	mustGetAttachmentRepairObject(t, recording, fixture.stale, stale)
	if err := recording.Client.Delete(context.Background(), stale); err != nil {
		t.Fatalf("delete original stale attachment: %v", err)
	}
	recreated := attachmentRepairVolumeAttachment(fixture.stale.Name, "recreated-stale-uid", fixture.pv.Name, fixture.staleNode.Name, true)
	recreated.ResourceVersion = ""
	if err := recording.Create(context.Background(), recreated); err != nil {
		t.Fatalf("create replacement: %v", err)
	}
	reconcileAttachmentRepairOperation(t, fixture.operationReconciler(recording, true), fixture.operation)
	status := attachmentRepairObjectStatus(t, recording, fixture.operation, newStorageAttachmentRepairOperationObject())
	if status["phase"] != "Blocked" || recording.deleteCalls != 0 {
		t.Fatalf("status=%#v deleteCalls=%d, want recreated attachment block", status, recording.deleteCalls)
	}
}

func TestAttachmentRepairWaitsWithoutRemovingFinalizers(t *testing.T) {
	fixture := newAttachmentRepairFixture(false)
	fixture.stale.Finalizers = []string{"storage.csi.example.test/external-attacher"}
	fixture.addExactApproval(t)
	baseClient := newAttachmentRepairTestClient(fixture.objects...)
	recording := &attachmentRepairDeleteRecordingClient{Client: baseClient}
	operationReconciler := fixture.operationReconciler(recording, true)
	reconcileAttachmentRepairApproval(t, fixture.approvalReconciler(recording), fixture.approval)
	reconcileAttachmentRepairOperation(t, operationReconciler, fixture.operation)
	reconcileAttachmentRepairOperation(t, operationReconciler, fixture.operation)
	reconcileAttachmentRepairOperation(t, operationReconciler, fixture.operation)
	status := attachmentRepairObjectStatus(t, recording, fixture.operation, newStorageAttachmentRepairOperationObject())
	if status["phase"] != "Running" || recording.deleteCalls != 1 {
		t.Fatalf("status=%#v deleteCalls=%d, want idempotent convergence", status, recording.deleteCalls)
	}
	stale := &storagev1.VolumeAttachment{}
	mustGetAttachmentRepairObject(t, recording, fixture.stale, stale)
	if stale.DeletionTimestamp == nil || len(stale.Finalizers) != 1 {
		t.Fatalf("stale metadata = %#v, want finalizer preserved", stale.ObjectMeta)
	}
}

func TestAttachmentRepairResumesWhenDeleteCompletesBeforeStatusPatch(t *testing.T) {
	fixture := newAttachmentRepairFixture(false)
	fixture.addExactApproval(t)
	cl := newAttachmentRepairTestClient(fixture.objects...)
	operationReconciler := fixture.operationReconciler(cl, true)
	approvalReconciler := fixture.approvalReconciler(cl)
	reconcileAttachmentRepairApproval(t, approvalReconciler, fixture.approval)
	reconcileAttachmentRepairOperation(t, operationReconciler, fixture.operation)
	reconcileAttachmentRepairOperation(t, operationReconciler, fixture.operation)

	stale := &storagev1.VolumeAttachment{}
	mustGetAttachmentRepairObject(t, cl, fixture.stale, stale)
	if err := cl.Delete(context.Background(), stale); err != nil {
		t.Fatalf("simulate completed committed delete: %v", err)
	}
	reconcileAttachmentRepairApproval(t, approvalReconciler, fixture.approval)
	approvalStatus := attachmentRepairObjectStatus(t, cl, fixture.approval, newStorageAttachmentRepairApprovalObject())
	if approvalStatus["phase"] != "Accepted" {
		t.Fatalf("approval status = %#v, want accepted committed evidence", approvalStatus)
	}
	reconcileAttachmentRepairOperation(t, operationReconciler, fixture.operation)
	status := attachmentRepairObjectStatus(t, cl, fixture.operation, newStorageAttachmentRepairOperationObject())
	if status["phase"] != "Running" || status["currentStage"] != "WaitForConvergence" {
		t.Fatalf("status = %#v, want resumed convergence", status)
	}
}

func TestAttachmentRepairMutationEventIsRedactedAndCorrelated(t *testing.T) {
	operation := newStorageAttachmentRepairOperationObject()
	status := map[string]any{"phase": "Running", "currentStage": "WaitForConvergence", "result": "Exact stale attachment deletion was requested"}
	operation.Object["status"] = map[string]any{"execution": map[string]any{"auditCorrelationID": "audit-attachment-repair-001"}}
	eventType, reason, message := storageAttachmentRepairOperationEvent(operation, status)
	if eventType != corev1.EventTypeNormal || reason != "StorageAttachmentRepairDeletionRequested" || !strings.Contains(message, "auditCorrelationID=audit-attachment-repair-001") {
		t.Fatalf("event = (%q, %q, %q)", eventType, reason, message)
	}
}

func TestAttachmentRepairApprovalExpiryEventIsRedactedAndCorrelated(t *testing.T) {
	approval := newStorageAttachmentRepairApprovalObject()
	approval.Object["spec"] = map[string]any{"auditCorrelationID": "audit-attachment-repair-001"}
	status := map[string]any{"phase": "Expired", "message": "Attachment repair approval expired before new authorization"}
	eventType, reason, message := storageAttachmentRepairApprovalEvent(approval, status)
	if eventType != corev1.EventTypeWarning || reason != "StorageAttachmentRepairApprovalExpired" || !strings.Contains(message, "auditCorrelationID=audit-attachment-repair-001") {
		t.Fatalf("event = (%q, %q, %q)", eventType, reason, message)
	}
}

type attachmentRepairFixture struct {
	operation   *unstructured.Unstructured
	approval    *unstructured.Unstructured
	capability  *unstructured.Unstructured
	pvc         *corev1.PersistentVolumeClaim
	pv          *corev1.PersistentVolume
	pod         *corev1.Pod
	currentNode *corev1.Node
	staleNode   *corev1.Node
	current     *storagev1.VolumeAttachment
	stale       *storagev1.VolumeAttachment
	objects     []client.Object
}

func newAttachmentRepairFixture(dryRun bool) *attachmentRepairFixture {
	storageClass := defaultRookStorageClassName
	operation := newStorageAttachmentRepairOperationObject()
	operation.SetNamespace("storage-system")
	operation.SetName("repair-operation")
	operation.SetUID("repair-operation-uid")
	operation.SetResourceVersion("1")
	operation.SetGeneration(1)
	spec := map[string]any{
		"workflowChannel":          "W",
		"targetBinding":            attachmentRepairTestControlPlaneTargetBinding(),
		"storageCapabilityRef":     map[string]any{"name": "rook-ceph"},
		"persistentVolumeClaimRef": map[string]any{"namespace": "appthrust-system", "name": "postgres-data-private"},
		"dryRun":                   dryRun,
	}
	if !dryRun {
		spec["approvalRef"] = map[string]any{"name": "repair-approval"}
	}
	operation.Object["spec"] = spec
	capability := newProviderRecoveryCapability("storage-system", "rook-ceph", true)
	capability.SetUID("capability-uid")
	capability.SetResourceVersion("1")
	pvc := &corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{Namespace: "appthrust-system", Name: "postgres-data-private", UID: "pvc-private-uid", ResourceVersion: "1"},
		Spec:       corev1.PersistentVolumeClaimSpec{StorageClassName: &storageClass, VolumeName: "pv-private"},
		Status:     corev1.PersistentVolumeClaimStatus{Phase: corev1.ClaimBound},
	}
	pv := &corev1.PersistentVolume{
		ObjectMeta: metav1.ObjectMeta{Name: "pv-private", UID: "pv-private-uid", ResourceVersion: "1"},
		Spec: corev1.PersistentVolumeSpec{
			StorageClassName: storageClass,
			ClaimRef:         &corev1.ObjectReference{Namespace: pvc.Namespace, Name: pvc.Name, UID: pvc.UID},
		},
		Status: corev1.PersistentVolumeStatus{Phase: corev1.VolumeBound},
	}
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Namespace: pvc.Namespace, Name: "postgres-private", UID: "pod-private-uid", ResourceVersion: "1"},
		Spec: corev1.PodSpec{
			NodeName: "current-node-private",
			Volumes:  []corev1.Volume{{Name: "data", VolumeSource: corev1.VolumeSource{PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{ClaimName: pvc.Name}}}},
		},
		Status: corev1.PodStatus{Phase: corev1.PodPending},
	}
	currentNode := attachmentRepairNode("current-node-private", "current-node-uid", corev1.ConditionTrue)
	staleNode := attachmentRepairNode("stale-node-private", "stale-node-uid", corev1.ConditionFalse)
	current := attachmentRepairVolumeAttachment("va-current-private", "va-current-uid", pv.Name, currentNode.Name, false)
	stale := attachmentRepairVolumeAttachment("va-stale-private", "va-stale-uid", pv.Name, staleNode.Name, true)
	objects := []client.Object{operation, capability, pvc, pv, pod, currentNode, staleNode, current, stale}
	return &attachmentRepairFixture{
		operation: operation, capability: capability, pvc: pvc, pv: pv, pod: pod,
		currentNode: currentNode, staleNode: staleNode, current: current, stale: stale, objects: objects,
	}
}

func (f *attachmentRepairFixture) addExactApproval(t *testing.T) {
	t.Helper()
	observer := f.operationReconciler(newAttachmentRepairTestClient(f.objects...), false)
	obs, reason, message := observer.observeInitialSafety(context.Background(), f.operation)
	if reason != "" {
		t.Fatalf("observe fixture: %s: %s", reason, message)
	}
	approval := newStorageAttachmentRepairApprovalObject()
	approval.SetNamespace(f.operation.GetNamespace())
	approval.SetName("repair-approval")
	approval.SetUID("repair-approval-uid")
	approval.SetResourceVersion("1")
	approval.SetGeneration(1)
	approval.Object["spec"] = map[string]any{
		"workflowChannel": "W",
		"targetBinding":   attachmentRepairTestControlPlaneTargetBinding(),
		"operationRef": map[string]any{
			"name": f.operation.GetName(), "uid": string(f.operation.GetUID()), "generation": f.operation.GetGeneration(),
		},
		"observationDigest":  obs.digest,
		"approvedBy":         "operator@example.test",
		"approvedAt":         attachmentRepairTestNow.Add(-time.Minute).Format(time.RFC3339),
		"expiresAt":          attachmentRepairTestNow.Add(29 * time.Minute).Format(time.RFC3339),
		"reason":             "Repair reviewed stale attachment",
		"auditCorrelationID": "audit-attachment-repair-001",
	}
	f.approval = approval
	f.objects = append(f.objects, approval)
}

func attachmentRepairTestControlPlaneTargetBinding() map[string]any {
	return map[string]any{
		"productCluster": map[string]any{
			"group": "runtime.appthrust.io", "kind": "ClusterDeployment", "namespace": "mgmt",
			"name": "wlc-next", "uid": "deployment-uid", "generation": int64(1),
		},
	}
}

func (f *attachmentRepairFixture) removeObject(target client.Object) {
	out := make([]client.Object, 0, len(f.objects))
	for _, object := range f.objects {
		if object.GetObjectKind().GroupVersionKind() == target.GetObjectKind().GroupVersionKind() && client.ObjectKeyFromObject(object) == client.ObjectKeyFromObject(target) {
			continue
		}
		if fmt.Sprintf("%T", object) == fmt.Sprintf("%T", target) && client.ObjectKeyFromObject(object) == client.ObjectKeyFromObject(target) {
			continue
		}
		out = append(out, object)
	}
	f.objects = out
}

func (f *attachmentRepairFixture) operationReconciler(cl client.Client, execution bool) *AttachmentRepairOperationReconciler {
	return f.operationReconcilerAt(cl, execution, func() time.Time { return attachmentRepairTestNow })
}

func (f *attachmentRepairFixture) operationReconcilerAt(cl client.Client, execution bool, now func() time.Time) *AttachmentRepairOperationReconciler {
	return &AttachmentRepairOperationReconciler{Client: cl, FeatureGates: StorageFeatureGates{AttachmentRepairExecution: execution}, Now: now}
}

func (f *attachmentRepairFixture) approvalReconciler(cl client.Client) *AttachmentRepairApprovalReconciler {
	return f.approvalReconcilerAt(cl, func() time.Time { return attachmentRepairTestNow })
}

func (f *attachmentRepairFixture) approvalReconcilerAt(cl client.Client, now func() time.Time) *AttachmentRepairApprovalReconciler {
	return &AttachmentRepairApprovalReconciler{Client: cl, Now: now}
}

func attachmentRepairNode(name string, uid types.UID, ready corev1.ConditionStatus) *corev1.Node {
	return &corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: name, UID: uid, ResourceVersion: "1"}, Status: corev1.NodeStatus{Conditions: []corev1.NodeCondition{{Type: corev1.NodeReady, Status: ready}}}}
}

func attachmentRepairVolumeAttachment(name string, uid types.UID, pvName, nodeName string, attached bool) *storagev1.VolumeAttachment {
	return &storagev1.VolumeAttachment{
		ObjectMeta: metav1.ObjectMeta{Name: name, UID: uid, ResourceVersion: "1"},
		Spec:       storagev1.VolumeAttachmentSpec{Attacher: "rook-ceph.rbd.csi.ceph.com", NodeName: nodeName, Source: storagev1.VolumeAttachmentSource{PersistentVolumeName: &pvName}},
		Status:     storagev1.VolumeAttachmentStatus{Attached: attached},
	}
}

func newAttachmentRepairTestClient(objects ...client.Object) client.Client {
	scheme := kruntime.NewScheme()
	_ = corev1.AddToScheme(scheme)
	_ = storagev1.AddToScheme(scheme)
	for _, gvk := range []schema.GroupVersionKind{storageCapabilityGVK, storageAttachmentRepairOperationGVK, storageAttachmentRepairApprovalGVK} {
		scheme.AddKnownTypeWithName(gvk, &unstructured.Unstructured{})
		scheme.AddKnownTypeWithName(gvk.GroupVersion().WithKind(gvk.Kind+"List"), &unstructured.UnstructuredList{})
	}
	return fake.NewClientBuilder().WithScheme(scheme).WithObjects(objects...).WithStatusSubresource(objects...).Build()
}

func reconcileAttachmentRepairOperation(t *testing.T, reconciler *AttachmentRepairOperationReconciler, operation *unstructured.Unstructured) ctrl.Result {
	t.Helper()
	result, err := reconciler.Reconcile(context.Background(), ctrl.Request{NamespacedName: client.ObjectKeyFromObject(operation)})
	if err != nil {
		t.Fatalf("operation Reconcile: %v", err)
	}
	return result
}

func reconcileAttachmentRepairApproval(t *testing.T, reconciler *AttachmentRepairApprovalReconciler, approval *unstructured.Unstructured) ctrl.Result {
	t.Helper()
	result, err := reconciler.Reconcile(context.Background(), ctrl.Request{NamespacedName: client.ObjectKeyFromObject(approval)})
	if err != nil {
		t.Fatalf("approval Reconcile: %v", err)
	}
	return result
}

func attachmentRepairObjectStatus(t *testing.T, cl client.Client, source, target *unstructured.Unstructured) map[string]any {
	t.Helper()
	if err := cl.Get(context.Background(), client.ObjectKeyFromObject(source), target); err != nil {
		t.Fatalf("get %s: %v", source.GetKind(), err)
	}
	status, found, err := unstructured.NestedMap(target.Object, "status")
	if err != nil || !found {
		t.Fatalf("status missing: found=%v err=%v object=%#v", found, err, target.Object)
	}
	return status
}

func mustGetAttachmentRepairObject(t *testing.T, cl client.Client, source, target client.Object) {
	t.Helper()
	if err := cl.Get(context.Background(), client.ObjectKeyFromObject(source), target); err != nil {
		t.Fatalf("get %T: %v", target, err)
	}
}

func assertAttachmentRepairStatusRedacted(t *testing.T, status map[string]any, fixture *attachmentRepairFixture) {
	t.Helper()
	rendered := fmt.Sprintf("%#v", status)
	for _, forbidden := range []string{
		fixture.pvc.Name, string(fixture.pvc.UID), fixture.pv.Name, string(fixture.pv.UID),
		fixture.stale.Name, string(fixture.stale.UID), fixture.stale.Spec.NodeName,
		fixture.current.Name, string(fixture.current.UID), fixture.current.Spec.NodeName,
	} {
		if strings.Contains(rendered, forbidden) {
			t.Fatalf("status exposed target detail %q: %s", forbidden, rendered)
		}
	}
}

func assertApprovalStatusRedacted(t *testing.T, status map[string]any, fixture *attachmentRepairFixture) {
	t.Helper()
	rendered := fmt.Sprintf("%#v", status)
	for _, forbidden := range []string{fixture.pv.Name, fixture.stale.Name, string(fixture.stale.UID), fixture.staleNode.Name} {
		if strings.Contains(rendered, forbidden) {
			t.Fatalf("approval status exposed target detail %q: %s", forbidden, rendered)
		}
	}
}
