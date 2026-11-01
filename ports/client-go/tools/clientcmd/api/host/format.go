// Copyright 2014 The Kubernetes Authors. Licensed under Apache-2.0.
package host

import (
 "context"
 "fmt"
 "reflect"

 "github.com/reoring/gofx/ports/client-go/host/deps"
 "k8s.io/apimachinery/pkg/runtime"
)

// These adapters format only Go primitive/dependency values. Native records,
// collection traversal, error selection and diagnostics belong to .gfx.
func FormatBytes(value any) any {
 return fmt.Sprint(hostdeps.To(reflect.TypeFor[[]byte](), value, context.Background()).Interface())
}
func FormatRawExtension(value any) any {
 return fmt.Sprint(hostdeps.To(reflect.TypeFor[runtime.RawExtension](), value, context.Background()).Interface())
}
