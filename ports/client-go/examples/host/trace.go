// Copyright 2026 The go4t Authors. SPDX-License-Identifier: Apache-2.0
// External value codecs and URL decoding only. No client-go dependency.
package host

import (
 "context"
 "encoding/json"
 "errors"
 "fmt"
 "net/url"
 "reflect"

 "go4t/rt"
 "github.com/reoring/gofx/ports/client-go/host/deps"
 metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
 u "k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
 "k8s.io/apimachinery/pkg/runtime"
)
func effect(f func(context.Context)(any,error))any{return rt.IO(func(ctx context.Context)(any,error){if e:=ctx.Err();e!=nil{return nil,e};return f(ctx)})}
func Quote(value string)string{b,_:=json.Marshal(value);return string(b)}
func QueryJSON(value string)(string,error){q,e:=url.ParseQuery(value);if e!=nil{return "",e};b,e:=json.Marshal(q);return string(b),e}
func ObjectJSON(value any)any{return effect(func(ctx context.Context)(any,error){object:=hostdeps.To(reflect.TypeFor[runtime.Object](),value,ctx).Interface();b,e:=json.Marshal(object);return string(b),e})}
func ErrorJSON(value any)any{return effect(func(ctx context.Context)(any,error){
 errValue,ok:=rt.OptionValue(value);if !ok{return `{"error":null}`,nil}
 err:=hostdeps.To(reflect.TypeFor[error](),rt.SomeOf(errValue),ctx).Interface().(error)
 fields:=map[string]any{"message":err.Error(),"go_type":fmt.Sprintf("%T",err),"context_canceled":errors.Is(err,context.Canceled),"deadline_exceeded":errors.Is(err,context.DeadlineExceeded)}
 var status interface{Status()metav1.Status};if errors.As(err,&status){fields["status"]=status.Status()}
 b,e:=json.Marshal(fields);return string(b),e
})}
func DecodeObject(template,data any)any{return effect(func(ctx context.Context)(any,error){
 object:=hostdeps.To(reflect.TypeFor[runtime.Object](),rt.SomeOf(template),ctx).Interface().(runtime.Object)
 typ:=reflect.TypeOf(object);if typ.Kind()!=reflect.Pointer{return nil,fmt.Errorf("fixture object is not pointer-backed: %v",typ)}
 decoded:=reflect.New(typ.Elem());if err:=json.Unmarshal([]byte(data.(string)),decoded.Interface());err!=nil{return nil,err}
 var objectResult runtime.Object=decoded.Interface().(runtime.Object)
 value:=hostdeps.From(reflect.ValueOf(&objectResult).Elem());payload,_:=rt.OptionValue(value);return payload,nil
})}
func unstructuredValue(ctx context.Context,value any)*u.Unstructured{v:=hostdeps.To(reflect.TypeFor[u.Unstructured](),value,ctx).Interface().(u.Unstructured);return &v}
func SetNestedInt(object,path,value any)any{return effect(func(ctx context.Context)(any,error){
 copy:=unstructuredValue(ctx,object).DeepCopy();fields:=rt.ListToStrings(path)
 if err:=u.SetNestedField(copy.Object,value.(int64),fields...);err!=nil{return nil,err};return hostdeps.From(reflect.ValueOf(*copy)),nil
})}
func SetNestedString(object,path,value any)any{return effect(func(ctx context.Context)(any,error){
 copy:=unstructuredValue(ctx,object).DeepCopy();fields:=rt.ListToStrings(path)
 if err:=u.SetNestedField(copy.Object,value.(string),fields...);err!=nil{return nil,err};return hostdeps.From(reflect.ValueOf(*copy)),nil
})}
func NestedMaps(object,path any)any{return effect(func(ctx context.Context)(any,error){
 values,found,err:=u.NestedSlice(unstructuredValue(ctx,object).Object,rt.ListToStrings(path)...);if err!=nil{return nil,err};if !found{return rt.NoneOf(),nil}
 out:=rt.Shapes.Nil();for i:=len(values)-1;i>=0;i--{value,ok:=values[i].(map[string]any);if !ok{return nil,fmt.Errorf("nested array item %d is not an object",i)};out=rt.Shapes.Cons(hostdeps.From(reflect.ValueOf(u.Unstructured{Object:value})),out)}
 return rt.SomeOf(out),nil
})}
func SetNestedMaps(object,path,values any)any{return effect(func(ctx context.Context)(any,error){
 copy:=unstructuredValue(ctx,object).DeepCopy();var items []any
 for xs:=values;;{v,rest,ok:=rt.Shapes.Uncons(xs);if !ok{break};items=append(items,unstructuredValue(ctx,v).Object);xs=rest}
 if err:=u.SetNestedSlice(copy.Object,items,rt.ListToStrings(path)...);err!=nil{return nil,err};return hostdeps.From(reflect.ValueOf(*copy)),nil
})}
