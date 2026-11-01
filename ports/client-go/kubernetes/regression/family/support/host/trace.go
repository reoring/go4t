// Copyright The Kubernetes Authors. Licensed under Apache-2.0.
// Verification-only observation of actual HTTP requests and canonical API values.
package host

import (
 "context"
 "encoding/json"
 "fmt"
 "io"
 "net/http"
 "os"
 "reflect"
 "strings"

 "go4t/rt"
 deps "github.com/reoring/gofx/ports/client-go/host/deps"
 apimeta "k8s.io/apimachinery/pkg/api/meta"
 "k8s.io/apimachinery/pkg/runtime"
)

func ObserveRequest(value any) any {
 return rt.IO(func(ctx context.Context) (any,error) {
  request:=deps.To(reflect.TypeFor[*http.Request](),rt.SomeOf(value),ctx).Interface().(*http.Request)
  var body any
  if request.Body!=nil {
   data,err:=io.ReadAll(request.Body);if err!=nil{return nil,err}
   if len(data)>0 {if err:=json.Unmarshal(data,&body);err!=nil{return nil,fmt.Errorf("family request body is not JSON: %w",err)}}
  }
  encoded,err:=json.Marshal(map[string]any{"method":request.Method,"path":request.URL.Path,"query":request.URL.Query(),"body":body,"accept":request.Header.Get("Accept"),"content_type":request.Header.Get("Content-Type")})
  return string(encoded),err
 })
}

func DescribeResult(value any) any {
 return rt.IO(func(ctx context.Context) (any,error) {
  object:=deps.To(reflect.TypeFor[runtime.Object](),rt.SomeOf(value),ctx).Interface().(runtime.Object)
  typ:=reflect.TypeOf(object);for typ.Kind()==reflect.Pointer {typ=typ.Elem()}
  version:=object.GetObjectKind().GroupVersionKind()
  name,namespace,rv:="","",""
  if metadata,err:=apimeta.Accessor(object);err==nil {name=metadata.GetName();namespace=metadata.GetNamespace();rv=metadata.GetResourceVersion()} else if list,err:=apimeta.ListAccessor(object);err==nil {rv=list.GetResourceVersion()}
  encoded,err:=json.Marshal(map[string]any{"go_type":typ.PkgPath()+"."+typ.Name(),"api_version":version.GroupVersion().String(),"kind":version.Kind,"name":name,"namespace":namespace,"resource_version":rv})
  return string(encoded),err
 })
}

func ResultMatches(encoded,goType,kind string) bool {
 var value map[string]string
 if json.Unmarshal([]byte(encoded),&value)!=nil{return false}
 name,namespace:="response-name","response-namespace"
 if strings.HasSuffix(kind,"List") {name,namespace="",""}
 return value["go_type"]==goType && value["name"]==name && value["namespace"]==namespace && value["resource_version"]=="response-rv"
}

func Emit(resource,operation,requests,result,castOK,errorValue any) any {
 return rt.IO(func(ctx context.Context) (any,error) {
  observed:=make([]json.RawMessage,0,1)
  remaining:=requests
  for {head,tail,present:=rt.Shapes.Uncons(remaining);if !present {break};observed=append(observed,json.RawMessage(head.(string)));remaining=tail}
  var object any
  if value,present:=rt.OptionValue(result);present {object=json.RawMessage(value.(string))}
  var message any
  if value,present:=rt.OptionValue(errorValue);present {message=value.(string)}
  row:=map[string]any{"schema_version":1,"resource":resource.(string),"operation":operation.(string),"requests":observed,"result":object,"cast_ok":castOK.(bool),"error":message}
  if err:=json.NewEncoder(os.Stdout).Encode(row);err!=nil{return nil,err}
  return rt.Unit{},nil
 })
}
