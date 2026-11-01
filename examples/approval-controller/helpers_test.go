package storage

import (
	"fmt"
	"testing"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	kruntime "k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func newAutopilotTestClient(t *testing.T, objects ...client.Object) client.Client {
	t.Helper()
	scheme := kruntime.NewScheme()
	scheme.AddKnownTypeWithName(storageMaintenanceWindowResourceGVK, &unstructured.Unstructured{})
	scheme.AddKnownTypeWithName(storageMaintenanceWindowResourceGVK.GroupVersion().WithKind("StorageMaintenanceWindowList"), &unstructured.UnstructuredList{})
	for _, kind := range []string{"StaticHostPool", "BareMetalPool"} {
		gvk := schema.GroupVersionKind{Group: "storage.appthrust.io", Version: "v1alpha1", Kind: kind}
		scheme.AddKnownTypeWithName(gvk, &unstructured.Unstructured{})
		scheme.AddKnownTypeWithName(gvk.GroupVersion().WithKind(kind+"List"), &unstructured.UnstructuredList{})
	}
	return fake.NewClientBuilder().WithScheme(scheme).WithObjects(objects...).Build()
}

func newStorageAutopilotPolicy(namespace, name, storageCapabilityName string) *unstructured.Unstructured {
	policy := newStorageAutopilotPolicyObject()
	policy.SetNamespace(namespace)
	policy.SetName(name)
	policy.SetGeneration(1)
	policy.Object["spec"] = map[string]any{
		"mode": "ObserveOnly",
		"storageCapabilityRef": map[string]any{
			"name": storageCapabilityName,
		},
		"capacity": map[string]any{
			"minMonitorQuorum":            int64(3),
			"minFailureDomains":           int64(3),
			"minOSDs":                     int64(3),
			"minPoolReplicationSize":      int64(3),
			"targetRawUtilizationPercent": int64(70),
			"hardRawUtilizationPercent":   int64(85),
		},
		"providerCapacity": map[string]any{
			"driver":   "EKSKarpenter",
			"minNodes": int64(3),
			"maxNodes": int64(6),
		},
	}
	return policy
}

func hasConditionStatus(conditions []any, conditionType, status string) bool {
	for _, raw := range conditions {
		condition, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		if condition["type"] == conditionType && condition["status"] == status {
			return true
		}
	}
	return false
}

func newStorageResizePlan(namespace, name, capabilityName, policyName string) *unstructured.Unstructured {
	plan := newStorageResizePlanObject()
	plan.SetNamespace(namespace)
	plan.SetName(name)
	plan.SetGeneration(1)
	plan.SetCreationTimestamp(metav1.NewTime(time.Date(2026, 6, 25, 0, 0, 0, 0, time.UTC)))
	plan.Object["spec"] = map[string]any{
		"workflowChannel": "W",
		"storageCapabilityRef": map[string]any{
			"name": capabilityName,
		},
		"policyRef": map[string]any{
			"name": policyName,
		},
		"maintenanceWindowRef": map[string]any{
			"name": "window-a",
		},
		"type": "OSDScaleIn",
		"intent": map[string]any{
			"targetOSDCount":          int64(3),
			"targetProviderNodeCount": int64(3),
			"candidateSet":            []any{"candidate-7f3c8c1a"},
		},
		"baseRevision": map[string]any{
			"gitCommit": "abc123",
		},
		"expiresAfter": "24h",
		"reason":       "test resize plan",
	}
	return plan
}

func newOpenStorageMaintenanceWindow(namespace, name, policyName string) *unstructured.Unstructured {
	window := newStorageMaintenanceWindowObject()
	window.SetNamespace(namespace)
	window.SetName(name)
	window.Object["spec"] = map[string]any{
		"targetRef": map[string]any{
			"name": policyName,
		},
		"schedule": map[string]any{
			"timezone": "UTC",
			"windows": []any{
				map[string]any{
					"cron":        "0 * * * *",
					"maxDuration": "2h",
				},
			},
		},
	}
	window.Object["status"] = map[string]any{
		"phase": "Open",
		"currentWindow": map[string]any{
			"open": true,
		},
	}
	return window
}

func newStorageResizeOperation(namespace, name, phase string, stageCount int) *unstructured.Unstructured {
	obj := newStorageResizeOperationObject()
	obj.SetNamespace(namespace)
	obj.SetName(name)
	obj.Object["spec"] = map[string]any{
		"workflowChannel":      "W",
		"type":                 "OSDScaleIn",
		"planRef":              map[string]any{"name": "rook-ceph-plan"},
		"storageCapabilityRef": map[string]any{"name": "rook-ceph"},
		"executionProfile":     "ManualHold",
	}
	stages := make([]any, 0, stageCount)
	for i := 0; i < stageCount; i++ {
		stages = append(stages, map[string]any{
			"name":    fmt.Sprintf("stage-%02d", i),
			"phase":   "Succeeded",
			"message": fmt.Sprintf("stage %02d completed", i),
		})
	}
	obj.Object["status"] = map[string]any{
		"phase":  phase,
		"stages": stages,
	}
	return obj
}
