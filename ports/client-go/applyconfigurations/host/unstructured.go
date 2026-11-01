// Copyright 2021 The Kubernetes Authors. Licensed under Apache-2.0.
package host

import (
 "context"
 "reflect"
 "go4t/rt"
 "github.com/reoring/gofx/ports/client-go/host/deps"
 openapiv2 "github.com/google/gnostic-models/openapiv2"
 "k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
 "k8s.io/apimachinery/pkg/util/managedfields"
 "k8s.io/kube-openapi/pkg/util/proto"
)
func BuildGVKParser(doc any) any {return rt.IO(func(ctx context.Context)(any,error){
 value:=hostdeps.To(reflect.TypeFor[openapiv2.Document](),doc,ctx).Interface().(openapiv2.Document)
 models,err:=proto.NewOpenAPIData(&value);if err!=nil{return result(rt.NoneOf(),err),nil}
 parser,err:=managedfields.NewGVKParser(models,false);if err!=nil{return result(rt.NoneOf(),err),nil}
 return result(rt.SomeOf(hostdeps.From(reflect.ValueOf(*parser))),nil),nil
})}
func ExtractUnstructured(parser,object,manager,subresource any) any {return rt.IO(func(ctx context.Context)(any,error){
 p:=hostdeps.To(reflect.TypeFor[managedfields.GvkParser](),parser,ctx).Interface().(managedfields.GvkParser)
 value:=hostdeps.To(reflect.TypeFor[unstructured.Unstructured](),object,ctx).Interface().(unstructured.Unstructured)
 objectType:=p.Type(value.GroupVersionKind());destination:=&unstructured.Unstructured{}
 err:=managedfields.ExtractInto(&value,*objectType,manager.(string),destination,subresource.(string))
 if err!=nil{return result(rt.NoneOf(),err),nil};return result(rt.SomeOf(hostdeps.From(reflect.ValueOf(*destination))),nil),nil
})}
func UnstructuredIdentity(object any) any {
 value:=hostdeps.To(reflect.TypeFor[unstructured.Unstructured](),object,context.Background()).Interface().(unstructured.Unstructured)
 return rt.Tuple2Of(value.GetName(),rt.Tuple2Of(value.GetNamespace(),rt.Tuple2Of(value.GetKind(),value.GetAPIVersion())))
}
func WithUnstructuredIdentity(object,name,namespace,kind,apiVersion any) any {
 value:=hostdeps.To(reflect.TypeFor[unstructured.Unstructured](),object,context.Background()).Interface().(unstructured.Unstructured)
 result:=value.DeepCopy();result.SetName(name.(string));result.SetNamespace(namespace.(string));result.SetKind(kind.(string));result.SetAPIVersion(apiVersion.(string));return hostdeps.From(reflect.ValueOf(*result))
}
