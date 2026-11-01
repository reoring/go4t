// Copyright 2026 The Kubernetes Authors. Licensed under Apache-2.0.
package host

import (
 "context"
 "reflect"
 "go4t/rt"
 deps "github.com/reoring/gofx/ports/client-go/host/deps"
 "k8s.io/apimachinery/pkg/runtime"
)
func boxedObjectOption(object runtime.Object) any {
 return deps.From(reflect.ValueOf(&object).Elem())
}
func boxedObject(object runtime.Object) any {
 value,present:=rt.OptionValue(boxedObjectOption(object))
 if !present {panic("nil runtime.Object at required object boundary")}
 return value
}
// DecodeInto retains every independently observable external decoder output,
// including a partially mutated destination when a different object is returned.
func DecodeInto(decoder,data,destination any) any {return rt.IO(func(ctx context.Context)(any,error){
 d:=deps.To(reflect.TypeFor[runtime.Decoder](),rt.SomeOf(decoder),ctx).Interface().(runtime.Decoder)
 original:=deps.To(reflect.TypeFor[runtime.Object](),rt.SomeOf(destination),ctx).Interface().(runtime.Object)
 target:=original.DeepCopyObject()
 decoded,_,err:=d.Decode([]byte(data.(string)),nil,target)
 return rt.Tuple2Of(boxedObject(target),rt.Tuple2Of(boxedObjectOption(decoded),rt.Tuple2Of(decoded==target,errOption(err)))),nil
})}
