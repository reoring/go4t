package storage

import (
	"context"
	"fmt"
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
	StorageResizePlanKind = "StorageResizePlan"
	StorageResizeOperationKind = "StorageResizeOperation"
	StorageAutopilotPolicyKind = "StorageAutopilotPolicy"
)

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

var (
	storageResizePlanResourceGVK = schema.GroupVersionKind{
		Group:   StorageCapabilityGroup,
		Version: StorageCapabilityVersion,
		Kind:    StorageResizePlanKind,
	}
	storageMaintenanceWindowResourceGVK = schema.GroupVersionKind{
		Group:   StorageCapabilityGroup,
		Version: StorageCapabilityVersion,
		Kind:    "StorageMaintenanceWindow",
	}
)

func newStorageResizePlanObject() *unstructured.Unstructured {
	obj := &unstructured.Unstructured{}
	obj.SetGroupVersionKind(storageResizePlanResourceGVK)
	return obj
}

func newStorageMaintenanceWindowObject() *unstructured.Unstructured {
	obj := &unstructured.Unstructured{}
	obj.SetGroupVersionKind(storageMaintenanceWindowResourceGVK)
	return obj
}

var storageResizeOperationResourceGVK = schema.GroupVersionKind{
	Group:   StorageCapabilityGroup,
	Version: StorageCapabilityVersion,
	Kind:    StorageResizeOperationKind,
}

var storageResizeOperationListResourceGVK = schema.GroupVersionKind{
	Group:   StorageCapabilityGroup,
	Version: StorageCapabilityVersion,
	Kind:    StorageResizeOperationKind + "List",
}

func isTerminalStorageResizeOperationPhase(phase string) bool {
	switch phase {
	case "Succeeded", "Failed", "Canceled", "RolledBack":
		return true
	default:
		return false
	}
}

func newStorageResizeOperationObject() *unstructured.Unstructured {
	obj := &unstructured.Unstructured{}
	obj.SetGroupVersionKind(storageResizeOperationResourceGVK)
	return obj
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

func firstNestedStringSlice(obj map[string]any, paths ...[]string) ([]string, bool) {
	for _, path := range paths {
		values, ok, _ := unstructured.NestedStringSlice(obj, path...)
		if ok {
			return values, true
		}
		items, ok, _ := unstructured.NestedSlice(obj, path...)
		if !ok {
			continue
		}
		stringValues := make([]string, 0, len(items))
		for _, item := range items {
			value, ok := item.(string)
			if ok {
				stringValues = append(stringValues, value)
			}
		}
		return stringValues, true
	}
	return nil, false
}

func newStorageAutopilotPolicyObject() *unstructured.Unstructured {
	obj := &unstructured.Unstructured{}
	obj.SetGroupVersionKind(storageAutopilotPolicyGVK)
	return obj
}

var storageAutopilotPolicyGVK = schema.GroupVersionKind{
	Group:   StorageCapabilityGroup,
	Version: StorageCapabilityVersion,
	Kind:    StorageAutopilotPolicyKind,
}

func recordStorageWorkflowEvent(recorder record.EventRecorder, obj *unstructured.Unstructured, eventType, reason, message string) {
	if recorder == nil || obj == nil || reason == "" {
		return
	}
	recorder.Event(obj, eventType, reason, message)
}

func storageApprovalEvent(approval *unstructured.Unstructured, status map[string]any) (string, string, string) {
	phase := fmt.Sprint(status["phase"])
	message := strings.TrimSpace(fmt.Sprint(status["message"]))
	if message == "" {
		message = "Storage approval status changed"
	}
	message = appendAuditCorrelation(message, firstNonEmptyNestedString(approval.Object, []string{"spec", "auditCorrelationID"}))
	switch phase {
	case "Accepted":
		return corev1.EventTypeNormal, "StorageApprovalAccepted", message
	case "Rejected":
		return corev1.EventTypeWarning, "StorageApprovalRejected", message
	case "Expired":
		return corev1.EventTypeWarning, "StorageApprovalExpired", message
	case "Superseded":
		return corev1.EventTypeWarning, "StorageApprovalSuperseded", message
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

func mapStringDefault(obj map[string]any, key string) string {
	if obj == nil {
		return ""
	}
	value, _ := obj[key].(string)
	return value
}

