package storage

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func newProviderRecoveryCapability(namespace, name string, ready bool) *unstructured.Unstructured {
	capability := newStorageCapabilityObject()
	capability.SetNamespace(namespace)
	capability.SetName(name)
	capability.Object["spec"] = map[string]any{
		"provider": "RookCeph",
		"mode":     "DedicatedProvider",
		"features": map[string]any{
			"block":  true,
			"object": true,
		},
		"rookCeph": map[string]any{
			"namespace":                    "rook-ceph",
			"clusterName":                  "rook-ceph",
			"storageClassName":             defaultRookStorageClassName,
			"objectStoreName":              "appthrust-object-store",
			"objectBucketStorageClassName": defaultRookObjectBucketStorage,
		},
	}
	conditions := []metav1.Condition{
		condition(capability, "Ready", metav1.ConditionFalse, "WaitingForProviderResources", "provider is not ready"),
		condition(capability, "CephClusterReady", metav1.ConditionFalse, "CephClusterNotHealthy", "provider data plane is not ready"),
	}
	if ready {
		conditions[0] = condition(capability, "Ready", metav1.ConditionTrue, "Ready", "provider is ready")
		conditions[1] = condition(capability, "CephClusterReady", metav1.ConditionTrue, "CephClusterHealthy", "provider data plane is ready")
	}
	capability.Object["status"] = map[string]any{
		"conditions": conditionsToUnstructured(conditions),
	}
	return capability
}

func hasConditionReason(conditions []any, conditionType, reason string) bool {
	for _, item := range conditions {
		condition, ok := item.(map[string]any)
		if !ok {
			continue
		}
		if condition["type"] == conditionType && condition["reason"] == reason {
			return true
		}
	}
	return false
}
