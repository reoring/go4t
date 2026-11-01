// Copyright 2026 The gofx Authors.
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"context"
	"fmt"
	"net/http"
	"reflect"

	corev1 "k8s.io/api/core/v1"
	autoscalingv1 "k8s.io/api/autoscaling/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	applyautoscaling "k8s.io/client-go/applyconfigurations/autoscaling/v1"
	applycore "k8s.io/client-go/applyconfigurations/core/v1"
	"k8s.io/client-go/kubernetes"
	fakeclientset "k8s.io/client-go/kubernetes/fake"
	clienttesting "k8s.io/client-go/testing"
)

var fixtures = []struct {
	id string
	run func(*scenario) error
}{
	{"typed_pod_crud_status_collection_patch", typedPod},
	{"typed_resize_explicit_parent_name", resizePod},
	{"apply_scale_without_configuration_name", applyScale},
	{"real_and_fake_bind_namespace", bindNamespace},
	{"fake_namespace_finalize", finalizeNamespace},
	{"fake_list_label_not_field_filter", fakeList},
	{"fake_action_snapshot_and_reactor_mutation", actionSnapshot},
	{"apply_builder_presence_merge_append", builders},
	{"field_managed_clientset_conflicts", managedFields},
}

func pod(name, namespace string) *corev1.Pod {
	return &corev1.Pod{
		TypeMeta: metav1.TypeMeta{APIVersion: "v1", Kind: "Pod"},
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: namespace, ResourceVersion: "17"},
		Spec: corev1.PodSpec{Containers: []corev1.Container{{Name: "main", Image: "image:fixed"}}},
	}
}

