// Package fetch contains only the controller-runtime reads that go4t cannot
// express: Unstructured has no generated upcast to client.Object/ObjectList.
package fetch

import (
	"context"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// Get returns the raw object, not-found flag, and read-error flag.
// It does not extract any fields or construct decision inputs.
func Get(ctx context.Context, c client.Client, namespace, name, kind string) (*unstructured.Unstructured, bool, bool) {
	obj := &unstructured.Unstructured{}
	obj.SetGroupVersionKind(schema.GroupVersionKind{Group: "storage.appthrust.io", Version: "v1alpha1", Kind: kind})
	err := c.Get(ctx, client.ObjectKey{Namespace: namespace, Name: name}, obj)
	return obj, apierrors.IsNotFound(err), err != nil
}

// ListOperations returns the read-error flag and unfiltered namespace list.
func ListOperations(ctx context.Context, c client.Client, namespace string) (bool, *unstructured.UnstructuredList) {
	objects := &unstructured.UnstructuredList{}
	objects.SetGroupVersionKind(schema.GroupVersionKind{Group: "storage.appthrust.io", Version: "v1alpha1", Kind: "StorageResizeOperationList"})
	err := c.List(ctx, objects, client.InNamespace(namespace))
	return err != nil, objects
}
