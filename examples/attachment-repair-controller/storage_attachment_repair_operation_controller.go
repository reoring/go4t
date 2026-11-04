package storage

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"
	"strings"
	"time"

	"attachrepairctl/decide"

	"github.com/reoring/go4t/rt"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	storagev1 "k8s.io/api/storage/v1"
	apiequality "k8s.io/apimachinery/pkg/api/equality"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

const (
	attachmentRepairRequeueInterval = 20 * time.Second
	attachmentRepairApprovalMaxAge  = time.Hour
)

var (
	storageAttachmentRepairOperationGVK = schema.GroupVersionKind{
		Group: StorageCapabilityGroup, Version: StorageCapabilityVersion, Kind: "StorageAttachmentRepairOperation",
	}
	storageAttachmentRepairApprovalGVK = schema.GroupVersionKind{
		Group: StorageCapabilityGroup, Version: StorageCapabilityVersion, Kind: "StorageAttachmentRepairApproval",
	}
)

type AttachmentRepairOperationReconciler struct {
	Client       client.Client
	Log          logr.Logger
	Recorder     record.EventRecorder
	FeatureGates StorageFeatureGates
	Now          func() time.Time
}

type AttachmentRepairApprovalReconciler struct {
	Client   client.Client
	Log      logr.Logger
	Recorder record.EventRecorder
	Now      func() time.Time
}

type attachmentRepairObservation struct {
	capability        *unstructured.Unstructured
	pvc               *corev1.PersistentVolumeClaim
	pv                *corev1.PersistentVolume
	pods              []corev1.Pod
	currentNode       *corev1.Node
	staleNode         *corev1.Node
	currentAttachment *storagev1.VolumeAttachment
	staleAttachment   *storagev1.VolumeAttachment
	attachments       []storagev1.VolumeAttachment
	digest            string
	safety            map[string]any
}

type obsBlock struct {
	reason  decide.ObsReason
	message string
}

func (r *AttachmentRepairOperationReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx, "storageAttachmentRepairOperation", req.NamespacedName)
	if r.Log.GetSink() != nil {
		logger = r.Log.WithValues("storageAttachmentRepairOperation", req.NamespacedName)
	}
	operation := newStorageAttachmentRepairOperationObject()
	if err := r.Client.Get(ctx, req.NamespacedName, operation); err != nil {
		if apierrors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, fmt.Errorf("get StorageAttachmentRepairOperation: %w", err)
	}
	if !operation.GetDeletionTimestamp().IsZero() || attachmentRepairTerminal(operation) {
		return ctrl.Result{}, nil
	}

	oldPhase := firstStatusString(operation, "status", "phase")
	oldStage := firstStatusString(operation, "status", "currentStage")
	status, requeueAfter := r.reconcileOperation(ctx, operation)
	if err := patchUnstructuredStatus(ctx, r.Client, operation, status); err != nil {
		return ctrl.Result{}, err
	}
	if oldPhase != fmt.Sprint(status["phase"]) || oldStage != fmt.Sprint(status["currentStage"]) {
		eventType, reason, message := storageAttachmentRepairOperationEvent(operation, status)
		recordStorageWorkflowEvent(r.Recorder, operation, eventType, reason, message)
	}
	logger.V(1).Info("published storage attachment repair operation status")
	return ctrl.Result{RequeueAfter: requeueAfter}, nil
}

func (r *AttachmentRepairOperationReconciler) reconcileOperation(ctx context.Context, operation *unstructured.Unstructured) (map[string]any, time.Duration) {
	mode, approval := r.readMode(ctx, operation)
	o := decide.Observed{
		Mode:                      mode,
		FeatureGateEnabled:        r.FeatureGates.AttachmentRepairExecution,
		OperationGeneration:       operation.GetGeneration(),
		Authorized:                nestedStatusBool(operation, "execution", "deletionAuthorized"),
		Committed:                 nestedStatusBool(operation, "execution", "deletionCommitted"),
		Requested:                 nestedStatusBool(operation, "execution", "deletionRequested"),
		ApprovedObservationDigest: firstStatusString(operation, "status", "execution", "approvedObservationDigest"),
		NowNanos:                  r.now().UnixNano(),
		ApprovalMaxAgeNanos:        int64(attachmentRepairApprovalMaxAge),
	}
	var initialObs, authorizedObs *attachmentRepairObservation
	for {
		switch a := decide.Decide(o).(type) {
		case decide.ActionObserveInitial:
			if o.Initial.Present {
				panic("decide asked twice")
			}
			var block *obsBlock
			initialObs, block = r.observeInitialTyped(ctx, operation)
			var observation decide.Observation
			if block != nil {
				observation = decide.ObservationObsBlocked{F0: block.reason, F1: block.message}
			} else {
				observation = decide.ObservationObsOk{F0: initialObs.digest}
			}
			o.Initial = rt.Option[decide.Observation]{Present: true, Value: observation}
		case decide.ActionObserveAuthorized:
			if o.AuthorizedObs.Present {
				panic("decide asked twice")
			}
			var target decide.Target
			var block *obsBlock
			authorizedObs, target, block = r.observeAuthorizedSafety(ctx, operation, o.Requested)
			var observation decide.AuthorizedObservation
			if block != nil {
				observation = decide.AuthorizedObservationAuthBlocked{F0: block.reason, F1: block.message}
			} else {
				staleIdentity := ""
				if authorizedObs.staleAttachment != nil {
					staleIdentity = attachmentRepairObjectIdentityDigest("volume-attachment-v1", authorizedObs.staleAttachment)
				}
				observation = decide.AuthorizedObservationAuthOk{F0: decide.AuthOkObs{
					Target:          target,
					Digest:          authorizedObs.digest,
					CurrentAttached: authorizedObs.currentAttachment != nil && authorizedObs.currentAttachment.Status.Attached,
					StaleIdentity:   staleIdentity,
				}}
			}
			o.AuthorizedObs = rt.Option[decide.AuthorizedObservation]{Present: true, Value: observation}
		case decide.ActionDeleteStale:
			if o.DeleteResult.Present {
				panic("decide asked twice")
			}
			staleAttachment := observedStaleAttachment(authorizedObs, a.F0)
			uid := staleAttachment.GetUID()
			resourceVersion := staleAttachment.GetResourceVersion()
			err := r.Client.Delete(ctx, staleAttachment, client.Preconditions{UID: &uid, ResourceVersion: &resourceVersion})
			result := decide.DeleteResultDeleted
			switch {
			case apierrors.IsNotFound(err) || apierrors.IsConflict(err) || apierrors.IsInvalid(err):
				result = decide.DeleteResultCasFailed
			case err != nil:
				result = decide.DeleteResultDeleteFailed
			}
			o.DeleteResult = rt.Option[decide.DeleteResult]{Present: true, Value: result}
		case decide.ActionPublish:
			if authorizedObs != nil {
				return r.publish(operation, a.F0, authorizedObs, approval)
			}
			return r.publish(operation, a.F0, initialObs, approval)
		default:
			panic("unknown decide action")
		}
	}
}

