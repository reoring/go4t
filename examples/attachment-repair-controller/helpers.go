package storage

import (
	"context"
	"fmt"
	"math"
	"reflect"
	"strconv"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (


	StorageCapabilityGroup   = "storage.appthrust.io"
	StorageCapabilityVersion = "v1alpha1"
	StorageCapabilityKind    = "StorageCapability"


	defaultRookNamespace                         = "rook-ceph"
	defaultRookClusterName                       = "rook-ceph"
	defaultRookStorageClassName                  = "appthrust-ceph-block"
	defaultRookVolumeSnapshotClass               = "appthrust-ceph-block"
	defaultRookBlockPoolName                     = "appthrust-blockpool"
	defaultRookFilesystemName                    = "appthrust-cephfs"
	defaultRookFilesystemStorageName             = "appthrust-ceph-filesystem"
	defaultRookObjectBucketStorage               = "appthrust-ceph-object-bucket"
	defaultRookObjectGatewayPort                 = int32(80)
	defaultRookMDSCommandImage                   = "quay.io/ceph/ceph:v19.2.4"
	defaultRookMDSSessionAutocloseSeconds        = int32(45)
	defaultRookMDSReconnectTimeoutSeconds        = int32(30)
	defaultRookMDSBlocklistOnTimeout             = true
	defaultRookMDSBlocklistOnEvict               = true
	defaultRookObjectGatewaySvcType              = corev1.ServiceTypeLoadBalancer
	defaultRookFailureDomain                     = "host"


	storageCapabilityManagedProvider             = "RookCeph"
	storageCapabilityConvergedMode               = "SingleClusterConverged"


)



var (


	storageCapabilityGVK = schema.GroupVersionKind{
		Group:   StorageCapabilityGroup,
		Version: StorageCapabilityVersion,
		Kind:    StorageCapabilityKind,
	}


)



type capabilitySpec struct {
	namespace                    string
	provider                     string
	mode                         string
	rookNamespace                string
	clusterID                    string
	csiDriverNamespace           string
	clusterName                  string
	storageClassName             string
	volumeSnapshotClassName      string
	blockPoolName                string
	filesystemName               string
	filesystemStorageClassName   string
	mdsSessionAutocloseSeconds   int32
	mdsReconnectTimeoutSeconds   int32
	mdsBlocklistOnTimeout        bool
	mdsBlocklistOnEvict          bool
	mdsCommandImage              string
	objectStoreName              string
	objectBucketStorageClassName string
	objectBucketProvisionerName  string
	objectGatewayPort            int32
	objectGatewayEndpoints       []objectGatewayEndpoint
	objectGatewayServiceEnabled  bool
	objectGatewayServiceName     string
	objectGatewayServiceType     corev1.ServiceType
	objectGatewayServiceLabels   map[string]string
	objectGatewayServiceAnn      map[string]string
	replicationSize              int32
	replicationFailureDomain     string
	blockEnabled                 bool
	volumeSnapshotsEnabled       bool
	fileEnabled                  bool
	objectEnabled                bool
}

type objectGatewayEndpoint struct {
	ip       string
	hostname string
}

//nolint:gocyclo // StorageCapability is a compatibility facade over several optional spec sections.
func parseSpec(capability *unstructured.Unstructured) capabilitySpec {
	spec := capabilitySpec{
		namespace:                    capability.GetNamespace(),
		provider:                     storageCapabilityManagedProvider,
		mode:                         storageCapabilityConvergedMode,
		rookNamespace:                defaultRookNamespace,
		clusterID:                    defaultRookNamespace,
		csiDriverNamespace:           defaultRookNamespace,
		clusterName:                  defaultRookClusterName,
		storageClassName:             defaultRookStorageClassName,
		volumeSnapshotClassName:      defaultRookVolumeSnapshotClass,
		blockPoolName:                defaultRookBlockPoolName,
		filesystemName:               defaultRookFilesystemName,
		filesystemStorageClassName:   defaultRookFilesystemStorageName,
		mdsSessionAutocloseSeconds:   defaultRookMDSSessionAutocloseSeconds,
		mdsReconnectTimeoutSeconds:   defaultRookMDSReconnectTimeoutSeconds,
		mdsBlocklistOnTimeout:        defaultRookMDSBlocklistOnTimeout,
		mdsBlocklistOnEvict:          defaultRookMDSBlocklistOnEvict,
		mdsCommandImage:              defaultRookMDSCommandImage,
		objectStoreName:              "appthrust-object-store",
		objectBucketStorageClassName: defaultRookObjectBucketStorage,
		objectGatewayPort:            defaultRookObjectGatewayPort,
		objectGatewayServiceType:     defaultRookObjectGatewaySvcType,
		replicationSize:              3,
		replicationFailureDomain:     defaultRookFailureDomain,
		blockEnabled:                 true,
		volumeSnapshotsEnabled:       true,
	}
	if provider, ok, _ := unstructured.NestedString(capability.Object, "spec", "provider"); ok && provider != "" {
		spec.provider = provider
	}
	if mode, ok, _ := unstructured.NestedString(capability.Object, "spec", "mode"); ok && mode != "" {
		spec.mode = mode
	}
	if namespace, ok, _ := unstructured.NestedString(capability.Object, "spec", "rookCeph", "namespace"); ok && namespace != "" {
		spec.rookNamespace = namespace
		spec.clusterID = namespace
		spec.csiDriverNamespace = namespace
	}
	if clusterID, ok, _ := unstructured.NestedString(capability.Object, "spec", "rookCeph", "clusterID"); ok && clusterID != "" {
		spec.clusterID = clusterID
	}
	if namespace, ok, _ := unstructured.NestedString(capability.Object, "spec", "rookCeph", "csiDriverNamespace"); ok && namespace != "" {
		spec.csiDriverNamespace = namespace
	}
	if clusterName, ok, _ := unstructured.NestedString(capability.Object, "spec", "rookCeph", "clusterName"); ok && clusterName != "" {
		spec.clusterName = clusterName
	}
	if storageClassName, ok, _ := unstructured.NestedString(capability.Object, "spec", "rookCeph", "storageClassName"); ok && storageClassName != "" {
		spec.storageClassName = storageClassName
	}
	if snapshotClassName, ok, _ := unstructured.NestedString(capability.Object, "spec", "rookCeph", "volumeSnapshotClassName"); ok && snapshotClassName != "" {
		spec.volumeSnapshotClassName = snapshotClassName
	}
	if blockPoolName, ok, _ := unstructured.NestedString(capability.Object, "spec", "rookCeph", "blockPoolName"); ok && blockPoolName != "" {
		spec.blockPoolName = blockPoolName
	}
	if filesystemName, ok, _ := unstructured.NestedString(capability.Object, "spec", "rookCeph", "filesystemName"); ok && filesystemName != "" {
		spec.filesystemName = filesystemName
	}
	if storageClassName, ok, _ := unstructured.NestedString(capability.Object, "spec", "rookCeph", "filesystemStorageClassName"); ok && storageClassName != "" {
		spec.filesystemStorageClassName = storageClassName
	}
	if seconds, ok := nestedInt64(capability.Object, "spec", "rookCeph", "mds", "sessionAutocloseSeconds"); ok && seconds > 0 {
		spec.mdsSessionAutocloseSeconds = int32(seconds)
	}
	if seconds, ok := nestedInt64(capability.Object, "spec", "rookCeph", "mds", "reconnectTimeoutSeconds"); ok && seconds > 0 {
		spec.mdsReconnectTimeoutSeconds = int32(seconds)
	}
	if enabled, ok, _ := unstructured.NestedBool(capability.Object, "spec", "rookCeph", "mds", "blocklistOnTimeout"); ok {
		spec.mdsBlocklistOnTimeout = enabled
	}
	if enabled, ok, _ := unstructured.NestedBool(capability.Object, "spec", "rookCeph", "mds", "blocklistOnEvict"); ok {
		spec.mdsBlocklistOnEvict = enabled
	}
	if image, ok, _ := unstructured.NestedString(capability.Object, "spec", "rookCeph", "mds", "commandImage"); ok && strings.TrimSpace(image) != "" {
		spec.mdsCommandImage = strings.TrimSpace(image)
	}
	if objectStoreName, ok, _ := unstructured.NestedString(capability.Object, "spec", "rookCeph", "objectStoreName"); ok && objectStoreName != "" {
		spec.objectStoreName = objectStoreName
	}
	if storageClassName, ok, _ := unstructured.NestedString(capability.Object, "spec", "rookCeph", "objectBucketStorageClassName"); ok && storageClassName != "" {
		spec.objectBucketStorageClassName = storageClassName
	}
	if provisioner, ok, _ := unstructured.NestedString(capability.Object, "spec", "rookCeph", "objectBucketProvisioner"); ok && strings.TrimSpace(provisioner) != "" {
		spec.objectBucketProvisionerName = strings.TrimSpace(provisioner)
	}
	if port, ok := nestedInt64(capability.Object, "spec", "rookCeph", "objectGatewayPort"); ok && port > 0 {
		spec.objectGatewayPort = int32(port)
	}
	spec.objectGatewayEndpoints = parseObjectGatewayEndpoints(capability.Object)
	if enabled, ok, _ := unstructured.NestedBool(capability.Object, "spec", "rookCeph", "objectGatewayService", "enabled"); ok {
		spec.objectGatewayServiceEnabled = enabled
	}
	if name, ok, _ := unstructured.NestedString(capability.Object, "spec", "rookCeph", "objectGatewayService", "name"); ok && name != "" {
		spec.objectGatewayServiceName = name
	}
	if serviceType, ok, _ := unstructured.NestedString(capability.Object, "spec", "rookCeph", "objectGatewayService", "type"); ok && serviceType != "" {
		spec.objectGatewayServiceType = corev1.ServiceType(serviceType)
	}
	spec.objectGatewayServiceLabels = parseStringMap(capability.Object, "spec", "rookCeph", "objectGatewayService", "labels")
	spec.objectGatewayServiceAnn = parseStringMap(capability.Object, "spec", "rookCeph", "objectGatewayService", "annotations")
	if size, ok := nestedInt64(capability.Object, "spec", "rookCeph", "replication", "size"); ok {
		spec.replicationSize = replicationSizeFromInt64(size, spec.replicationSize)
	}
	if failureDomain, ok, _ := unstructured.NestedString(capability.Object, "spec", "rookCeph", "replication", "failureDomain"); ok && failureDomain != "" {
		spec.replicationFailureDomain = failureDomain
	}
	if enabled, ok, _ := unstructured.NestedBool(capability.Object, "spec", "features", "block"); ok {
		spec.blockEnabled = enabled
	}
	if enabled, ok, _ := unstructured.NestedBool(capability.Object, "spec", "features", "volumeSnapshots"); ok {
		spec.volumeSnapshotsEnabled = enabled
	}
	if enabled, ok, _ := unstructured.NestedBool(capability.Object, "spec", "features", "file"); ok {
		spec.fileEnabled = enabled
	}
	if enabled, ok, _ := unstructured.NestedBool(capability.Object, "spec", "features", "object"); ok {
		spec.objectEnabled = enabled
	}
	if spec.objectGatewayServiceName == "" {
		spec.objectGatewayServiceName = spec.objectStoreName + "-gateway"
	}
	return spec
}

func parseObjectGatewayEndpoints(obj map[string]any) []objectGatewayEndpoint {
	raw, found, err := unstructured.NestedSlice(obj, "spec", "rookCeph", "objectGatewayEndpoints")
	if err != nil || !found {
		return nil
	}
	endpoints := make([]objectGatewayEndpoint, 0, len(raw))
	for _, item := range raw {
		m, ok := item.(map[string]any)
		if !ok {
			continue
		}
		endpoint := objectGatewayEndpoint{
			ip:       strings.TrimSpace(stringValue(m["ip"])),
			hostname: strings.TrimSpace(stringValue(m["hostname"])),
		}
		if endpoint.ip == "" && endpoint.hostname == "" {
			continue
		}
		endpoints = append(endpoints, endpoint)
	}
	return endpoints
}


func stringValue(value any) string {
	text, _ := value.(string)
	return text
}


func parseStringMap(obj map[string]any, fields ...string) map[string]string {
	if raw, found, err := unstructured.NestedStringMap(obj, fields...); err == nil && found {
		out := map[string]string{}
		for key, value := range raw {
			key = strings.TrimSpace(key)
			value = strings.TrimSpace(value)
			if key != "" && value != "" {
				out[key] = value
			}
		}
		if len(out) > 0 {
			return out
		}
		return nil
	}
	rawMap, found, err := unstructured.NestedMap(obj, fields...)
	if err != nil || !found {
		return nil
	}
	out := map[string]string{}
	for key, value := range rawMap {
		key = strings.TrimSpace(key)
		text := strings.TrimSpace(stringValue(value))
		if key != "" && text != "" {
			out[key] = text
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}


func newStorageCapabilityObject() *unstructured.Unstructured {
	obj := &unstructured.Unstructured{}
	obj.SetGroupVersionKind(storageCapabilityGVK)
	return obj
}

func condition(capability *unstructured.Unstructured, conditionType string, status metav1.ConditionStatus, reason, message string) metav1.Condition {
	return metav1.Condition{
		Type:               conditionType,
		Status:             status,
		ObservedGeneration: capability.GetGeneration(),
		LastTransitionTime: metav1.Now(),
		Reason:             reason,
		Message:            message,
	}
}


//nolint:gocritic // meta.SetStatusCondition accepts a value and callers build conditions inline.
func setCondition(conditions *[]metav1.Condition, next metav1.Condition) {
	meta.SetStatusCondition(conditions, next)
}

func replicationSizeFromInt64(size int64, fallback int32) int32 {
	if size <= 0 || size > math.MaxInt32 {
		return fallback
	}
	return int32(size)
}

func conditionsToUnstructured(conditions []metav1.Condition) []any {
	out := make([]any, 0, len(conditions))
	for _, c := range conditions {
		item := map[string]any{
			"type":               c.Type,
			"status":             string(c.Status),
			"observedGeneration": c.ObservedGeneration,
			"lastTransitionTime": c.LastTransitionTime.Format(time.RFC3339),
			"reason":             c.Reason,
			"message":            c.Message,
		}
		out = append(out, item)
	}
	return out
}

func existingConditions(capability *unstructured.Unstructured) []metav1.Condition {
	items, ok, _ := unstructured.NestedSlice(capability.Object, "status", "conditions")
	if !ok {
		return nil
	}
	conditions := make([]metav1.Condition, 0, len(items))
	for _, item := range items {
		raw, ok := item.(map[string]any)
		if !ok {
			continue
		}
		conditionType, _ := raw["type"].(string)
		statusString, _ := raw["status"].(string)
		reason, _ := raw["reason"].(string)
		message, _ := raw["message"].(string)
		if conditionType == "" || statusString == "" || reason == "" {
			continue
		}
		condition := metav1.Condition{
			Type:    conditionType,
			Status:  metav1.ConditionStatus(statusString),
			Reason:  reason,
			Message: message,
		}
		if observedGeneration, ok := numericToInt64(raw["observedGeneration"]); ok {
			condition.ObservedGeneration = observedGeneration
		}
		if timestamp, ok := raw["lastTransitionTime"].(string); ok {
			if parsed, err := time.Parse(time.RFC3339, timestamp); err == nil {
				condition.LastTransitionTime = metav1.NewTime(parsed)
			}
		}
		conditions = append(conditions, condition)
	}
	return conditions
}


func nestedInt64(obj map[string]any, fields ...string) (int64, bool) {
	value, ok, err := unstructured.NestedFieldNoCopy(obj, fields...)
	if !ok || err != nil {
		return 0, false
	}
	return numericToInt64(value)
}

func numericToInt64(value any) (int64, bool) {
	switch typed := value.(type) {
	case int64:
		return typed, true
	case int32:
		return int64(typed), true
	case int:
		return int64(typed), true
	case float64:
		return int64(typed), true
	case string:
		parsed, parseErr := strconv.ParseInt(typed, 10, 64)
		return parsed, parseErr == nil
	default:
		return 0, false
	}
}


type StorageFeatureGates struct {
	ScaleInPlans                   bool
	ScaleInApprovals               bool
	ScaleInNonDestructiveExecution bool
	OSDScaleInDryRun               bool
	OSDScaleInExecution            bool
	ProviderResourcePruning        bool
	ProviderRecoveryExecution      bool
	AttachmentRepairExecution      bool
}


func patchUnstructuredStatus(ctx context.Context, cl client.Client, obj *unstructured.Unstructured, status map[string]any) error {
	original := obj.DeepCopy()
	current, _, _ := unstructured.NestedMap(obj.Object, "status")
	if reflect.DeepEqual(current, status) {
		return nil
	}
	if err := unstructured.SetNestedMap(obj.Object, status, "status"); err != nil {
		return fmt.Errorf("set %s status: %w", obj.GetKind(), err)
	}
	if err := cl.Status().Patch(ctx, obj, client.MergeFrom(original)); err != nil {
		return fmt.Errorf("patch %s status: %w", obj.GetKind(), err)
	}
	return nil
}


func firstStatusString(obj *unstructured.Unstructured, fields ...string) string {
	value, ok, _ := unstructured.NestedString(obj.Object, fields...)
	if ok {
		return value
	}
	return ""
}


func firstNonEmptyNestedString(obj map[string]any, paths ...[]string) string {
	for _, path := range paths {
		value, ok, _ := unstructured.NestedString(obj, path...)
		if ok && value != "" {
			return value
		}
	}
	return ""
}


func recordStorageWorkflowEvent(recorder record.EventRecorder, obj *unstructured.Unstructured, eventType, reason, message string) {
	if recorder == nil || obj == nil || reason == "" {
		return
	}
	recorder.Event(obj, eventType, reason, message)
}


func storageAttachmentRepairApprovalEvent(approval *unstructured.Unstructured, status map[string]any) (string, string, string) {
	phase := fmt.Sprint(status["phase"])
	message := strings.TrimSpace(fmt.Sprint(status["message"]))
	if message == "" {
		message = "Storage attachment repair approval status changed"
	}
	message = appendAuditCorrelation(message, firstNonEmptyNestedString(approval.Object, []string{"spec", "auditCorrelationID"}))
	switch phase {
	case "Accepted":
		return corev1.EventTypeNormal, "StorageAttachmentRepairApprovalAccepted", message
	case "Rejected":
		return corev1.EventTypeWarning, "StorageAttachmentRepairApprovalRejected", message
	case "Superseded":
		return corev1.EventTypeWarning, "StorageAttachmentRepairApprovalSuperseded", message
	case "Expired":
		return corev1.EventTypeWarning, "StorageAttachmentRepairApprovalExpired", message
	default:
		return "", "", ""
	}
}


func storageAttachmentRepairOperationEvent(operation *unstructured.Unstructured, status map[string]any) (string, string, string) {
	phase := fmt.Sprint(status["phase"])
	stage := strings.TrimSpace(fmt.Sprint(status["currentStage"]))
	message := strings.TrimSpace(fmt.Sprint(status["result"]))
	if message == "" {
		message = "Storage attachment repair operation status changed"
	}
	if stage != "" {
		message = fmt.Sprintf("%s: %s", stage, message)
	}
	message = appendAuditCorrelation(message, firstNonEmptyNestedString(operation.Object, []string{"status", "execution", "auditCorrelationID"}))
	switch phase {
	case "Running":
		switch stage {
		case "AuthorizeExactDeletion":
			return corev1.EventTypeNormal, "StorageAttachmentRepairDeletionAuthorized", message
		case "CommitExactDeletion":
			return corev1.EventTypeNormal, "StorageAttachmentRepairDeletionCommitted", message
		case "WaitForConvergence":
			return corev1.EventTypeNormal, "StorageAttachmentRepairDeletionRequested", message
		default:
			return "", "", ""
		}
	case "Blocked":
		return corev1.EventTypeWarning, "StorageAttachmentRepairOperationBlocked", message
	case "Succeeded":
		return corev1.EventTypeNormal, "StorageAttachmentRepairOperationCompleted", message
	case "Failed":
		return corev1.EventTypeWarning, "StorageAttachmentRepairOperationFailed", message
	default:
		return "", "", ""
	}
}

func appendAuditCorrelation(message, auditCorrelationID string) string {
	auditCorrelationID = strings.TrimSpace(auditCorrelationID)
	if auditCorrelationID == "" {
		return message
	}
	return message + " auditCorrelationID=" + auditCorrelationID
}