func typedPod(s *scenario) error {
	fixture := pod("pod-a", "team-a")
	seconds := int64(21)
	grace := int64(0)
	uid := types.UID("fixture-uid")
	status := &metav1.Status{TypeMeta: metav1.TypeMeta{APIVersion: "v1", Kind: "Status"}, Status: metav1.StatusSuccess, Code: 200}
	missing := apierrors.NewNotFound(corev1.Resource("pods"), "missing").ErrStatus
	missing.APIVersion = "v1"; missing.Kind = "Status"
	exchanges := []exchange{
		{method: "POST", path: "/api/v1/namespaces/team-a/pods", response: fixture, status: http.StatusCreated},
		{method: "GET", path: "/api/v1/namespaces/team-a/pods/pod-a", response: fixture},
		{method: "GET", path: "/api/v1/namespaces/team-a/pods", response: &corev1.PodList{TypeMeta: metav1.TypeMeta{APIVersion: "v1", Kind: "PodList"}, ListMeta: metav1.ListMeta{ResourceVersion: "17"}, Items: []corev1.Pod{*fixture}}},
		{method: "PUT", path: "/api/v1/namespaces/team-a/pods/pod-a", response: fixture},
		{method: "PUT", path: "/api/v1/namespaces/team-a/pods/pod-a/status", response: fixture},
		{method: "PATCH", path: "/api/v1/namespaces/team-a/pods/pod-a/status", response: fixture},
		{method: "DELETE", path: "/api/v1/namespaces/team-a/pods/pod-a", response: status},
		{method: "DELETE", path: "/api/v1/namespaces/team-a/pods", response: status},
		{method: "GET", path: "/api/v1/namespaces/team-a/pods/missing", response: &missing, status: http.StatusNotFound},
	}
	return withHTTP(s, exchanges, func(ctx context.Context, client *kubernetes.Clientset) error {
		pods := client.CoreV1().Pods("team-a")
		if err := require(fixture.Name == "pod-a" && fixture.Namespace == "team-a", "invalid Pod fixture identity"); err != nil { return err }
		result, err := pods.Create(ctx, fixture.DeepCopy(), metav1.CreateOptions{FieldManager: "creator", DryRun: []string{metav1.DryRunAll}, FieldValidation: "Strict"})
		if err != nil { return err }
		if err := observe(s, "typed_result", map[string]any{"operation": "create", "object": result}); err != nil { return err }
		result, err = pods.Get(ctx, "pod-a", metav1.GetOptions{ResourceVersion: "17"})
		if err != nil { return err }
		if err := require(result.Name == fixture.Name, "Get did not decode the response object"); err != nil { return err }
		if err := observe(s, "typed_result", map[string]any{"operation": "get", "object": result}); err != nil { return err }
		list, err := pods.List(ctx, metav1.ListOptions{LabelSelector: "app=demo", FieldSelector: "metadata.name=pod-a", ResourceVersion: "17", TimeoutSeconds: &seconds, Limit: 7, Continue: "fixture-token"})
		if err != nil { return err }
		if err := require(len(list.Items) == 1 && list.ResourceVersion == "17", "invalid decoded list fixture"); err != nil { return err }
		if err := observe(s, "typed_result", map[string]any{"operation": "list", "object": list}); err != nil { return err }
		result, err = pods.Update(ctx, fixture.DeepCopy(), metav1.UpdateOptions{FieldManager: "updater", DryRun: []string{metav1.DryRunAll}})
		if err != nil { return err }
		if err := observe(s, "typed_result", map[string]any{"operation": "update", "object": result}); err != nil { return err }
		statusPod := fixture.DeepCopy(); statusPod.Status.Phase = corev1.PodRunning
		result, err = pods.UpdateStatus(ctx, statusPod, metav1.UpdateOptions{FieldManager: "status-manager"})
		if err != nil { return err }
		if err := observe(s, "typed_result", map[string]any{"operation": "update_status", "object": result}); err != nil { return err }
		result, err = pods.Patch(ctx, "pod-a", types.MergePatchType, []byte(`{"status":{"phase":"Running"}}`), metav1.PatchOptions{FieldManager: "patcher", FieldValidation: "Strict"}, "status")
		if err != nil { return err }
		if err := observe(s, "typed_result", map[string]any{"operation": "patch_status", "object": result}); err != nil { return err }
		deleteOptions := metav1.DeleteOptions{GracePeriodSeconds: &grace, Preconditions: &metav1.Preconditions{UID: &uid}, DryRun: []string{metav1.DryRunAll}}
		if err := pods.Delete(ctx, "pod-a", deleteOptions); err != nil { return err }
		if err := observe(s, "operation_result", map[string]any{"operation": "delete", "error": nil}); err != nil { return err }
		if err := pods.DeleteCollection(ctx, deleteOptions, metav1.ListOptions{LabelSelector: "app=demo", FieldSelector: "metadata.name=pod-a", TimeoutSeconds: &seconds}); err != nil { return err }
		if err := observe(s, "operation_result", map[string]any{"operation": "delete_collection", "error": nil}); err != nil { return err }
		result, err = pods.Get(ctx, "missing", metav1.GetOptions{})
		if err := require(apierrors.IsNotFound(err), "missing fixture did not return structured NotFound: %v", err); err != nil { return err }
		fields := errorFields(err); fields["operation"] = "get_missing"; fields["object"] = result
		return observe(s, "api_error", fields)
	})
}

func resizePod(s *scenario) error {
	body := pod("body-name", "body-namespace")
	return withHTTP(s, []exchange{{method: "PUT", path: "/api/v1/namespaces/client-namespace/pods/parent-name/resize", response: body}}, func(ctx context.Context, client *kubernetes.Clientset) error {
		if err := require(body.Name != "parent-name" && body.Namespace != "client-namespace", "resize identities must differ"); err != nil { return err }
		result, err := client.CoreV1().Pods("client-namespace").UpdateResize(ctx, "parent-name", body, metav1.UpdateOptions{FieldManager: "resizer"})
		if err != nil { return err }
		return observe(s, "typed_result", map[string]any{"operation": "resize", "object": result})
	})
}

