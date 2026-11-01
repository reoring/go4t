package hostdeps

import (
 "context"
 "fmt"
 "reflect"
 "sync"

 "go4t/rt"
 apimeta "k8s.io/apimachinery/pkg/api/meta"
 "k8s.io/apimachinery/pkg/runtime"
)

var runtimeObjectType = reflect.TypeFor[runtime.Object]()

// Object values are boxed once at the external ingress, never at each cast or
// upcast. Addressable fields and pointer-backed objects already own storage.
func storedValue(value reflect.Value) Value {
 if value.Kind()==reflect.Struct&&!value.CanAddr()&&(value.Type().Implements(runtimeObjectType)||reflect.PointerTo(value.Type()).Implements(runtimeObjectType)){
  owned:=reflect.New(value.Type()).Elem()
  owned.Set(value)
  value=owned
 }
 return Value{value}
}

type snapshotCopier interface { copySnapshot(context.Context) (reflect.Value,error) }

func snapshotReceiver(value reflect.Value,session *copySession,ctx context.Context) (reflect.Value,error) {
 if snapshotType(value.Type()){return session.clone(value),nil}
 source:=value
 if source.Kind()!=reflect.Pointer&&source.CanAddr()&&reflect.PointerTo(source.Type()).Implements(runtimeObjectType){source=source.Addr()}
 if source.CanInterface(){if copier,ok:=source.Interface().(snapshotCopier);ok{return copier.copySnapshot(ctx)}}
 if !source.Type().Implements(runtimeObjectType){return reflect.Value{},fmt.Errorf("hostdeps: data interface receiver has no snapshot copy contract")}
 original:=source.Interface().(runtime.Object)
 copied:=original.DeepCopyObject()
 if copied==nil{return reflect.Value{},fmt.Errorf("hostdeps: DeepCopyObject returned nil")}
 result:=reflect.ValueOf(copied)
 if result.Kind()==reflect.Pointer&&result.IsNil(){return reflect.Value{},fmt.Errorf("hostdeps: DeepCopyObject returned a nil pointer")}
 if source.Kind()==reflect.Pointer&&result.Kind()==reflect.Pointer&&source.Type()==result.Type()&&source.Pointer()==result.Pointer(){return reflect.Value{},fmt.Errorf("hostdeps: DeepCopyObject retained input identity")}
 return result,nil
}

func copyFromCallback(ctx context.Context,callback any,target reflect.Type,original reflect.Value) (reflect.Value,error) {
 action:=reflect.ValueOf(callback).Call(nil)[0].Interface().(rt.IO)
 copied,err:=action(ctx)
 if err!=nil{return reflect.Value{},err}
 candidate,present:=option(copied)
 if !present{return reflect.Value{},fmt.Errorf("hostdeps: snapshot copy callback returned nil")}
 converted:=To(target,rt.SomeOf(candidate),ctx).Elem()
 if converted.Kind()==reflect.Pointer&&converted.IsNil(){return reflect.Value{},fmt.Errorf("hostdeps: snapshot copy callback returned a nil pointer")}
 if original.Kind()==reflect.Pointer&&converted.Kind()==reflect.Pointer&&original.Type()==converted.Type()&&original.Pointer()==converted.Pointer(){return reflect.Value{},fmt.Errorf("hostdeps: snapshot copy callback retained input identity")}
 return converted,nil
}

type objectTypeDescriptor struct { typ reflect.Type }
var objectTypeDescriptors sync.Map

func ObjectDescriptor(id string) any {
 typ:=registered(id)
 if cached,ok:=objectTypeDescriptors.Load(typ);ok{return cached}
 if !typ.Implements(runtimeObjectType)&&!reflect.PointerTo(typ).Implements(runtimeObjectType){panic("hostdeps: descriptor type does not implement runtime.Object")}
 descriptor:=&objectTypeDescriptor{typ:typ}
 cached,_:=objectTypeDescriptors.LoadOrStore(typ,descriptor)
 return cached
}
func objectDescriptor(value any) *objectTypeDescriptor {
 descriptor,ok:=value.(*objectTypeDescriptor)
 if !ok||descriptor==nil{panic("hostdeps: invalid object descriptor")}
 return descriptor
}
func TypedObjectTo(descriptor,value any) any {
 expected:=objectDescriptor(descriptor).typ
 actual:=value.(Value).value
 if actual.Kind()==reflect.Pointer {
  if actual.IsNil()||actual.Type().Elem()!=expected{panic("hostdeps: object descriptor type mismatch")}
  if !actual.Type().Implements(runtimeObjectType){panic("hostdeps: value does not implement runtime.Object")}
  return Value{actual}
 }
 if actual.Type()!=expected{panic("hostdeps: object descriptor type mismatch")}
 if actual.Type().Implements(runtimeObjectType){return Value{actual}}
 if !actual.CanAddr(){panic("hostdeps: object snapshot has no owned storage")}
 return Value{actual.Addr()}
}
func TypedObjectFrom(descriptor,value any) any {
 expected:=objectDescriptor(descriptor).typ
 present,ok:=option(value)
 if !ok{return rt.NoneOf()}
 actual:=present.(Value).value
 if !actual.IsValid()||!actual.Type().Implements(runtimeObjectType){return rt.NoneOf()}
 for actual.Kind()==reflect.Interface||actual.Kind()==reflect.Pointer {
  if actual.IsNil(){return rt.NoneOf()}
  actual=actual.Elem()
 }
 if actual.Type()!=expected{return rt.NoneOf()}
 return rt.SomeOf(Value{actual})
}

// A Go lister value copy owns a new outer object, retaining immutable descendants.
func ObjectShallowCopy(descriptor,value any) any {
 return rt.IO(func(context.Context)(any,error){
  expected:=objectDescriptor(descriptor).typ
  source:=value.(Value).value
  if source.Kind()==reflect.Pointer {
   if source.IsNil()||source.Type().Elem()!=expected{return nil,fmt.Errorf("hostdeps: object descriptor type mismatch")}
   source=source.Elem()
  }
  if source.Type()!=expected{return nil,fmt.Errorf("hostdeps: object descriptor type mismatch")}
  copied:=reflect.New(expected).Elem()
  copied.Set(source)
  return Value{copied},nil
 })
}

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
func ObjectAsStatus(value any) any {return TypedObjectFrom(ObjectDescriptor("k8s.io/apimachinery/pkg/apis/meta/v1.Status"),rt.SomeOf(value))}

func ObjectGoString(value any) any {
 return rt.IO(func(ctx context.Context)(any,error){
  object:=To(reflect.TypeFor[runtime.Object](),value,ctx).Interface()
  return fmt.Sprintf("%#v",object),nil
 })
}

func ObjectTypeName(value any) any {
 return rt.IO(func(ctx context.Context)(any,error){
  object:=To(reflect.TypeFor[runtime.Object](),value,ctx).Interface()
  return fmt.Sprintf("%T",object),nil
 })
}
