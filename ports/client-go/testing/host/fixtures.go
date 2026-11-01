// Copyright 2026 The Kubernetes Authors. Licensed under Apache-2.0.
package host

import (
 "context"
 "reflect"
 "io"
 "net/http"
 "strings"
 "go4t/rt"
 deps "github.com/reoring/gofx/ports/client-go/host/deps"
 core "k8s.io/api/core/v1"
 "k8s.io/apimachinery/pkg/runtime"
 "k8s.io/apimachinery/pkg/runtime/serializer"
 "k8s.io/apimachinery/pkg/util/managedfields"
)

// FixtureDependencies constructs only external dependency values; every tracker
// operation exercised by the tests still executes the native implementation.
func FixtureDependencies() rt.IO {return func(context.Context)(any,error){s:=runtime.NewScheme();if e:=core.AddToScheme(s);e!=nil{return nil,e};var d runtime.Decoder=serializer.NewCodecFactory(s).UniversalDeserializer();boxed:=deps.From(reflect.ValueOf(&d).Elem());decoder,_:=rt.OptionValue(boxed);return rt.Tuple2Of(box(s),decoder),nil}}
func DeducedTypeConverter() rt.IO {return func(context.Context)(any,error){t:=managedfields.NewDeducedTypeConverter();boxed:=deps.From(reflect.ValueOf(&t).Elem());v,_:=rt.OptionValue(boxed);return v,nil}}
func RegisterKind(s,k,v any) rt.IO {return func(c context.Context)(any,error){scheme(s,c).AddKnownTypeWithName(gvk(k,c),obj(v,c));return rt.Unit{},nil}}
func FixtureResponse(status int64,body string) rt.IO {return func(context.Context)(any,error){return box(&http.Response{StatusCode:int(status),Header:http.Header{"Content-Type":[]string{"application/json"}},Body:io.NopCloser(strings.NewReader(body))}),nil}}
func RequestDescription(v any) rt.IO {return func(c context.Context)(any,error){r:=deps.To(reflect.TypeFor[*http.Request](),rt.SomeOf(v),c).Interface().(*http.Request);var body []byte;var e error;if r.Body!=nil{body,e=io.ReadAll(r.Body)};return rt.Tuple2Of(r.Method,rt.Tuple2Of(r.URL.String(),string(body))),e}}
