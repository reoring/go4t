// Copyright The Kubernetes Authors. Licensed under Apache-2.0.
package host

import (
 "context"
 "fmt"
 "reflect"
 deps "github.com/reoring/gofx/ports/client-go/host/deps"
 "k8s.io/apimachinery/pkg/runtime"
)

// ObjectTypeName observes external nominal identity; no client-go policy or implementation.
func ObjectTypeName(value any) string {
 object:=deps.To(reflect.TypeFor[runtime.Object](),value,context.Background()).Interface()
 return fmt.Sprintf("%T",object)
}