func applyScale(s *scenario) error {
	configuration := applyautoscaling.Scale().WithSpec(applyautoscaling.ScaleSpec().WithReplicas(3))
	if err := require(configuration.ObjectMetaApplyConfiguration == nil, "scale configuration must omit metadata and name"); err != nil { return err }
	response := &autoscalingv1.Scale{TypeMeta: metav1.TypeMeta{APIVersion: "autoscaling/v1", Kind: "Scale"}, ObjectMeta: metav1.ObjectMeta{Name: "parent-name", Namespace: "team-a"}, Spec: autoscalingv1.ScaleSpec{Replicas: 3}}
	return withHTTP(s, []exchange{{method: "PATCH", path: "/apis/apps/v1/namespaces/team-a/deployments/parent-name/scale", response: response}}, func(ctx context.Context, client *kubernetes.Clientset) error {
		result, err := client.AppsV1().Deployments("team-a").ApplyScale(ctx, "parent-name", configuration, metav1.ApplyOptions{FieldManager: "scale-manager", Force: true})
		if err != nil { return err }
		if err := require(result.Spec.Replicas == 3, "scale response not decoded"); err != nil { return err }
		return observe(s, "typed_result", map[string]any{"operation": "apply_scale", "configuration_name_absent": configuration.ObjectMetaApplyConfiguration == nil || configuration.Name == nil, "object": result})
	})
}

func bindNamespace(s *scenario) error {
	binding := &corev1.Binding{TypeMeta: metav1.TypeMeta{APIVersion: "v1", Kind: "Binding"}, ObjectMeta: metav1.ObjectMeta{Name: "pod-a", Namespace: "body-namespace"}, Target: corev1.ObjectReference{Kind: "Node", Name: "node-a"}}
	if err := require(binding.Namespace != "client-namespace", "Bind namespaces must differ"); err != nil { return err }
	status := &metav1.Status{TypeMeta: metav1.TypeMeta{APIVersion: "v1", Kind: "Status"}, Status: metav1.StatusSuccess, Code: 201}
	if err := withHTTP(s, []exchange{{method: "POST", path: "/api/v1/namespaces/client-namespace/pods/pod-a/binding", response: status, status: 201}}, func(ctx context.Context, client *kubernetes.Clientset) error {
		err := client.CoreV1().Pods("client-namespace").Bind(ctx, binding, metav1.CreateOptions{FieldManager: "binder"})
		if err != nil { return err }
		return observe(s, "operation_result", map[string]any{"implementation": "real", "error": nil})
	}); err != nil { return err }
	client := &fakeclientset.Clientset{}
	if err := client.CoreV1().Pods("client-namespace").Bind(context.Background(), binding, metav1.CreateOptions{FieldManager: "binder"}); err != nil { return err }
	actions := client.Actions()
	if err := require(len(actions) == 1, "fake Bind recorded %d actions", len(actions)); err != nil { return err }
	if err := require(actions[0].GetNamespace() == binding.Namespace, "fake Bind did not use body namespace"); err != nil { return err }
	return observeAction(s, "fake_bind", actions[0])
}

func finalizeNamespace(s *scenario) error {
	client := &fakeclientset.Clientset{}
	body := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "namespace-a"}, Spec: corev1.NamespaceSpec{Finalizers: []corev1.FinalizerName{corev1.FinalizerKubernetes}}}
	if err := require(body.Name != "", "Finalize requires named fixture"); err != nil { return err }
	result, err := client.CoreV1().Namespaces().Finalize(context.Background(), body, metav1.UpdateOptions{FieldManager: "finalizer"})
	if err != nil { return err }
	actions := client.Actions()
	if err := require(len(actions) == 1 && actions[0].GetVerb() == "create" && actions[0].GetSubresource() == "finalize", "unexpected Finalize action: %v", actions); err != nil { return err }
	if err := observeAction(s, "finalize", actions[0]); err != nil { return err }
	return observe(s, "typed_result", map[string]any{"object": result})
}

