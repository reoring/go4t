package storage

import (
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

const (
	StorageCapabilityGroup     = "storage.appthrust.io"
	StorageCapabilityVersion   = "v1alpha1"
	StorageResizePlanKind      = "StorageResizePlan"
	StorageResizeOperationKind = "StorageResizeOperation"
	StorageAutopilotPolicyKind = "StorageAutopilotPolicy"
)

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

func newStorageResizeOperationObject() *unstructured.Unstructured {
	obj := &unstructured.Unstructured{}
	obj.SetGroupVersionKind(storageResizeOperationResourceGVK)
	return obj
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

func newStorageApprovalObject() *unstructured.Unstructured {
	obj := &unstructured.Unstructured{}
	obj.SetGroupVersionKind(storageApprovalResourceGVK)
	return obj
}
