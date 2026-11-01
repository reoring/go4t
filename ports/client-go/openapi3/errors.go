// Copyright 2023 The Kubernetes Authors. Licensed under Apache-2.0.
package openapi3
import (
 "context"
 "fmt"
 "reflect"
 deps "github.com/reoring/gofx/ports/client-go/host/deps"
 "k8s.io/apimachinery/pkg/runtime/schema"
)
type GroupVersionNotFoundError struct { GroupVersion schema.GroupVersion }
func (e *GroupVersionNotFoundError) Error() string {return fmt.Sprintf("GroupVersion (%v) not found as OpenAPI V3 path",e.GroupVersion)}
func NotFound(value any) any {gv:=deps.To(reflect.TypeFor[schema.GroupVersion](),value,context.Background()).Interface().(schema.GroupVersion);return deps.ErrorFromGo(&GroupVersionNotFoundError{gv})}
func IsNotFound(value any) bool {_,ok:=deps.ErrorToGo(value).(*GroupVersionNotFoundError);return ok}