func fakeList(s *scenario) error {
	match := pod("label-match", "team-a"); match.Labels = map[string]string{"app": "selected"}
	other := pod("field-match", "team-a"); other.Labels = map[string]string{"app": "other"}
	outside := pod("outside", "team-b"); outside.Labels = map[string]string{"app": "selected"}
	if err := require(match.Name != other.Name && match.Labels["app"] != other.Labels["app"], "label and field selector fixture must disagree"); err != nil { return err }
	client := fakeclientset.NewSimpleClientset(match, other, outside)
	options := metav1.ListOptions{LabelSelector: "app=selected", FieldSelector: "metadata.name=field-match"}
	result, err := client.CoreV1().Pods("team-a").List(context.Background(), options)
	if err != nil { return err }
	if err := require(len(result.Items) == 1 && result.Items[0].Name == "label-match", "fake List did not filter by label while ignoring field selector: %+v", result.Items); err != nil { return err }
	actions := client.Actions()
	if err := require(len(actions) == 1, "fake List recorded %d actions", len(actions)); err != nil { return err }
	if err := observeAction(s, "list", actions[0]); err != nil { return err }
	return observe(s, "typed_result", map[string]any{"object": result})
}

func actionSnapshot(s *scenario) error {
	client := &fakeclientset.Clientset{}
	body := pod("pod-a", "team-a")
	if err := require(body.SelfLink == "", "snapshot fixture must begin unmutated"); err != nil { return err }
	var observationError error
	seenSecond := false
	client.AddReactor("create", "pods", func(action clienttesting.Action) (bool, runtime.Object, error) {
		create, ok := action.(clienttesting.CreateAction)
		if !ok { return true, nil, fmt.Errorf("expected CreateAction, got %T", action) }
		object, ok := create.GetObject().(*corev1.Pod)
		if !ok { return true, nil, fmt.Errorf("expected Pod, got %T", create.GetObject()) }
		object.SelfLink = "reactor-mutated"
		observationError = observeAction(s, "first_reactor_after_mutation", action)
		return false, object, observationError
	})
	client.AddReactor("create", "pods", func(action clienttesting.Action) (bool, runtime.Object, error) {
		object := action.(clienttesting.CreateAction).GetObject().(*corev1.Pod)
		seenSecond = object.SelfLink == "reactor-mutated"
		if !seenSecond { return true, nil, fmt.Errorf("second reactor did not see first mutation") }
		observationError = observeAction(s, "second_reactor", action)
		return true, object, observationError
	})
	result, err := client.CoreV1().Pods("team-a").Create(context.Background(), body, metav1.CreateOptions{FieldManager: "creator"})
	if err != nil { return err }
	if observationError != nil { return observationError }
	actions := client.Actions()
	if err := require(seenSecond && len(actions) == 1 && result.SelfLink == "reactor-mutated" && body.SelfLink == "", "snapshot or reactor mutation invariant failed"); err != nil { return err }
	recorded := actions[0].(clienttesting.CreateAction).GetObject().(*corev1.Pod)
	if err := require(recorded.SelfLink == "", "recorded action was changed by reactor"); err != nil { return err }
	if err := observeAction(s, "recorded_action", actions[0]); err != nil { return err }
	return observe(s, "typed_result", map[string]any{"input": body, "returned": result})
}

func builders(s *scenario) error {
	configuration := &applycore.PodApplyConfiguration{}
	if err := require(configuration.Spec == nil && configuration.ObjectMetaApplyConfiguration == nil, "builder fixture not unset"); err != nil { return err }
	if err := observe(s, "apply_configuration", map[string]any{"stage": "unset", "object": configuration}); err != nil { return err }
	configuration = applycore.Pod("pod-a", "team-a").WithSpec(applycore.PodSpec().WithHostNetwork(false).WithTerminationGracePeriodSeconds(0).WithNodeName(""))
	if err := require(configuration.Spec.HostNetwork != nil && !*configuration.Spec.HostNetwork && configuration.Spec.TerminationGracePeriodSeconds != nil && *configuration.Spec.TerminationGracePeriodSeconds == 0 && configuration.Spec.NodeName != nil && *configuration.Spec.NodeName == "", "explicit zero/false/empty presence lost"); err != nil { return err }
	if err := observe(s, "apply_configuration", map[string]any{"stage": "explicit_zero_false_empty", "object": configuration}); err != nil { return err }
	configuration.WithLabels(map[string]string{"first": "one", "shared": "before"}).WithLabels(map[string]string{"second": "two", "shared": "after"}).WithFinalizers("first").WithFinalizers("second", "third")
	if err := require(reflect.DeepEqual(configuration.Labels, map[string]string{"first": "one", "second": "two", "shared": "after"}) && reflect.DeepEqual(configuration.Finalizers, []string{"first", "second", "third"}), "map merge or list append lost"); err != nil { return err }
	configuration.WithSpec(configuration.Spec.WithContainers(applycore.Container().WithName("first")).WithContainers(applycore.Container().WithName("second")))
	if err := require(len(configuration.Spec.Containers) == 2, "nested builder append lost"); err != nil { return err }
	if err := observe(s, "apply_configuration", map[string]any{"stage": "merged_and_appended", "object": configuration}); err != nil { return err }
	var recovered any
	func() {
		defer func() { recovered = recover() }()
		configuration.WithOwnerReferences(nil)
	}()
	if err := require(recovered != nil, "nil owner reference did not panic"); err != nil { return err }
	return observe(s, "builder_panic", map[string]any{"message": fmt.Sprint(recovered), "go_type": fmt.Sprintf("%T", recovered)})
}

