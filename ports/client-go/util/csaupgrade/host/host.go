// Copyright 2022 The Kubernetes Authors. Licensed under Apache-2.0.
// External fieldpath, metadata conversion, and JSON codecs; upgrade policy is native.
package host

import (
 "context"
 "encoding/json"
 "reflect"
 "go4t/rt"
 "github.com/reoring/gofx/ports/client-go/host/deps"
 "k8s.io/apimachinery/pkg/api/meta"
 metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
 kruntime "k8s.io/apimachinery/pkg/runtime"
 "sigs.k8s.io/structured-merge-diff/v6/fieldpath"
)
func to[T any](ctx context.Context,v any) T {return hostdeps.To(reflect.TypeFor[T](),v,ctx).Interface().(T)}
func from[T any](v T) any {return hostdeps.From(reflect.ValueOf(&v).Elem())}
func pair(value any,err error) any {return rt.Tuple2Of(value,from[error](err))}
func Decode(entry any) any {return rt.IO(func(ctx context.Context)(any,error){
 f:=to[metav1.ManagedFieldsEntry](ctx,entry);s:=fieldpath.Set{};err:=s.FromJSON(f.FieldsV1.GetRawReader());if err!=nil{return pair(rt.NoneOf(),err),nil};return pair(rt.SomeOf(from(s)),nil),nil
})}
func Encode(entry,fields any) any {return rt.IO(func(ctx context.Context)(any,error){
 f:=to[metav1.ManagedFieldsEntry](ctx,entry);s:=to[fieldpath.Set](ctx,fields);raw,err:=s.ToJSON();if f.FieldsV1!=nil{f.FieldsV1=f.FieldsV1.DeepCopy()};f.FieldsV1.SetRawBytes(raw);return pair(from(f),err),nil
})}
func Union(a,b any) any {return rt.IO(func(ctx context.Context)(any,error){x,y:=to[fieldpath.Set](ctx,a),to[fieldpath.Set](ctx,b);return from(*x.Union(&y)),nil})}
func Contains(a,b any) any {return rt.IO(func(ctx context.Context)(any,error){x,y:=to[fieldpath.Set](ctx,a),to[fieldpath.Set](ctx,b);return y.Difference(&x).Empty(),nil})}
func Read(object any) any {return rt.IO(func(ctx context.Context)(any,error){obj:=to[kruntime.Object](ctx,rt.SomeOf(object));a,err:=meta.Accessor(obj);if err!=nil{return rt.Tuple2Of(rt.NoneOf(),pair("",err)),nil};return rt.Tuple2Of(from(a.GetManagedFields()),pair(a.GetResourceVersion(),nil)),nil})}
func Write(object,entries any) any {return rt.IO(func(ctx context.Context)(any,error){obj:=to[kruntime.Object](ctx,rt.SomeOf(object)).DeepCopyObject();a,err:=meta.Accessor(obj);if err!=nil{return pair(rt.NoneOf(),err),nil};a.SetManagedFields(to[[]metav1.ManagedFieldsEntry](ctx,entries));return pair(from[kruntime.Object](obj),nil),nil})}
func Equal(a,b any) any {ctx:=context.Background();return reflect.DeepEqual(to[[]metav1.ManagedFieldsEntry](ctx,a),to[[]metav1.ManagedFieldsEntry](ctx,b))}
func Marshal(entries any) any {return rt.IO(func(ctx context.Context)(any,error){data,err:=json.Marshal(to[[]metav1.ManagedFieldsEntry](ctx,entries));return pair(string(data),err),nil})}
func Quote(value any) any {data,err:=json.Marshal(value.(string));if err!=nil{panic(err)};return string(data)}
