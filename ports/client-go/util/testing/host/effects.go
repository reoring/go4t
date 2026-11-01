// Copyright 2026 The go4t Authors.
// SPDX-License-Identifier: Apache-2.0
// Generic loopback HTTP and filesystem effects. Fixture policies live in .gt.
package host

import (
 "context"
 "encoding/json"
 "fmt"
 "io"
 "io/fs"
 "net/http"
 "net/http/httptest"
 "net/url"
 "os"
 "path/filepath"
 "strings"
 "reflect"
 "sync"

 "go4t/rt"
 "github.com/reoring/gofx/ports/client-go/host/deps"
 "k8s.io/kube-openapi/pkg/handler3"
 "k8s.io/kube-openapi/pkg/spec3"
)

func effect(run func(context.Context) (any,error)) any { return rt.IO(func(ctx context.Context)(any,error){ if err:=ctx.Err();err!=nil{return nil,err};return run(ctx) }) }
func call(ctx context.Context, fn any, args ...any)(any,error){ in:=make([]reflect.Value,len(args));for i,arg:=range args{in[i]=reflect.ValueOf(arg)};return reflect.ValueOf(fn).Call(in)[0].Interface().(rt.IO)(ctx) }
func list(values []any)any { out:=rt.Shapes.Nil();for i:=len(values)-1;i>=0;i--{out=rt.Shapes.Cons(values[i],out)};return out }
func headers(values http.Header)any { pairs:=make([]any,0,len(values));for key,value:=range values{pairs=append(pairs,rt.Tuple2Of(key,rt.ListFromStrings(value)))};return list(pairs) }
func each(xs any,fn func(any)){ for { h,t,ok:=rt.Shapes.Uncons(xs);if !ok{return};fn(h);xs=t } }
type server struct { http *httptest.Server; mu sync.Mutex; err error }
func(s *server)fail(err error){s.mu.Lock();defer s.mu.Unlock();if s.err==nil{s.err=err}}
func StartHTTP(handler any)any { return effect(func(parent context.Context)(any,error){
 s:=&server{}
 s.http=httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter,r *http.Request){
  ctx,cancel:=context.WithCancel(r.Context());stop:=context.AfterFunc(parent,cancel);defer stop();defer cancel()
  body,err:=io.ReadAll(r.Body);if err!=nil{s.fail(err);http.Error(w,err.Error(),500);return}
  result,err:=call(ctx,handler,r.Method,r.URL.Path,r.URL.RawQuery,headers(r.Header),string(body))
  if err!=nil{s.fail(err);http.Error(w,err.Error(),500);return}
  status,rest:=rt.Shapes.Untuple2(result);hs,payload:=rt.Shapes.Untuple2(rest)
  each(hs,func(pair any){key,vs:=rt.Shapes.Untuple2(pair);each(vs,func(v any){w.Header().Add(key.(string),v.(string))})})
  w.WriteHeader(int(status.(int64)));if _,err=w.Write([]byte(payload.(string)));err!=nil{s.fail(err)}
 }));return s,nil
 }) }
func ServerURL(value any)any{return effect(func(context.Context)(any,error){return value.(*server).http.URL,nil})}
func CheckServer(value any)any{return effect(func(context.Context)(any,error){s:=value.(*server);s.mu.Lock();defer s.mu.Unlock();return rt.Unit{},s.err})}
func CloseServer(value any)any{return rt.IO(func(context.Context)(any,error){s:=value.(*server);s.http.CloseClientConnections();s.http.Close();return rt.Unit{},nil})}
func ParsePath(value any)any { u,err:=url.Parse(value.(string));if err!=nil{return rt.Tuple2Of(false,rt.Tuple2Of("",""))};return rt.Tuple2Of(true,rt.Tuple2Of(u.Path,u.Query().Encode())) }
func CanonicalQuery(value any)any { q,_:=url.ParseQuery(value.(string));return q.Encode() }
func MkdirTemp(prefix any)any{return effect(func(context.Context)(any,error){return os.MkdirTemp(os.TempDir(),prefix.(string))})}
func RemoveAll(path any)any{return effect(func(context.Context)(any,error){return rt.Unit{},os.RemoveAll(path.(string))})}
func RemovePath(path any)any{return effect(func(context.Context)(any,error){return rt.Unit{},os.Remove(path.(string))})}
func TempFile(prefix any)any{return effect(func(context.Context)(any,error){
 file,err:=os.CreateTemp("",prefix.(string));if err!=nil{return nil,err}
 var identity any=file
 value,_:=rt.OptionValue(hostdeps.From(reflect.ValueOf(&identity).Elem()))
 return value,nil
})}
func Panic(message any)any{return rt.IO(func(context.Context)(any,error){return nil,&rt.Failure{Kind:rt.Panic,Message:fmt.Sprint(message)}})}

// Directory enumeration and external OpenAPI service/codec adapters only.
func ReadDocuments(root any)any{return effect(func(context.Context)(any,error){
 var documents []any
 err:=filepath.WalkDir(root.(string),func(path string,d fs.DirEntry,err error)error{
  if err!=nil{return err};if d.IsDir()||filepath.Ext(path)!=".json"{return nil}
  data,err:=os.ReadFile(path);if err!=nil{return err};rel,err:=filepath.Rel(root.(string),path);if err!=nil{return err}
  documents=append(documents,rt.Tuple2Of(filepath.ToSlash(strings.TrimSuffix(rel,filepath.Ext(rel))),string(data)));return nil
 });if err!=nil{return nil,err};return list(documents),nil
})}
type openAPIMux struct { *http.ServeMux }
func(m openAPIMux)HandlePrefix(path string,handler http.Handler){m.Handle(path,handler)}
type openAPI struct{ service *handler3.OpenAPIService; mux openAPIMux }
func NewOpenAPI()any{return effect(func(context.Context)(any,error){
 s:=&openAPI{handler3.NewOpenAPIService(),openAPIMux{http.NewServeMux()}}
 if err:=s.service.RegisterOpenAPIV3VersionedService("/openapi/v3",s.mux);err!=nil{return nil,err};return s,nil
})}
func UpdateOpenAPI(service,key,document any)any{return effect(func(context.Context)(any,error){
 spec:=new(spec3.OpenAPI);if err:=json.Unmarshal([]byte(document.(string)),spec);err!=nil{return nil,err}
 service.(*openAPI).service.UpdateGroupVersion(key.(string),spec)
 var identity any=spec
 value,_:=rt.OptionValue(hostdeps.From(reflect.ValueOf(&identity).Elem()))
 return value,nil
})}
func ServeOpenAPI(service,method,path,query,hs,body any)any{return effect(func(ctx context.Context)(any,error){
 target:=path.(string);if query.(string)!=""{target+="?"+query.(string)}
 request,err:=http.NewRequestWithContext(ctx,method.(string),target,strings.NewReader(body.(string)));if err!=nil{return nil,err}
 each(hs,func(p any){k,v:=rt.Shapes.Untuple2(p);each(v,func(x any){request.Header.Add(k.(string),x.(string))})})
 response:=httptest.NewRecorder();service.(*openAPI).mux.ServeHTTP(response,request)
 return rt.Tuple2Of(int64(response.Code),rt.Tuple2Of(headers(response.Header()),response.Body.String())),nil
})}