func observedStaleAttachment(authorizedObs *attachmentRepairObservation, identity string) *storagev1.VolumeAttachment {
	if authorizedObs == nil || authorizedObs.staleAttachment == nil || attachmentRepairObjectIdentityDigest("volume-attachment-v1", authorizedObs.staleAttachment) != identity {
		panic("decide named a stale attachment that was not observed")
	}
	return authorizedObs.staleAttachment
}

func (r *AttachmentRepairOperationReconciler) publish(operation *unstructured.Unstructured, status decide.Status, obs *attachmentRepairObservation, approval *unstructured.Unstructured) (map[string]any, time.Duration) {
	switch s := status.(type) {
	case decide.StatusBlocked:
		if s.ApprovalMatches {
			obs.safety["approvalMatchesObservation"] = true
		}
		return r.blockedStatus(operation, s.Stage, reasonString(s.Reason), s.Message, obs, approval), attachmentRepairRequeueInterval
	case decide.StatusRunning:
		return r.runningStatus(operation, s.F0, s.F1, obs, approval, s.F2), attachmentRepairRequeueInterval
	case decide.StatusSucceeded:
		return r.succeededStatus(operation, s.F0, s.F1, obs, approval, s.F2), 0
	case decide.StatusAwaitingApproval:
		return r.awaitingApprovalStatus(operation, obs), attachmentRepairRequeueInterval
	case decide.StatusAuthorized:
		obs.safety["approvalMatchesObservation"] = true
		return r.authorizedStatus(operation, obs, approval), 0
	case decide.StatusCommitted:
		return r.committedStatus(operation, obs, approval), 0
	default:
		panic("unknown decide status")
	}
}

func reasonString(reason decide.ReasonCode) string {
	switch r := reason.(type) {
	case decide.ReasonCodeDecided:
		return r.F0.String()
	case decide.ReasonCodeFromObservation:
		return r.F0.String()
	default:
		panic("unknown decide reason")
	}
}

func (r *AttachmentRepairOperationReconciler) committedStatus(operation *unstructured.Unstructured, obs *attachmentRepairObservation, approval *unstructured.Unstructured) map[string]any {
	status := r.baseStatus(operation)
	status["phase"] = "Running"
	status["currentStage"] = "CommitExactDeletion"
	status["lastSafetyCheck"] = r.now().Format(time.RFC3339)
	status["observationDigest"] = obs.digest
	status["safety"] = obs.safety
	execution := attachmentRepairExecutionStatus(operation, obs, approval, true, false)
	execution["deletionCommitted"] = true
	status["execution"] = execution
	status["result"] = "Exact attachment deletion commitment was recorded and will be revalidated before mutation"
	status["nextAction"] = "Wait for controller-owned compare-and-swap deletion."
	status["operatorDiagnosticsAvailable"] = true
	conditions := existingConditions(operation)
	setCondition(&conditions, condition(operation, "Ready", metav1.ConditionFalse, "OperationRunning", "Exact attachment deletion is committed"))
	status["conditions"] = conditionsToUnstructured(conditions)
	return status
}

func (r *AttachmentRepairOperationReconciler) readMode(ctx context.Context, operation *unstructured.Unstructured) (decide.Mode, *unstructured.Unstructured) {
	if attachmentRepairDryRun(operation) {
		return decide.ModeDryRun{}, nil
	}
	approvalName := firstNonEmptyNestedString(operation.Object, []string{"spec", "approvalRef", "name"})
	if approvalName == "" {
		return decide.ModeLive{F0: decide.ApprovalReadNoRef{}}, nil
	}
	approval := newStorageAttachmentRepairApprovalObject()
	if err := r.Client.Get(ctx, client.ObjectKey{Namespace: operation.GetNamespace(), Name: approvalName}, approval); err != nil {
		if apierrors.IsNotFound(err) {
			return decide.ModeLive{F0: decide.ApprovalReadMissing{}}, nil
		}
		return decide.ModeLive{F0: decide.ApprovalReadReadFailed{}}, nil
	}
	return decide.ModeLive{F0: decide.ApprovalReadRead{F0: projectApproval(operation, approval)}}, approval
}

func projectApproval(operation, approval *unstructured.Unstructured) decide.Approval {
	generation, generationFound := nestedAttachmentRepairInt64(approval, "spec", "operationRef", "generation")
	operationTarget, operationTargetFound, operationTargetErr := unstructured.NestedMap(operation.Object, "spec", "targetBinding")
	approvalTarget, approvalTargetFound, approvalTargetErr := unstructured.NestedMap(approval.Object, "spec", "targetBinding")
	digest := nestedAttachmentRepairString(approval, "spec", "observationDigest")
	approvedBy := nestedAttachmentRepairString(approval, "spec", "approvedBy")
	reason := nestedAttachmentRepairString(approval, "spec", "reason")
	auditID := nestedAttachmentRepairString(approval, "spec", "auditCorrelationID")
	approvedAtRaw := nestedAttachmentRepairString(approval, "spec", "approvedAt")
	expiresAtRaw := nestedAttachmentRepairString(approval, "spec", "expiresAt")
	return decide.Approval{
		Deleting:   approval.GetDeletionTimestamp() != nil,
		RefMatches: firstNonEmptyNestedString(operation.Object, []string{"spec", "approvalRef", "name"}) == approval.GetName(),
		OpIdentityMatches: nestedAttachmentRepairString(approval, "spec", "operationRef", "name") == operation.GetName() &&
			types.UID(nestedAttachmentRepairString(approval, "spec", "operationRef", "uid")) == operation.GetUID(),
		Generation: rt.Option[int64]{Present: generationFound, Value: generation},
		TargetBindingMatches: operationTargetErr == nil && approvalTargetErr == nil && operationTargetFound && approvalTargetFound &&
			apiequality.Semantic.DeepEqual(operationTarget, approvalTarget),
		DigestValid:      validAttachmentRepairDigest(digest),
		EvidenceComplete: approvedBy != "" && reason != "" && auditID != "" && approvedAtRaw != "" && expiresAtRaw != "",
		ApprovedAt:       projectApprovalTimestamp(approvedAtRaw),
		ExpiresAt:        projectApprovalTimestamp(expiresAtRaw),
		Phase:            firstStatusString(approval, "status", "phase"),
		Digest:           digest,
	}
}

func projectApprovalTimestamp(raw string) decide.Timestamp {
	if raw == "" {
		return decide.TimestampNoTimestamp{}
	}
	value, err := parseAttachmentRepairApprovalTime(raw)
	if err != nil || value.IsZero() {
		return decide.TimestampInvalidTimestamp{}
	}
	return decide.TimestampAtNanos{F0: value.UnixNano()}
}

