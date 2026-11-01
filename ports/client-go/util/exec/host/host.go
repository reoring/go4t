// Copyright 2014 The Kubernetes Authors. Licensed under Apache-2.0.
package host
import("context";"reflect";"go4t/rt";"github.com/reoring/gofx/ports/client-go/host/deps")
func ErrorMessage(value any) any{return rt.IO(func(ctx context.Context)(any,error){err:=hostdeps.To(reflect.TypeFor[error](),rt.SomeOf(value),ctx).Interface().(error);return err.Error(),nil})}
