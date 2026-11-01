package hostdeps

import (
 "context"
 "reflect"
 "sync"

 "gofx/rt"
 apierrors "k8s.io/apimachinery/pkg/api/errors"
 apimeta "k8s.io/apimachinery/pkg/api/meta"
 metainternal "k8s.io/apimachinery/pkg/apis/meta/internalversion"
 metavalidation "k8s.io/apimachinery/pkg/apis/meta/internalversion/validation"
 metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
 "k8s.io/apimachinery/pkg/runtime"
 netutil "k8s.io/apimachinery/pkg/util/net"
)

func objectValue(value any,ctx context.Context) runtime.Object {
 return To(reflect.TypeFor[runtime.Object](),rt.SomeOf(value),ctx).Interface().(runtime.Object)
}
func ObjectMetadata(value any) any {
 return rt.IO(func(ctx context.Context)(any,error){
  accessor,err:=apimeta.Accessor(objectValue(value,ctx))
  if err!=nil{return rt.Tuple2Of(rt.NoneOf(),errorOption(err)),nil}
  metadata:=apimeta.AsPartialObjectMetadata(accessor).ObjectMeta
  return rt.Tuple2Of(rt.SomeOf(Value{reflect.ValueOf(metadata)}),rt.NoneOf()),nil
 })
}
func ObjectResourceVersionValue(value any) any {
 return rt.IO(func(ctx context.Context)(any,error){
  accessor,ok:=objectValue(value,ctx).(interface{GetResourceVersion() string})
  if !ok{return rt.NoneOf(),nil}
  return rt.SomeOf(accessor.GetResourceVersion()),nil
 })
}

func ObjectSetResourceVersion(value,resourceVersion any) any {
 return rt.IO(func(ctx context.Context)(any,error){
  updated:=objectValue(value,ctx).DeepCopyObject()
  accessor,err:=apimeta.Accessor(updated)
  if err!=nil{return rt.Tuple2Of(rt.NoneOf(),errorOption(err)),nil}
  accessor.SetResourceVersion(resourceVersion.(string))
  return rt.Tuple2Of(objectOption(updated),rt.NoneOf()),nil
 })
}
func ErrorFromObject(value any) any {
 return rt.IO(func(ctx context.Context)(any,error){return Value{reflect.ValueOf(apierrors.FromObject(objectValue(value,ctx)))},nil})
}
func ErrorIsResourceExpired(value any) any {return apierrors.IsResourceExpired(externalError(value))}
func ErrorIsTimeout(value any) any {return apierrors.IsTimeout(externalError(value))}
func ErrorIsTooManyRequests(value any) any {return apierrors.IsTooManyRequests(externalError(value))}
func ErrorIsInternal(value any) any {return apierrors.IsInternalError(externalError(value))}
func ErrorIsConnectionRefused(value any) any {return netutil.IsConnectionRefused(externalError(value))}
func ErrorHasStatusCause(value,cause any) any {return apierrors.HasStatusCause(externalError(value),cause.(Value).value.Interface().(metav1.CauseType))}

func ObjectListInfo(value any) any {
 return rt.IO(func(ctx context.Context)(any,error){
  accessor,err:=apimeta.ListAccessor(objectValue(value,ctx))
  if err!=nil{return rt.Tuple2Of(rt.Tuple2Of("",rt.Tuple2Of("","")),errorOption(err)),nil}
  info:=rt.Tuple2Of(accessor.GetResourceVersion(),rt.Tuple2Of(accessor.GetSelfLink(),accessor.GetContinue()))
  return rt.Tuple2Of(info,rt.NoneOf()),nil
 })
}
func ObjectListItems(value,allocate any) any {
 return rt.IO(func(ctx context.Context)(any,error){
  var items []runtime.Object
  var err error
  if allocate.(bool){items,err=apimeta.ExtractListWithAlloc(objectValue(value,ctx))}else{items,err=apimeta.ExtractList(objectValue(value,ctx))}
  return rt.Tuple2Of(From(reflect.ValueOf(items)),errorOption(err)),nil
 })
}
func NewObjectList(resourceVersion,selfLink,items any) any {
 return rt.IO(func(ctx context.Context)(any,error){
  values:=To(reflect.TypeFor[[]runtime.Object](),items,ctx).Interface().([]runtime.Object)
  result:=&metainternal.List{ListMeta:metav1.ListMeta{ResourceVersion:resourceVersion.(string),SelfLink:selfLink.(string)},Items:values}
  return Value{reflect.ValueOf(result)},nil
 })
}
var listValidation struct {once sync.Once; scheme *runtime.Scheme; err error}
func ValidateListOptions(value,watchListFeatureEnabled any) any {
 return rt.IO(func(context.Context)(any,error){
  listValidation.once.Do(func(){listValidation.scheme=runtime.NewScheme();listValidation.err=metainternal.AddToScheme(listValidation.scheme)})
  if listValidation.err!=nil{return rt.Tuple2Of(false,errorOption(listValidation.err)),nil}
  input:=checked(value,"k8s.io/apimachinery/pkg/apis/meta/v1.ListOptions").Interface().(metav1.ListOptions)
  var internal metainternal.ListOptions
  if err:=listValidation.scheme.Convert(&input,&internal,nil);err!=nil{return rt.Tuple2Of(false,errorOption(err)),nil}
  valid:=len(metavalidation.ValidateListOptions(&internal,watchListFeatureEnabled.(bool)))==0
  return rt.Tuple2Of(valid,rt.NoneOf()),nil
 })
}

func ObjectGroupVersionKind(value any) any {return rt.IO(func(ctx context.Context)(any,error){return Value{reflect.ValueOf(objectValue(value,ctx).GetObjectKind().GroupVersionKind())},nil})}
func StatusToObject(value any) any {status:=checked(value,"k8s.io/apimachinery/pkg/apis/meta/v1.Status").Interface().(metav1.Status);return Value{reflect.ValueOf(&status)}}
func ErrorInternal(cause any) any {return Value{reflect.ValueOf(apierrors.NewInternalError(externalError(cause)))}}
func ErrorIsForbidden(value any) any {return apierrors.IsForbidden(externalError(value))}
func ErrorIsUnauthorized(value any) any {return apierrors.IsUnauthorized(externalError(value))}

func ListOptionsToObject(value any) any {
 options:=checked(value,"k8s.io/apimachinery/pkg/apis/meta/v1.ListOptions").Interface().(metav1.ListOptions)
 return Value{reflect.ValueOf(&options)}
}
func StandardParameterCodec() any {
 return rt.IO(func(context.Context)(any,error){return Value{reflect.ValueOf(metav1.ParameterCodec)},nil})
}