func validateAttachmentRepairApprovalBinding(operation, approval *unstructured.Unstructured, now time.Time) (string, string) {
	if approval.GetDeletionTimestamp() != nil {
		return "ApprovalDeletionInProgress", "Attachment repair approval is deleting"
	}
	if firstNonEmptyNestedString(operation.Object, []string{"spec", "approvalRef", "name"}) != approval.GetName() {
		return "ApprovalReferenceMismatch", "Operation does not reference this attachment repair approval"
	}
	if nestedAttachmentRepairString(approval, "spec", "operationRef", "name") != operation.GetName() ||
		types.UID(nestedAttachmentRepairString(approval, "spec", "operationRef", "uid")) != operation.GetUID() {
		return "ApprovalOperationIdentityMismatch", "Approval does not reference the exact operation UID"
	}
	generation, ok := nestedAttachmentRepairInt64(approval, "spec", "operationRef", "generation")
	if !ok || generation != operation.GetGeneration() {
		return "ApprovalOperationGenerationMismatch", "Approval does not reference the exact operation generation"
	}
	operationTarget, operationTargetFound, operationTargetErr := unstructured.NestedMap(operation.Object, "spec", "targetBinding")
	approvalTarget, approvalTargetFound, approvalTargetErr := unstructured.NestedMap(approval.Object, "spec", "targetBinding")
	if operationTargetErr != nil || approvalTargetErr != nil || !operationTargetFound || !approvalTargetFound ||
		!apiequality.Semantic.DeepEqual(operationTarget, approvalTarget) {
		return "ApprovalTargetBindingMismatch", "Approval does not reference the exact operation target binding"
	}
	digest := nestedAttachmentRepairString(approval, "spec", "observationDigest")
	approvedBy := nestedAttachmentRepairString(approval, "spec", "approvedBy")
	reason := nestedAttachmentRepairString(approval, "spec", "reason")
	auditID := nestedAttachmentRepairString(approval, "spec", "auditCorrelationID")
	approvedAtRaw := nestedAttachmentRepairString(approval, "spec", "approvedAt")
	expiresAtRaw := nestedAttachmentRepairString(approval, "spec", "expiresAt")
	if !validAttachmentRepairDigest(digest) || approvedBy == "" || reason == "" || auditID == "" || approvedAtRaw == "" || expiresAtRaw == "" {
		return "ApprovalEvidenceIncomplete", "Attachment repair approval evidence is incomplete"
	}
	approvedAt, err := parseAttachmentRepairApprovalTime(approvedAtRaw)
	if err != nil || approvedAt.IsZero() || approvedAt.After(now) {
		return "ApprovalTimeInvalid", "Attachment repair approval time is invalid"
	}
	expiresAt, err := parseAttachmentRepairApprovalTime(expiresAtRaw)
	if err != nil || expiresAt.IsZero() || !expiresAt.After(approvedAt) || expiresAt.Sub(approvedAt) > attachmentRepairApprovalMaxAge {
		return "ApprovalWindowInvalid", "Attachment repair approval window is invalid"
	}
	return "", ""
}

func parseAttachmentRepairApprovalTime(raw string) (time.Time, error) {
	value, err := time.Parse(time.RFC3339Nano, raw)
	if err != nil {
		return time.Time{}, err
	}
	return value, nil
}

func attachmentRepairApprovalExpiresAt(approval *unstructured.Unstructured) (time.Time, bool) {
	expiresAt, err := parseAttachmentRepairApprovalTime(nestedAttachmentRepairString(approval, "spec", "expiresAt"))
	return expiresAt, err == nil && !expiresAt.IsZero()
}

func (r *AttachmentRepairOperationReconciler) observeInitialSafety(ctx context.Context, operation *unstructured.Unstructured) (*attachmentRepairObservation, string, string) {
	obs, block := r.observeInitialTyped(ctx, operation)
	if block != nil {
		return obs, block.reason.String(), block.message
	}
	return obs, "", ""
}

func (r *AttachmentRepairOperationReconciler) observeInitialTyped(ctx context.Context, operation *unstructured.Unstructured) (*attachmentRepairObservation, *obsBlock) {
	obs, block := r.observeBase(ctx, operation)
	if block != nil {
		return obs, block
	}
	if block = r.classifyInitialAttachments(ctx, obs); block != nil {
		return obs, block
	}
	obs.digest = attachmentRepairObservationDigest(obs)
	obs.safety["repairAdmitted"] = true
	return obs, nil
}

