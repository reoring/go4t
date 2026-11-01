// Copyright 2026 The Kubernetes Authors. Licensed under Apache-2.0.
// Only external schema codecs and value conversions; retrieval policy is native.
package host

import (
 "context"
 "encoding/json"
 "net/url"
 "reflect"
 "go4t/rt"
 deps "github.com/reoring/gofx/ports/client-go/host/deps"
 v2 "github.com/google/gnostic-models/openapiv2"
 "google.golang.org/protobuf/proto"
 "k8s.io/apimachinery/pkg/util/managedfields"
 "k8s.io/kube-openapi/pkg/spec3"
 "k8s.io/kube-openapi/pkg/validation/spec"
)
func list(values []any) any { out:=rt.Shapes.Nil(); for i:=len(values)-1;i>=0;i-- {out=rt.Shapes.Cons(values[i],out)};return out }
func errValue(err error) any {return deps.ErrorOptionFromGo(err)}
func optional(value any) any { return deps.From(reflect.ValueOf(value)) }
func Discovery(data any) any {return rt.IO(func(context.Context)(any,error){var value struct{Paths map[string]struct{URL string `json:"serverRelativeURL"`} `json:"paths"`};err:=json.Unmarshal([]byte(data.(string)),&value);entries:=[]any{};for key,entry:=range value.Paths {entries=append(entries,rt.Tuple2Of(key,entry.URL))};return rt.Tuple2Of(list(entries),errValue(err)),nil})}
func URLParts(value any) any {return rt.IO(func(context.Context)(any,error){parsed,err:=url.Parse(value.(string));path:="";entries:=[]any{};if err==nil {path=parsed.Path;for key,values:=range parsed.Query(){for _,value:=range values{entries=append(entries,rt.Tuple2Of(key,value))}}};return rt.Tuple2Of(rt.Tuple2Of(path,list(entries)),errValue(err)),nil})}
func DecodeV2(data any) any {return rt.IO(func(context.Context)(any,error){value:=new(v2.Document);err:=proto.Unmarshal([]byte(data.(string)),value);return rt.Tuple2Of(optional(value),errValue(err)),nil})}
func DecodeV3(data any) any {return rt.IO(func(context.Context)(any,error){value:=new(spec3.OpenAPI);err:=json.Unmarshal([]byte(data.(string)),value);return rt.Tuple2Of(optional(value),errValue(err)),nil})}
func Components(value any) any {return rt.IO(func(ctx context.Context)(any,error){document:=deps.To(reflect.TypeFor[*spec3.OpenAPI](),rt.SomeOf(value),ctx).Interface().(*spec3.OpenAPI);entries:=[]any{};if document.Components!=nil{for key,schema:=range document.Components.Schemas{entries=append(entries,rt.Tuple2Of(key,optional(schema)))}};return list(entries),nil})}
func Converter(entries,preserve any) any {return rt.IO(func(ctx context.Context)(any,error){schemas:=map[string]*spec.Schema{};for{entry,next,ok:=rt.Shapes.Uncons(entries);if !ok{break};key,value:=rt.Shapes.Untuple2(entry);schemas[key.(string)]=deps.To(reflect.TypeFor[*spec.Schema](),value,ctx).Interface().(*spec.Schema);entries=next};value,err:=managedfields.NewTypeConverter(schemas,preserve.(bool));return rt.Tuple2Of(deps.From(reflect.ValueOf(&value).Elem()),errValue(err)),nil})}
func DecodeMap(data any) any {return rt.IO(func(context.Context)(any,error){var value map[string]interface{};err:=json.Unmarshal([]byte(data.(string)),&value);return rt.Tuple2Of(deps.From(reflect.ValueOf(value)),errValue(err)),nil})}
