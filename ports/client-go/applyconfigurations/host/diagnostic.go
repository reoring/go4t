// Copyright The Kubernetes Authors. Licensed under Apache-2.0.
package host
import (
 "context"
 "go4t/rt"
 "k8s.io/apimachinery/pkg/runtime/serializer/cbor/direct"
)
func Diagnose(value any) any {return rt.IO(func(context.Context)(any,error){text,err:=direct.Diagnose([]byte(value.(string)));return result(text,err),nil})}