func (r *AttachmentRepairOperationReconciler) observeBase(ctx context.Context, operation *unstructured.Unstructured) (*attachmentRepairObservation, *obsBlock) {
	obs := &attachmentRepairObservation{safety: attachmentRepairSafetyStatus()}
	capabilityName := firstNonEmptyNestedString(operation.Object, []string{"spec", "storageCapabilityRef", "name"})
	pvcNamespace := firstNonEmptyNestedString(operation.Object, []string{"spec", "persistentVolumeClaimRef", "namespace"})
	pvcName := firstNonEmptyNestedString(operation.Object, []string{"spec", "persistentVolumeClaimRef", "name"})
	if capabilityName == "" || pvcNamespace == "" || pvcName == "" {
		return obs, &obsBlock{decide.ObsReasonTargetReferenceMissing, "StorageCapability and PersistentVolumeClaim references are required"}
	}
	obs.capability = newStorageCapabilityObject()
	if err := r.Client.Get(ctx, client.ObjectKey{Namespace: operation.GetNamespace(), Name: capabilityName}, obs.capability); err != nil {
		if apierrors.IsNotFound(err) {
			return obs, &obsBlock{decide.ObsReasonStorageCapabilityNotFound, "Referenced StorageCapability was not found"}
		}
		return obs, &obsBlock{decide.ObsReasonStorageCapabilityReadFailed, "Referenced StorageCapability could not be read"}
	}
	if !obs.capability.GetDeletionTimestamp().IsZero() {
		return obs, &obsBlock{decide.ObsReasonStorageCapabilityDeletionInProgress, "Referenced StorageCapability is deleting"}
	}
	capabilitySpec := parseSpec(obs.capability)
	if capabilitySpec.storageClassName == "" {
		return obs, &obsBlock{decide.ObsReasonStorageClassMissing, "Referenced StorageCapability does not publish a block StorageClass"}
	}

	obs.pvc = &corev1.PersistentVolumeClaim{}
	if err := r.Client.Get(ctx, client.ObjectKey{Namespace: pvcNamespace, Name: pvcName}, obs.pvc); err != nil {
		if apierrors.IsNotFound(err) {
			return obs, &obsBlock{decide.ObsReasonPersistentVolumeClaimNotFound, "Target PersistentVolumeClaim was not found"}
		}
		return obs, &obsBlock{decide.ObsReasonPersistentVolumeClaimReadFailed, "Target PersistentVolumeClaim could not be read"}
	}
	if !obs.pvc.GetDeletionTimestamp().IsZero() {
		return obs, &obsBlock{decide.ObsReasonTargetDeletionInProgress, "Target PersistentVolumeClaim is deleting"}
	}
	if obs.pvc.Status.Phase != corev1.ClaimBound || obs.pvc.Spec.VolumeName == "" {
		return obs, &obsBlock{decide.ObsReasonPersistentVolumeClaimNotBound, "Target PersistentVolumeClaim is not bound"}
	}
	obs.safety["persistentVolumeClaimBound"] = true
	if obs.pvc.Spec.StorageClassName == nil || *obs.pvc.Spec.StorageClassName != capabilitySpec.storageClassName {
		return obs, &obsBlock{decide.ObsReasonStorageClassMismatch, "Target PersistentVolumeClaim does not use the referenced StorageCapability"}
	}
	obs.safety["storageClassMatches"] = true

	obs.pv = &corev1.PersistentVolume{}
	if err := r.Client.Get(ctx, client.ObjectKey{Name: obs.pvc.Spec.VolumeName}, obs.pv); err != nil {
		if apierrors.IsNotFound(err) {
			return obs, &obsBlock{decide.ObsReasonPersistentVolumeNotFound, "Bound PersistentVolume was not found"}
		}
		return obs, &obsBlock{decide.ObsReasonPersistentVolumeReadFailed, "Bound PersistentVolume could not be read"}
	}
	if !obs.pv.GetDeletionTimestamp().IsZero() {
		return obs, &obsBlock{decide.ObsReasonTargetDeletionInProgress, "Bound PersistentVolume is deleting"}
	}
	if obs.pv.Status.Phase != corev1.VolumeBound {
		return obs, &obsBlock{decide.ObsReasonPersistentVolumeNotBound, "Bound PersistentVolume is not in the Bound phase"}
	}
	claimRef := obs.pv.Spec.ClaimRef
	if claimRef == nil || claimRef.Namespace != obs.pvc.Namespace || claimRef.Name != obs.pvc.Name || claimRef.UID != obs.pvc.UID || obs.pv.Spec.StorageClassName != capabilitySpec.storageClassName {
		return obs, &obsBlock{decide.ObsReasonPersistentVolumeBindingMismatch, "PersistentVolume binding does not match the exact target claim"}
	}
	obs.safety["persistentVolumeBindingMatches"] = true

	var podList corev1.PodList
	if err := r.Client.List(ctx, &podList, client.InNamespace(pvcNamespace)); err != nil {
		return obs, &obsBlock{decide.ObsReasonPodObservationFailed, "Pods using the target claim could not be read"}
	}
	for i := range podList.Items {
		pod := podList.Items[i]
		if !podUsesAttachmentRepairClaim(&pod, pvcName) {
			continue
		}
		if !pod.GetDeletionTimestamp().IsZero() {
			obs.safety["terminatingPodPresent"] = true
			return obs, &obsBlock{decide.ObsReasonTerminatingPodUsesClaim, "A terminating Pod still uses the target PersistentVolumeClaim"}
		}
		if pod.Status.Phase == corev1.PodSucceeded || pod.Status.Phase == corev1.PodFailed {
			continue
		}
		obs.pods = append(obs.pods, pod)
	}
	obs.safety["podsUsingClaimCount"] = int64(len(obs.pods))
	if len(obs.pods) != 1 || obs.pods[0].Spec.NodeName == "" {
		return obs, &obsBlock{decide.ObsReasonPodTargetNotUnique, "Exactly one non-terminating scheduled Pod must use the target PersistentVolumeClaim"}
	}
	obs.safety["currentPodScheduled"] = true

	var attachmentList storagev1.VolumeAttachmentList
	if err := r.Client.List(ctx, &attachmentList); err != nil {
		return obs, &obsBlock{decide.ObsReasonVolumeAttachmentObservationFailed, "VolumeAttachment evidence could not be read"}
	}
	for i := range attachmentList.Items {
		attachment := attachmentList.Items[i]
		if attachment.Spec.Source.PersistentVolumeName != nil && *attachment.Spec.Source.PersistentVolumeName == obs.pv.Name {
			obs.attachments = append(obs.attachments, attachment)
		}
	}
	obs.safety["volumeAttachmentCount"] = int64(len(obs.attachments))
	obs.safety["targetResolved"] = true
	return obs, nil
}

func (r *AttachmentRepairOperationReconciler) classifyInitialAttachments(ctx context.Context, obs *attachmentRepairObservation) *obsBlock {
	if len(obs.attachments) != 2 {
		return &obsBlock{decide.ObsReasonVolumeAttachmentSetNotUnique, "Exactly two VolumeAttachments must exist for the target PersistentVolume"}
	}
	currentNodeName := obs.pods[0].Spec.NodeName
	for i := range obs.attachments {
		attachment := &obs.attachments[i]
		if !attachment.GetDeletionTimestamp().IsZero() {
			return &obsBlock{decide.ObsReasonAttachmentDeletionInProgress, "A target VolumeAttachment is already deleting"}
		}
		if attachment.Spec.NodeName == currentNodeName {
			if attachment.Status.Attached || obs.currentAttachment != nil {
				return &obsBlock{decide.ObsReasonCurrentAttachmentNotUnique, "Exactly one pending current VolumeAttachment is required"}
			}
			obs.currentAttachment = attachment.DeepCopy()
			continue
		}
		if !attachment.Status.Attached || obs.staleAttachment != nil {
			return &obsBlock{decide.ObsReasonStaleAttachmentNotUnique, "Exactly one attached stale VolumeAttachment is required"}
		}
		obs.staleAttachment = attachment.DeepCopy()
	}
	if obs.currentAttachment == nil || obs.staleAttachment == nil {
		return &obsBlock{decide.ObsReasonAttachmentTargetMismatch, "Current and stale attachment targets could not be proven"}
	}
	if obs.currentAttachment.Spec.Attacher == "" || obs.currentAttachment.Spec.Attacher != obs.staleAttachment.Spec.Attacher {
		return &obsBlock{decide.ObsReasonAttachmentDriverMismatch, "Current and stale attachments do not use one CSI attacher"}
	}
	obs.safety["currentAttachmentCandidateCount"] = int64(1)
	obs.safety["staleAttachmentCandidateCount"] = int64(1)
	return r.observeAttachmentNodes(ctx, obs, true)
}