func managedFields(s *scenario) error {
	client := fakeclientset.NewClientset()
	ctx := context.Background()
	name, namespace := "config-a", "team-a"
	create := &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: namespace}, Data: map[string]string{"k0": "v0"}}
	if err := require(len(create.ManagedFields) == 0, "managed-field fixture must start without ownership"); err != nil { return err }
	result, err := client.CoreV1().ConfigMaps(namespace).Create(ctx, create, metav1.CreateOptions{FieldManager: "manager-0"})
	if err != nil { return err }
	capture := func(stage string, object *corev1.ConfigMap, expected map[string]string) error {
		if err := require(reflect.DeepEqual(object.Data, expected), "stage %s: unexpected data: %v", stage, object.Data); err != nil { return err }
		// Field-manager timestamps are excluded; ownership and FieldsV1 are retained.
		ownership := make([]map[string]any, 0, len(object.ManagedFields))
		for _, entry := range object.ManagedFields {
			ownership = append(ownership, map[string]any{"manager": entry.Manager, "operation": entry.Operation, "api_version": entry.APIVersion, "fields_type": entry.FieldsType, "fields_v1": entry.FieldsV1, "subresource": entry.Subresource})
		}
		return observe(s, "managed_object", map[string]any{"stage": stage, "data": object.Data, "ownership": ownership})
	}
	if err := capture("create", result, map[string]string{"k0": "v0"}); err != nil { return err }
	apply := func(manager string, data map[string]string, force bool) (*corev1.ConfigMap, error) {
		return client.CoreV1().ConfigMaps(namespace).Apply(ctx, applycore.ConfigMap(name, namespace).WithData(data), metav1.ApplyOptions{FieldManager: manager, Force: force})
	}
	result, err = apply("manager-1", map[string]string{"k1": "v1"}, false)
	if err != nil { return err }
	if err := capture("apply_manager_1", result, map[string]string{"k0": "v0", "k1": "v1"}); err != nil { return err }
	_, err = apply("manager-2", map[string]string{"k1": "conflict"}, false)
	if err := require(apierrors.IsConflict(err), "expected managed-field conflict, got %v", err); err != nil { return err }
	if err := observe(s, "api_error", errorFields(err)); err != nil { return err }
	result, err = apply("manager-2", map[string]string{"k2": "v2"}, false)
	if err != nil { return err }
	if err := capture("apply_manager_2", result, map[string]string{"k0": "v0", "k1": "v1", "k2": "v2"}); err != nil { return err }
	result, err = apply("manager-1", map[string]string{"k1": "updated"}, false)
	if err != nil { return err }
	if err := capture("update_owned_field", result, map[string]string{"k0": "v0", "k1": "updated", "k2": "v2"}); err != nil { return err }
	result, err = apply("manager-2", map[string]string{"k1": "forced"}, true)
	if err != nil { return err }
	return capture("force_and_omit_previous_field", result, map[string]string{"k0": "v0", "k1": "forced"})
}
