// Copyright 2026 The Kubernetes Authors. Licensed under Apache-2.0.
// Test-only I/O and comparisons; request construction and decoding remain native.
package host

import (
 "bufio"
 "bytes"
 "context"
 "encoding/json"
 "fmt"
 "net/http"
 "net/http/httputil"
 "os"
 "path/filepath"
 "reflect"
 goruntime "runtime"

 "go4t/rt"
 deps "github.com/reoring/gofx/ports/client-go/host/deps"
 metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
 "k8s.io/apimachinery/pkg/runtime"
)

func fixture(group, name string) ([]byte,error) {
 // Fixed allowlists prohibit accidentally consuming or updating other files.
 allowed := map[string]string{
  "TestGoldenRequest":" create update updatestatus delete deletecollection get list watch patch apply applystatus ",
  "TestGoldenResponse":" create update updatestatus get list watch ",
  "TestGoldenResponse/responses":" nonlist list events ",
 }
 names,ok:=allowed[group];if !ok || !bytes.Contains([]byte(names),[]byte(" "+name+" ")) {return nil,fmt.Errorf("unknown dynamic golden %s/%s",group,name)}
 _,file,_,ok:=goruntime.Caller(0);if !ok{return nil,fmt.Errorf("dynamic golden source location unavailable")}
 return os.ReadFile(filepath.Join(filepath.Dir(file),"..","..","testdata",group,name))
}
func request(ctx context.Context,value any)*http.Request{return deps.To(reflect.TypeFor[*http.Request](),rt.SomeOf(value),ctx).Interface().(*http.Request)}
func object(ctx context.Context,value any)runtime.Object{return deps.To(reflect.TypeFor[runtime.Object](),rt.SomeOf(value),ctx).Interface().(runtime.Object)}
func CheckRequest(name,value any)any{return rt.IO(func(ctx context.Context)(any,error){
 want,err:=fixture("TestGoldenRequest",name.(string));if err!=nil{return nil,err}
 // DumpRequestOut exercises net/http's actual HTTP/1 serialization (including
 // transport-added gzip) without an external endpoint. Never rewrite headers
 // or bodies to match the golden.
 wire,err:=httputil.DumpRequestOut(request(ctx,value),true);if err!=nil{return nil,err}
 // The upstream golden is the server-perceived request, not the outbound
 // writer's header ordering. Parse the actual wire bytes as an HTTP server.
 received,err:=http.ReadRequest(bufio.NewReader(bytes.NewReader(wire)));if err!=nil{return nil,err}
 defer received.Body.Close()
 got,err:=httputil.DumpRequest(received,true);if err!=nil{return nil,err}
 if !bytes.Equal(got,want){
  mismatch:=fmt.Errorf("dynamic request golden %s mismatch\nwant: %q\n got: %q",name,want,got)
  // The native HTTP transport retains this as an operation error. Also emit
  // the exact bytes so request-only tests cannot hide the diagnostic.
  fmt.Fprintln(os.Stderr,mismatch)
  return nil,mismatch
 }
 return true,nil
})}
func Response(name,value any)any{return rt.IO(func(ctx context.Context)(any,error){
 data,err:=fixture("TestGoldenResponse/responses",name.(string));if err!=nil{return nil,err}
 response,err:=http.ReadResponse(bufio.NewReader(bytes.NewReader(data)),request(ctx,value));if err!=nil{return nil,err}
 boxed,_:=rt.OptionValue(deps.From(reflect.ValueOf(response)))
 return boxed,nil
})}
func compare(name string,value any)(any,error){
 want,err:=fixture("TestGoldenResponse",name);if err!=nil{return nil,err}
 got,err:=json.Marshal(value);if err!=nil{return nil,err}
 var a,b any
 if err=json.Unmarshal(want,&a);err!=nil{return nil,err}
 if err=json.Unmarshal(got,&b);err!=nil{return nil,err}
 if !reflect.DeepEqual(a,b){return nil,fmt.Errorf("dynamic response golden %s mismatch\nwant: %s\n got: %s",name,want,got)}
 return true,nil
}
func CheckObject(name,value any)any{return rt.IO(func(ctx context.Context)(any,error){return compare(name.(string),object(ctx,value))})}
func CheckEvents(values any)any{return rt.IO(func(ctx context.Context)(any,error){
 events:=[]any{}
 for {head,tail,ok:=rt.Shapes.Uncons(values);if !ok{break};values=tail
  kind,wrapped:=rt.Shapes.Untuple2(head)
  raw,present:=rt.OptionValue(wrapped);if !present{return nil,fmt.Errorf("dynamic golden event has no object")}
  event,err:=runtime.DefaultUnstructuredConverter.ToUnstructured(&metav1.WatchEvent{Type:kind.(string),Object:runtime.RawExtension{Object:object(ctx,raw)}});if err!=nil{return nil,err}
  events=append(events,event)
 }
 return compare("watch",events)
})}