func (r *AttachmentRepairOperationReconciler) observeAttachmentNodes(ctx context.Context, obs *attachmentRepairObservation, allowStaleAbsent bool) *obsBlock {
	currentName := obs.currentAttachment.Spec.NodeName
	staleName := obs.staleAttachment.Spec.NodeName
	if currentName == "" || staleName == "" || currentName == staleName {
		return &obsBlock{decide.ObsReasonNodeTargetNotUnique, "Current and stale attachment Nodes must be distinct"}
	}
	obs.currentNode = &corev1.Node{}
	if err := r.Client.Get(ctx, client.ObjectKey{Name: currentName}, obs.currentNode); err != nil {
		if apierrors.IsNotFound(err) {
			return &obsBlock{decide.ObsReasonCurrentNodeNotFound, "Current attachment target Node was not found"}
		}
		return &obsBlock{decide.ObsReasonNodeObservationFailed, "Current attachment target Node could not be read"}
	}
	if !obs.currentNode.GetDeletionTimestamp().IsZero() || attachmentRepairNodeReady(obs.currentNode) != corev1.ConditionTrue {
		return &obsBlock{decide.ObsReasonCurrentNodeNotReady, "Current attachment target Node is not Ready"}
	}
	obs.safety["currentNodeReady"] = true

	obs.staleNode = &corev1.Node{}
	if err := r.Client.Get(ctx, client.ObjectKey{Name: staleName}, obs.staleNode); err != nil {
		if apierrors.IsNotFound(err) && allowStaleAbsent {
			obs.staleNode = nil
			obs.safety["staleNodeAbsent"] = true
			return nil
		}
		if apierrors.IsNotFound(err) {
			return &obsBlock{decide.ObsReasonStaleNodeNotFound, "Stale attachment target Node was not found"}
		}
		return &obsBlock{decide.ObsReasonNodeObservationFailed, "Stale attachment target Node could not be read"}
	}
	if !obs.staleNode.GetDeletionTimestamp().IsZero() {
		return &obsBlock{decide.ObsReasonNodeDeletionInProgress, "Stale attachment target Node is deleting"}
	}
	staleReady := attachmentRepairNodeReady(obs.staleNode)
	obs.safety["staleNodeRecovered"] = staleReady == corev1.ConditionTrue
	if staleReady == corev1.ConditionTrue {
		return &obsBlock{decide.ObsReasonStaleNodeRecovered, "Stale attachment Node recovered before repair"}
	}
	if staleReady != corev1.ConditionFalse {
		return &obsBlock{decide.ObsReasonStaleNodeReadinessAmbiguous, "Stale attachment Node readiness is ambiguous"}
	}
	return nil
}

func (r *AttachmentRepairOperationReconciler) observeAuthorizedSafety(ctx context.Context, operation *unstructured.Unstructured, requested bool) (*attachmentRepairObservation, decide.Target, *obsBlock) {
	obs, block := r.observeBase(ctx, operation)
	if block != nil {
		return obs, 0, block
	}
	if attachmentRepairTargetBindingIdentityDigest(obs) != firstStatusString(operation, "status", "execution", "targetBindingIdentityDigest") {
		return obs, 0, &obsBlock{decide.ObsReasonTargetBindingIdentityChanged, "StorageCapability, PersistentVolumeClaim, or PersistentVolume identity changed after authorization"}
	}
	staleDigest := firstStatusString(operation, "status", "execution", "staleAttachmentIdentityDigest")
	currentDigest := firstStatusString(operation, "status", "execution", "currentAttachmentIdentityDigest")
	for i := range obs.attachments {
		attachment := &obs.attachments[i]
		digest := attachmentRepairObjectIdentityDigest("volume-attachment-v1", attachment)
		switch digest {
		case staleDigest:
			obs.staleAttachment = attachment.DeepCopy()
		case currentDigest:
			obs.currentAttachment = attachment.DeepCopy()
		default:
			return obs, 0, &obsBlock{decide.ObsReasonAttachmentSetChanged, "An unapproved or recreated VolumeAttachment exists for the target PersistentVolume"}
		}
	}
	if obs.currentAttachment == nil {
		return obs, 0, &obsBlock{decide.ObsReasonCurrentAttachmentIdentityChanged, "The authorized current VolumeAttachment is missing or was recreated"}
	}
	if !obs.currentAttachment.GetDeletionTimestamp().IsZero() || obs.currentAttachment.Spec.Source.PersistentVolumeName == nil || *obs.currentAttachment.Spec.Source.PersistentVolumeName != obs.pv.Name || obs.currentAttachment.Spec.NodeName != obs.pods[0].Spec.NodeName {
		return obs, 0, &obsBlock{decide.ObsReasonCurrentAttachmentTargetChanged, "The authorized current attachment target changed"}
	}
	obs.currentNode = &corev1.Node{}
	if err := r.Client.Get(ctx, client.ObjectKey{Name: obs.currentAttachment.Spec.NodeName}, obs.currentNode); err != nil {
		return obs, 0, &obsBlock{decide.ObsReasonCurrentNodeNotFound, "Current attachment target Node was not found"}
	}
	if !obs.currentNode.GetDeletionTimestamp().IsZero() || attachmentRepairNodeReady(obs.currentNode) != corev1.ConditionTrue {
		return obs, 0, &obsBlock{decide.ObsReasonCurrentNodeNotReady, "Current attachment target Node is not Ready"}
	}
	if attachmentRepairObjectIdentityDigest("node-v1", obs.currentNode) != firstStatusString(operation, "status", "execution", "currentNodeIdentityDigest") {
		return obs, 0, &obsBlock{decide.ObsReasonCurrentNodeIdentityChanged, "Current attachment target Node was recreated"}
	}
	obs.safety["currentNodeReady"] = true
	obs.safety["currentAttachmentCandidateCount"] = int64(1)

	if obs.staleAttachment == nil {
		obs.safety["repairAdmitted"] = true
		return obs, decide.TargetAbsent, nil
	}
	if obs.staleAttachment.Spec.Source.PersistentVolumeName == nil || *obs.staleAttachment.Spec.Source.PersistentVolumeName != obs.pv.Name || obs.staleAttachment.Spec.NodeName == obs.currentAttachment.Spec.NodeName {
		return obs, 0, &obsBlock{decide.ObsReasonStaleAttachmentTargetChanged, "The authorized stale attachment target changed"}
	}
	obs.safety["staleAttachmentCandidateCount"] = int64(1)
	if block = r.observeAttachmentNodes(ctx, obs, true); block != nil {
		return obs, 0, block
	}
	storedStaleNodeDigest := firstStatusString(operation, "status", "execution", "staleNodeIdentityDigest")
	if obs.staleNode != nil {
		if storedStaleNodeDigest == "" || attachmentRepairObjectIdentityDigest("node-v1", obs.staleNode) != storedStaleNodeDigest {
			return obs, 0, &obsBlock{decide.ObsReasonStaleNodeIdentityChanged, "Stale attachment target Node appeared or was recreated after authorization"}
		}
	}
	if !obs.staleAttachment.GetDeletionTimestamp().IsZero() {
		obs.safety["repairAdmitted"] = true
		return obs, decide.TargetDeleting, nil
	}
	if requested {
		return obs, decide.TargetPresent, nil
	}
	if !obs.staleAttachment.Status.Attached || obs.currentAttachment.Status.Attached {
		return obs, 0, &obsBlock{decide.ObsReasonAttachmentStateChanged, "Attachment state changed after authorization"}
	}
	obs.digest = attachmentRepairObservationDigest(obs)
	obs.safety["repairAdmitted"] = true
	return obs, decide.TargetPresent, nil
}

