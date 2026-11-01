// Copyright 2018 The Kubernetes Authors. Licensed under Apache-2.0.
package host
import (
 "context"
 "reflect"
 "gofx/rt"
 deps "github.com/reoring/gofx/ports/client-go/host/deps"
 "k8s.io/apimachinery/pkg/api/meta"
 "k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
 "k8s.io/apimachinery/pkg/runtime"
 "k8s.io/apimachinery/pkg/runtime/schema"
 "k8s.io/apimachinery/pkg/runtime/serializer"
)
func ToUnstructured(value any) any {return rt.IO(func(ctx context.Context)(any,error){object:=from[runtime.Object](ctx,value);fields,err:=runtime.DefaultUnstructuredConverter.ToUnstructured(object);var result *unstructured.Unstructured;if err==nil{result=&unstructured.Unstructured{Object:fields}};return rt.Tuple2Of(deps.From(reflect.ValueOf(result)),deps.ErrorOptionFromGo(err)),nil})}
func SchemeToUnstructured(scheme,value any) any {return rt.IO(func(ctx context.Context)(any,error){s:=from[*runtime.Scheme](ctx,scheme);object:=from[runtime.Object](ctx,value);result:=&unstructured.Unstructured{};err:=s.Convert(object,result,nil);return rt.Tuple2Of(deps.From(reflect.ValueOf(result)),deps.ErrorOptionFromGo(err)),nil})}
func GuessKindResource(value any) any {return rt.IO(func(ctx context.Context)(any,error){plural,singular:=meta.UnsafeGuessKindToResource(from[schema.GroupVersionKind](ctx,value));return rt.Tuple2Of(box(plural),box(singular)),nil})}
func SchemeDecoder(scheme any) any {return rt.IO(func(ctx context.Context)(any,error){return box(serializer.NewCodecFactory(from[*runtime.Scheme](ctx,scheme)).UniversalDecoder()),nil})}
