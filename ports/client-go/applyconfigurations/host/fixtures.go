// Copyright The Kubernetes Authors. Licensed under Apache-2.0.
package host
import (
 "context"
 "encoding/json"
 "fmt"
 "reflect"
 "gofx/rt"
 "github.com/reoring/gofx/ports/client-go/host/deps"
 corev1 "k8s.io/api/core/v1"
)
// PodFromJSON is a typed fixture decoder, not a client-go implementation.
func PodFromJSON(raw any) any {return rt.IO(func(context.Context)(any,error){var pod corev1.Pod;err:=json.Unmarshal([]byte(raw.(string)),&pod);if err!=nil{return result(rt.NoneOf(),err),nil};return result(rt.SomeOf(hostdeps.From(reflect.ValueOf(pod))),nil),nil})}
func CatchPanic(action any) (result any) {defer func(){if p:=recover();p!=nil{result=rt.SomeOf(fmt.Sprint(p))}}();action.(func() any)();return rt.NoneOf()}