func attachmentRepairObservationDigest(obs *attachmentRepairObservation) string {
	values := []string{
		"storage-attachment-repair-observation-v2",
		obs.capability.GetNamespace(), obs.capability.GetName(), string(obs.capability.GetUID()), obs.capability.GetResourceVersion(),
		obs.pvc.Namespace, obs.pvc.Name, string(obs.pvc.UID), obs.pvc.ResourceVersion, obs.pvc.Spec.VolumeName,
		obs.pv.Name, string(obs.pv.UID), obs.pv.ResourceVersion,
		obs.staleAttachment.Name, string(obs.staleAttachment.UID), obs.staleAttachment.ResourceVersion,
		obs.staleAttachment.Spec.NodeName, fmt.Sprint(obs.staleAttachment.Status.Attached),
		obs.currentAttachment.Name, string(obs.currentAttachment.UID), obs.currentAttachment.ResourceVersion,
		obs.currentAttachment.Spec.NodeName, fmt.Sprint(obs.currentAttachment.Status.Attached),
		obs.currentNode.Name, string(obs.currentNode.UID), string(attachmentRepairNodeReady(obs.currentNode)),
	}
	if obs.staleNode == nil {
		values = append(values, "stale-node-absent")
	} else {
		values = append(values, obs.staleNode.Name, string(obs.staleNode.UID), string(attachmentRepairNodeReady(obs.staleNode)))
	}
	pods := append([]corev1.Pod(nil), obs.pods...)
	sort.Slice(pods, func(i, j int) bool {
		if pods[i].Namespace != pods[j].Namespace {
			return pods[i].Namespace < pods[j].Namespace
		}
		return pods[i].Name < pods[j].Name
	})
	for i := range pods {
		pod := &pods[i]
		values = append(values, pod.Namespace, pod.Name, string(pod.UID), pod.ResourceVersion, pod.Spec.NodeName, string(pod.Status.Phase))
	}
	return attachmentRepairDigest(values...)
}

func attachmentRepairTargetBindingIdentityDigest(obs *attachmentRepairObservation) string {
	return attachmentRepairDigest(
		"target-binding-v1",
		obs.capability.GetNamespace(), obs.capability.GetName(), string(obs.capability.GetUID()),
		obs.pvc.Namespace, obs.pvc.Name, string(obs.pvc.UID),
		obs.pv.Name, string(obs.pv.UID),
	)
}

func attachmentRepairObjectIdentityDigest(contract string, obj metav1.Object) string {
	if obj == nil {
		return ""
	}
	return attachmentRepairDigest(contract, obj.GetNamespace(), obj.GetName(), string(obj.GetUID()))
}

func attachmentRepairDigest(values ...string) string {
	var canonical strings.Builder
	for _, value := range values {
		fmt.Fprintf(&canonical, "%d:%s|", len(value), value)
	}
	sum := sha256.Sum256([]byte(canonical.String()))
	return "sha256:" + hex.EncodeToString(sum[:])
}

func validAttachmentRepairDigest(value string) bool {
	if len(value) != len("sha256:")+sha256.Size*2 || !strings.HasPrefix(value, "sha256:") {
		return false
	}
	_, err := hex.DecodeString(strings.TrimPrefix(value, "sha256:"))
	return err == nil
}

func attachmentRepairNodeReady(node *corev1.Node) corev1.ConditionStatus {
	for i := range node.Status.Conditions {
		if node.Status.Conditions[i].Type == corev1.NodeReady {
			return node.Status.Conditions[i].Status
		}
	}
	return corev1.ConditionUnknown
}

func podUsesAttachmentRepairClaim(pod *corev1.Pod, claimName string) bool {
	for i := range pod.Spec.Volumes {
		claim := pod.Spec.Volumes[i].PersistentVolumeClaim
		if claim != nil && claim.ClaimName == claimName {
			return true
		}
	}
	return false
}

func attachmentRepairSafetyStatus() map[string]any {
	return map[string]any{
		"targetResolved":                  false,
		"persistentVolumeClaimBound":      false,
		"persistentVolumeBindingMatches":  false,
		"storageClassMatches":             false,
		"podsUsingClaimCount":             int64(0),
		"terminatingPodPresent":           false,
		"currentPodScheduled":             false,
		"currentNodeReady":                false,
		"staleNodeAbsent":                 false,
		"staleNodeRecovered":              false,
		"volumeAttachmentCount":           int64(0),
		"staleAttachmentCandidateCount":   int64(0),
		"currentAttachmentCandidateCount": int64(0),
		"approvalMatchesObservation":      false,
		"repairAdmitted":                  false,
	}
}

func (r *AttachmentRepairOperationReconciler) awaitingApprovalStatus(operation *unstructured.Unstructured, obs *attachmentRepairObservation) map[string]any {
	status := r.baseStatus(operation)
	status["phase"] = "AwaitingApproval"
	status["currentStage"] = "AwaitApproval"
	status["lastSafetyCheck"] = r.now().Format(time.RFC3339)
	status["observationDigest"] = obs.digest
	status["safety"] = obs.safety
	status["execution"] = attachmentRepairExecutionStatus(operation, obs, nil, false, false)
	status["result"] = "Attachment repair observation is awaiting exact approval"
	status["nextAction"] = "Approve this operation observation through the trusted Platform API workflow."
	conditions := existingConditions(operation)
	setCondition(&conditions, condition(operation, "Ready", metav1.ConditionFalse, "AwaitingApproval", "Attachment repair observation is awaiting exact approval"))
	status["conditions"] = conditionsToUnstructured(conditions)
	return status
}

