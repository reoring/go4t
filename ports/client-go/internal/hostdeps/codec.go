package hostdeps

import (
 "context"
 "errors"
 "fmt"
 "reflect"

 "gofx/rt"
 apierrors "k8s.io/apimachinery/pkg/api/errors"
 "k8s.io/apimachinery/pkg/runtime"
 "k8s.io/apimachinery/pkg/runtime/schema"
 "k8s.io/apimachinery/pkg/runtime/serializer"
 "k8s.io/apimachinery/pkg/util/wait"
)

// Scheme registrations are supplied by the generated exact-version dependency inventory.
var schemeRegistrations []func(*runtime.Scheme) error
func RegisterScheme(f func(*runtime.Scheme) error) {schemeRegistrations=append(schemeRegistrations,f)}
func NewJSONDecoder() any {return rt.IO(func(context.Context)(any,error){s:=runtime.NewScheme();for _,f:=range schemeRegistrations{if err:=f(s);err!=nil{return nil,err}};d:=serializer.NewCodecFactory(s,serializer.EnableStrict).UniversalDeserializer();return Value{reflect.ValueOf(d)},nil})}
func Decode(decoder,data,defaults,into any) any {return rt.IO(func(ctx context.Context)(any,error){d:=To(reflect.TypeFor[runtime.Decoder](),rt.SomeOf(decoder),ctx).Interface().(runtime.Decoder);var target runtime.Object;if x,ok:=option(into);ok {target=To(reflect.TypeFor[runtime.Object](),rt.SomeOf(x),ctx).Interface().(runtime.Object);target=target.DeepCopyObject()};gvk:=To(reflect.TypeFor[*schema.GroupVersionKind](),defaults,ctx).Interface().(*schema.GroupVersionKind);obj,actual,err:=d.Decode([]byte(data.(string)),gvk,target);return rt.Tuple2Of(objectOption(obj),rt.Tuple2Of(From(reflect.ValueOf(actual)),errorOption(err))),nil})}
func objectOption(v runtime.Object) any {if v==nil{return rt.NoneOf()};return rt.SomeOf(Value{reflect.ValueOf(v)})}
func errorOption(err error) any {if err==nil{return rt.NoneOf()};return rt.SomeOf(Value{reflect.ValueOf(err)})}
func externalError(v any) error {return v.(Value).value.Interface().(error)}
func ErrorMessage(v any) any {return externalError(v).Error()}
func ErrorIs(v,target any) any {return errors.Is(externalError(v),externalError(target))}
func ErrorIsNotFound(v any) any {return apierrors.IsNotFound(externalError(v))}
func ErrorIsConflict(v any) any {return apierrors.IsConflict(externalError(v))}
func ErrorIsStrictDecoding(v any) any {return runtime.IsStrictDecodingError(externalError(v))}
func ErrorStatus(v any) any {var status apierrors.APIStatus;if !errors.As(externalError(v),&status){return rt.NoneOf()};return rt.SomeOf(Value{reflect.ValueOf(status.Status())})}
func NewNotFound(resource,name any) any {r:=resource.(Value).value.Interface().(schema.GroupResource);return Value{reflect.ValueOf(apierrors.NewNotFound(r,name.(string)))}}

func WaitTimeoutError() any {return Value{reflect.ValueOf(wait.ErrWaitTimeout)}}
func ErrorInterrupted(cause any) any {var err error;if v,ok:=option(cause);ok{err=externalError(v)};return Value{reflect.ValueOf(wait.ErrorInterrupted(err))}}
func ErrorIsInterrupted(v any) any {return wait.Interrupted(externalError(v))}
func ErrorConflict(group,resource,name,cause any) any {var err error;if v,ok:=option(cause);ok{err=externalError(v)};return Value{reflect.ValueOf(apierrors.NewConflict(schema.GroupResource{Group:group.(string),Resource:resource.(string)},name.(string),err))}}
func ErrorCancelled() any {return Value{reflect.ValueOf(context.Canceled)}}
func RaiseError(value any) any {return rt.IO(func(context.Context)(any,error){return nil,externalError(value)})}
func ErrorNew(message any) any {return Value{reflect.ValueOf(errors.New(message.(string)))}}
func ErrorWrap(message,cause any) any {return Value{reflect.ValueOf(fmt.Errorf("%s: %w",message.(string),externalError(cause)))}}
