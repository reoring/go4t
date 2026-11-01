package hostdeps

import (
 "context"
 "errors"
 "fmt"
 "reflect"

 "go4t/rt"
 apierrors "k8s.io/apimachinery/pkg/api/errors"
 "k8s.io/apimachinery/pkg/runtime"
 "k8s.io/apimachinery/pkg/runtime/schema"
 "k8s.io/apimachinery/pkg/runtime/serializer"
 "k8s.io/apimachinery/pkg/util/wait"
 cbor "k8s.io/apimachinery/pkg/runtime/serializer/cbor/direct"
 jsonutil "k8s.io/apimachinery/pkg/util/json"
)

// Scheme registrations are supplied by the generated exact-version dependency inventory.
var schemeRegistrations []func(*runtime.Scheme) error
func RegisterScheme(f func(*runtime.Scheme) error) {schemeRegistrations=append(schemeRegistrations,f)}
func NewJSONDecoder() any {return rt.IO(func(context.Context)(any,error){s:=runtime.NewScheme();for _,f:=range schemeRegistrations{if err:=f(s);err!=nil{return nil,err}};d:=serializer.NewCodecFactory(s,serializer.EnableStrict).UniversalDeserializer();return Value{reflect.ValueOf(d)},nil})}
func Decode(decoder,data,defaults,into any) any {return rt.IO(func(ctx context.Context)(any,error){d:=To(reflect.TypeFor[runtime.Decoder](),rt.SomeOf(decoder),ctx).Interface().(runtime.Decoder);var target runtime.Object;if x,ok:=option(into);ok {target=To(reflect.TypeFor[runtime.Object](),rt.SomeOf(x),ctx).Interface().(runtime.Object);target=target.DeepCopyObject()};gvk:=To(reflect.TypeFor[*schema.GroupVersionKind](),defaults,ctx).Interface().(*schema.GroupVersionKind);obj,actual,err:=d.Decode([]byte(data.(string)),gvk,target);return rt.Tuple2Of(objectOption(obj),rt.Tuple2Of(From(reflect.ValueOf(actual)),errorOption(err))),nil})}
func objectOption(v runtime.Object) any {if v==nil{return rt.NoneOf()};return rt.SomeOf(Value{reflect.ValueOf(v)})}
func errorOption(err error) any {if err==nil{return rt.NoneOf()};return rt.SomeOf(ErrorFromGo(err))}
func externalError(v any) error {return v.(Value).value.Interface().(error)}

// ErrorFromGo boxes a nonnil Go error as a bare canonical Error. A typed-nil
// error retains its dynamic type and identity; only a nil interface is rejected.
func ErrorFromGo(err error) any {
 if err==nil{panic("hostdeps: cannot box nil error")}
 return Value{reflect.ValueOf(err)}
}

// ErrorOptionFromGo adds exactly one Option layer, absent only for nil interface.
func ErrorOptionFromGo(err error) any {return errorOption(err)}

// ErrorToGo accepts only a bare canonical Error, never Option[Error] or text.
func ErrorToGo(value any) error {return externalError(value)}
func ErrorMessage(v any) any {return externalError(v).Error()}
func ErrorIs(v,target any) any {return errors.Is(externalError(v),externalError(target))}
func ErrorIsNotFound(v any) any {return apierrors.IsNotFound(externalError(v))}
func ErrorIsAlreadyExists(v any) any {return apierrors.IsAlreadyExists(externalError(v))}
func ErrorIsConflict(v any) any {return apierrors.IsConflict(externalError(v))}
func ErrorIsStrictDecoding(v any) any {return runtime.IsStrictDecodingError(externalError(v))}
func ErrorStatus(v any) any {var status apierrors.APIStatus;if !errors.As(externalError(v),&status){return rt.NoneOf()};return rt.SomeOf(storedValue(reflect.ValueOf(status.Status())))}
func NewNotFound(resource,name any) any {r:=resource.(Value).value.Interface().(schema.GroupResource);return Value{reflect.ValueOf(apierrors.NewNotFound(r,name.(string)))}}

func WaitTimeoutError() any {return Value{reflect.ValueOf(wait.ErrWaitTimeout)}}
func ErrorInterrupted(cause any) any {var err error;if v,ok:=option(cause);ok{err=externalError(v)};return Value{reflect.ValueOf(wait.ErrorInterrupted(err))}}
func ErrorIsInterrupted(v any) any {return wait.Interrupted(externalError(v))}
func ErrorConflict(group,resource,name,cause any) any {var err error;if v,ok:=option(cause);ok{err=externalError(v)};return Value{reflect.ValueOf(apierrors.NewConflict(schema.GroupResource{Group:group.(string),Resource:resource.(string)},name.(string),err))}}
func ErrorNotFound(group,resource,name any) any {return Value{reflect.ValueOf(apierrors.NewNotFound(schema.GroupResource{Group:group.(string),Resource:resource.(string)},name.(string)))}}
func ErrorCancelled() any {return Value{reflect.ValueOf(context.Canceled)}}
func RaiseError(value any) any {return rt.IO(func(context.Context)(any,error){return nil,externalError(value)})}
func ErrorNew(message any) any {return Value{reflect.ValueOf(errors.New(message.(string)))}}

// taggedError is an immutable, comparable sentinel value. Native callers own
// the domain, key and message; no registry or message-only matching is involved.
type taggedError struct {
 domain string
 key string
 message string
}
func (e taggedError) Error() string {return e.message}
func ErrorTagged(domain,key,message any) any {return ErrorFromGo(taggedError{domain.(string),key.(string),message.(string)})}
func ErrorWrap(message,cause any) any {return Value{reflect.ValueOf(fmt.Errorf("%s: %w",message.(string),externalError(cause)))}}

func ErrorIsNotRegistered(value any) any {return runtime.IsNotRegisteredError(externalError(value))}
func EncodeJSON(value any) any {
 return rt.IO(func(ctx context.Context)(any,error){
  encoded,err:=jsonutil.Marshal(objectValue(value,ctx))
  return rt.Tuple2Of(From(reflect.ValueOf(encoded)),errorOption(err)),nil
 })
}
func EncodeCBOR(value any) any {
 return rt.IO(func(ctx context.Context)(any,error){
  encoded,err:=cbor.Marshal(objectValue(value,ctx))
  return rt.Tuple2Of(From(reflect.ValueOf(encoded)),errorOption(err)),nil
 })
}
func DecodeJSON(descriptor,data any) any {
 return rt.IO(func(context.Context)(any,error){
  target:=reflect.New(objectDescriptor(descriptor).typ)
  err:=jsonutil.Unmarshal([]byte(data.(string)),target.Interface())
  return rt.Tuple2Of(rt.SomeOf(Value{target.Elem()}),errorOption(err)),nil
 })
}