func (r *AttachmentRepairOperationReconciler) authorizedStatus(operation *unstructured.Unstructured, obs *attachmentRepairObservation, approval *unstructured.Unstructured) map[string]any {
	status := r.baseStatus(operation)
	status["phase"] = "Running"
	status["currentStage"] = "AuthorizeExactDeletion"
	status["lastSafetyCheck"] = r.now().Format(time.RFC3339)
	status["observationDigest"] = obs.digest
	status["safety"] = obs.safety
	execution := attachmentRepairExecutionStatus(operation, obs, approval, true, false)
	execution["targetBindingIdentityDigest"] = attachmentRepairTargetBindingIdentityDigest(obs)
	execution["staleAttachmentIdentityDigest"] = attachmentRepairObjectIdentityDigest("volume-attachment-v1", obs.staleAttachment)
	execution["currentAttachmentIdentityDigest"] = attachmentRepairObjectIdentityDigest("volume-attachment-v1", obs.currentAttachment)
	execution["currentNodeIdentityDigest"] = attachmentRepairObjectIdentityDigest("node-v1", obs.currentNode)
	if obs.staleNode != nil {
		execution["staleNodeIdentityDigest"] = attachmentRepairObjectIdentityDigest("node-v1", obs.staleNode)
	}
	status["execution"] = execution
	status["result"] = "Exact attachment deletion was authorized and will be revalidated before mutation"
	status["nextAction"] = "Wait for controller-owned compare-and-swap deletion and attachment convergence."
	status["operatorDiagnosticsAvailable"] = true
	conditions := existingConditions(operation)
	setCondition(&conditions, condition(operation, "Ready", metav1.ConditionFalse, "OperationRunning", "Exact attachment deletion is authorized"))
	status["conditions"] = conditionsToUnstructured(conditions)
	return status
}

func (r *AttachmentRepairOperationReconciler) blockedStatus(operation *unstructured.Unstructured, stage, reason, message string, obs *attachmentRepairObservation, approval *unstructured.Unstructured) map[string]any {
	status := r.baseStatus(operation)
	status["phase"] = "Blocked"
	status["currentStage"] = stage
	status["lastSafetyCheck"] = r.now().Format(time.RFC3339)
	status["result"] = message
	status["nextAction"] = "Create a fresh operation with a new name; attachment repair approvals are one-shot."
	status["operatorDiagnosticsAvailable"] = true
	if obs != nil {
		status["safety"] = obs.safety
		if obs.digest != "" {
			status["observationDigest"] = obs.digest
		}
	}
	status["execution"] = attachmentRepairExecutionStatus(operation, obs, approval, false, false)
	conditions := existingConditions(operation)
	setCondition(&conditions, condition(operation, "Ready", metav1.ConditionFalse, reason, message))
	status["conditions"] = conditionsToUnstructured(conditions)
	return status
}

func (r *AttachmentRepairOperationReconciler) runningStatus(operation *unstructured.Unstructured, stage, message string, obs *attachmentRepairObservation, approval *unstructured.Unstructured, requested bool) map[string]any {
	status := r.baseStatus(operation)
	status["phase"] = "Running"
	status["currentStage"] = stage
	status["lastSafetyCheck"] = r.now().Format(time.RFC3339)
	status["result"] = message
	status["nextAction"] = "Wait for exact Kubernetes attachment convergence; safety evidence will be re-evaluated."
	status["operatorDiagnosticsAvailable"] = true
	status["safety"] = obs.safety
	status["execution"] = attachmentRepairExecutionStatus(operation, obs, approval, true, requested)
	conditions := existingConditions(operation)
	setCondition(&conditions, condition(operation, "Ready", metav1.ConditionFalse, "OperationRunning", message))
	status["conditions"] = conditionsToUnstructured(conditions)
	return status
}

func (r *AttachmentRepairOperationReconciler) succeededStatus(operation *unstructured.Unstructured, stage, message string, obs *attachmentRepairObservation, approval *unstructured.Unstructured, requested bool) map[string]any {
	status := r.baseStatus(operation)
	status["phase"] = "Succeeded"
	status["currentStage"] = stage
	status["completedAt"] = r.now().Format(time.RFC3339)
	status["lastSafetyCheck"] = r.now().Format(time.RFC3339)
	if obs.digest != "" {
		status["observationDigest"] = obs.digest
	}
	status["result"] = message
	status["nextAction"] = "No further attachment repair action is required for this operation."
	status["safety"] = obs.safety
	status["execution"] = attachmentRepairExecutionStatus(operation, obs, approval, requested, requested)
	conditions := existingConditions(operation)
	setCondition(&conditions, condition(operation, "Ready", metav1.ConditionTrue, "OperationSucceeded", message))
	status["conditions"] = conditionsToUnstructured(conditions)
	return status
}

func (r *AttachmentRepairOperationReconciler) baseStatus(operation *unstructured.Unstructured) map[string]any {
	status := map[string]any{"observedGeneration": operation.GetGeneration()}
	startedAt := firstStatusString(operation, "status", "startedAt")
	if startedAt == "" {
		startedAt = r.now().Format(time.RFC3339)
	}
	status["startedAt"] = startedAt
	return status
}

func attachmentRepairExecutionStatus(operation *unstructured.Unstructured, obs *attachmentRepairObservation, approval *unstructured.Unstructured, authorized, requested bool) map[string]any {
	existing, _, _ := unstructured.NestedMap(operation.Object, "status", "execution")
	status := map[string]any{
		"dryRun":             attachmentRepairDryRun(operation),
		"deletionAuthorized": authorized || nestedStatusBool(operation, "execution", "deletionAuthorized"),
		"deletionCommitted":  nestedStatusBool(operation, "execution", "deletionCommitted"),
		"deletionRequested":  requested || nestedStatusBool(operation, "execution", "deletionRequested"),
	}
	for _, key := range []string{
		"approvedObservationDigest", "targetBindingIdentityDigest", "staleAttachmentIdentityDigest",
		"currentAttachmentIdentityDigest", "currentNodeIdentityDigest", "staleNodeIdentityDigest", "auditCorrelationID",
	} {
		if value, ok := existing[key].(string); ok && value != "" {
			status[key] = value
		}
	}
	if approval != nil {
		if digest := nestedAttachmentRepairString(approval, "spec", "observationDigest"); digest != "" {
			status["approvedObservationDigest"] = digest
		}
		if auditID := nestedAttachmentRepairString(approval, "spec", "auditCorrelationID"); auditID != "" {
			status["auditCorrelationID"] = auditID
		}
	}
	if obs != nil {
		if obs.staleAttachment == nil {
			status["targetAbsent"] = true
		} else if !obs.staleAttachment.GetDeletionTimestamp().IsZero() {
			status["targetDeletionObserved"] = true
		}
		if obs.currentAttachment != nil && obs.currentAttachment.Status.Attached {
			status["currentAttachmentAttached"] = true
		}
	}
	return status
}

func (r *AttachmentRepairApprovalReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	approval := newStorageAttachmentRepairApprovalObject()
	if err := r.Client.Get(ctx, req.NamespacedName, approval); err != nil {
		if apierrors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, fmt.Errorf("get StorageAttachmentRepairApproval: %w", err)
	}
	if !approval.GetDeletionTimestamp().IsZero() {
		return ctrl.Result{}, nil
	}
	oldPhase := firstStatusString(approval, "status", "phase")
	status, requeueAfter := r.observeApproval(ctx, approval)
	if err := patchUnstructuredStatus(ctx, r.Client, approval, status); err != nil {
		return ctrl.Result{}, err
	}
	if oldPhase != fmt.Sprint(status["phase"]) {
		eventType, reason, message := storageAttachmentRepairApprovalEvent(approval, status)
		recordStorageWorkflowEvent(r.Recorder, approval, eventType, reason, message)
	}
	return ctrl.Result{RequeueAfter: requeueAfter}, nil
}

func (r *AttachmentRepairApprovalReconciler) observeApproval(ctx context.Context, approval *unstructured.Unstructured) (map[string]any, time.Duration) {
	operationName := nestedAttachmentRepairString(approval, "spec", "operationRef", "name")
	if operationName == "" {
		return attachmentRepairApprovalStatus(approval, "Rejected", "OperationReferenceMissing", "Approval does not reference an attachment repair operation"), 0
	}
	operation := newStorageAttachmentRepairOperationObject()
	if err := r.Client.Get(ctx, client.ObjectKey{Namespace: approval.GetNamespace(), Name: operationName}, operation); err != nil {
		if apierrors.IsNotFound(err) {
			return attachmentRepairApprovalStatus(approval, "Rejected", "OperationNotFound", "Referenced attachment repair operation was not found"), 0
		}
		return attachmentRepairApprovalStatus(approval, "Rejected", "OperationReadFailed", "Referenced attachment repair operation could not be read"), attachmentRepairRequeueInterval
	}
	if attachmentRepairDryRun(operation) {
		return attachmentRepairApprovalStatus(approval, "Rejected", "DryRunOperation", "Dry-run attachment repair operations cannot be approved for mutation"), 0
	}
	now := r.now()
	if reason, message := validateAttachmentRepairApprovalBinding(operation, approval, now); reason != "" {
		return attachmentRepairApprovalStatus(approval, "Rejected", reason, message), 0
	}
	if phase := firstStatusString(approval, "status", "phase"); phase == "Rejected" || phase == "Superseded" || phase == "Expired" {
		status, _, _ := unstructured.NestedMap(approval.Object, "status")
		return status, 0
	}
	expiresAt, ok := attachmentRepairApprovalExpiresAt(approval)
	if !ok || !now.Before(expiresAt) {
		return attachmentRepairApprovalStatus(approval, "Expired", "ApprovalExpired", "Attachment repair approval expired before new authorization"), 0
	}
	requeueAfter := attachmentRepairApprovalRequeueAfter(now, expiresAt)
	approvalDigest := nestedAttachmentRepairString(approval, "spec", "observationDigest")
	if nestedStatusBool(operation, "execution", "deletionCommitted") && firstStatusString(operation, "status", "execution", "approvedObservationDigest") == approvalDigest {
		return attachmentRepairApprovalStatus(approval, "Accepted", "ApprovalAccepted", "Approval remains bound to the authorized attachment repair observation"), requeueAfter
	}
	observer := &AttachmentRepairOperationReconciler{Client: r.Client, Now: r.Now}
	obs, block := observer.observeInitialTyped(ctx, operation)
	if block != nil {
		return attachmentRepairApprovalStatus(approval, "Superseded", block.reason.String(), "Attachment repair observation is no longer executable: "+block.message), 0
	}
	if approvalDigest != obs.digest {
		return attachmentRepairApprovalStatus(approval, "Superseded", "ObservationDigestChanged", "Approval observation digest no longer matches live attachment evidence"), 0
	}
	return attachmentRepairApprovalStatus(approval, "Accepted", "ApprovalAccepted", "Approval matches the exact operation and current attachment observation"), requeueAfter
}

func attachmentRepairApprovalRequeueAfter(now, expiresAt time.Time) time.Duration {
	remaining := expiresAt.Sub(now)
	if remaining < attachmentRepairRequeueInterval {
		return remaining
	}
	return attachmentRepairRequeueInterval
}

func attachmentRepairApprovalStatus(approval *unstructured.Unstructured, phase, reason, message string) map[string]any {
	conditions := existingConditions(approval)
	ready := metav1.ConditionFalse
	if phase == "Accepted" {
		ready = metav1.ConditionTrue
	}
	setCondition(&conditions, condition(approval, "Ready", ready, reason, message))
	return map[string]any{
		"observedGeneration": approval.GetGeneration(),
		"phase":              phase,
		"message":            message,
		"conditions":         conditionsToUnstructured(conditions),
	}
}

func attachmentRepairDryRun(operation *unstructured.Unstructured) bool {
	dryRun, found, _ := unstructured.NestedBool(operation.Object, "spec", "dryRun")
	return !found || dryRun
}

func nestedAttachmentRepairString(object *unstructured.Unstructured, fields ...string) string {
	if object == nil {
		return ""
	}
	value, _, _ := unstructured.NestedString(object.Object, fields...)
	return strings.TrimSpace(value)
}

func nestedAttachmentRepairInt64(object *unstructured.Unstructured, fields ...string) (int64, bool) {
	if object == nil {
		return 0, false
	}
	value, found, _ := unstructured.NestedInt64(object.Object, fields...)
	return value, found
}

func nestedStatusBool(operation *unstructured.Unstructured, fields ...string) bool {
	path := append([]string{"status"}, fields...)
	value, _, _ := unstructured.NestedBool(operation.Object, path...)
	return value
}

func attachmentRepairTerminal(operation *unstructured.Unstructured) bool {
	switch firstStatusString(operation, "status", "phase") {
	case "Succeeded", "Failed":
		return true
	default:
		return false
	}
}

func (r *AttachmentRepairOperationReconciler) now() time.Time {
	if r.Now != nil {
		return r.Now().UTC()
	}
	return time.Now().UTC()
}

func (r *AttachmentRepairApprovalReconciler) now() time.Time {
	if r.Now != nil {
		return r.Now().UTC()
	}
	return time.Now().UTC()
}

func newStorageAttachmentRepairOperationObject() *unstructured.Unstructured {
	obj := &unstructured.Unstructured{}
	obj.SetGroupVersionKind(storageAttachmentRepairOperationGVK)
	return obj
}

func newStorageAttachmentRepairApprovalObject() *unstructured.Unstructured {
	obj := &unstructured.Unstructured{}
	obj.SetGroupVersionKind(storageAttachmentRepairApprovalGVK)
	return obj
}

func (r *AttachmentRepairOperationReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).For(newStorageAttachmentRepairOperationObject()).Complete(r)
}

func (r *AttachmentRepairApprovalReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).For(newStorageAttachmentRepairApprovalObject()).Complete(r)
}
