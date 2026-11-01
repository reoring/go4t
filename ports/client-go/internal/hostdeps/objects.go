package hostdeps

import (
 "context"
 "fmt"
 "reflect"

 "gofx/rt"
 apimeta "k8s.io/apimachinery/pkg/api/meta"
 "k8s.io/apimachinery/pkg/runtime"
)

func ObjectDeepCopy(value any) any {
 return rt.IO(func(ctx context.Context)(any,error){
  object:=To(reflect.TypeFor[runtime.Object](),rt.SomeOf(value),ctx).Interface().(runtime.Object)
  return objectOption(object.DeepCopyObject()),nil
 })
}
func RawExtensionDeepCopy(value any) any {
 return rt.IO(func(context.Context)(any,error){
  extension:=checked(value,"k8s.io/apimachinery/pkg/runtime.RawExtension").Interface().(runtime.RawExtension)
  return Value{reflect.ValueOf(*extension.DeepCopy())},nil
 })
}

func ObjectEqual(left,right any) any {
 return rt.IO(func(ctx context.Context)(any,error){
  typ:=reflect.TypeFor[runtime.Object]()
  return reflect.DeepEqual(To(typ,left,ctx).Interface(),To(typ,right,ctx).Interface()),nil
 })
}
func RawExtensionToObject(value any) any {
 return rt.IO(func(context.Context)(any,error){
  in:=checked(value,"k8s.io/apimachinery/pkg/runtime.RawExtension").Interface().(runtime.RawExtension)
  var out runtime.Object
  err:=runtime.Convert_runtime_RawExtension_To_runtime_Object(&in,&out,nil)
  return rt.Tuple2Of(objectOption(out),errorOption(err)),nil
 })
}
func ObjectToRawExtension(value,previous any) any {
 return rt.IO(func(ctx context.Context)(any,error){
  var in *runtime.Object
  if inner,present:=option(value);present{
   var object runtime.Object
   converted:=To(reflect.TypeFor[runtime.Object](),inner,ctx)
   if !converted.IsNil(){object=converted.Interface().(runtime.Object)}
   in=&object
  }
  out:=checked(previous,"k8s.io/apimachinery/pkg/runtime.RawExtension").Interface().(runtime.RawExtension)
  err:=runtime.Convert_runtime_Object_To_runtime_RawExtension(in,&out,nil)
  return rt.Tuple2Of(Value{reflect.ValueOf(out)},errorOption(err)),nil
 })
}

func ObjectResourceVersion(value any) any {
 return rt.IO(func(ctx context.Context)(any,error){
  object:=To(reflect.TypeFor[runtime.Object](),rt.SomeOf(value),ctx).Interface().(runtime.Object)
  accessor,err:=apimeta.Accessor(object)
  if err!=nil{return rt.Tuple2Of("",errorOption(err)),nil}
  return rt.Tuple2Of(accessor.GetResourceVersion(),rt.NoneOf()),nil
 })
}
func ObjectAsStatus(value any) any {return Cast(value,"k8s.io/apimachinery/pkg/apis/meta/v1.Status")}

func ObjectGoString(value any) any {
 return rt.IO(func(ctx context.Context)(any,error){
  object:=To(reflect.TypeFor[runtime.Object](),value,ctx).Interface()
  return fmt.Sprintf("%#v",object),nil
 })
}
