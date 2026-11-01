// Copyright 2026 The Kubernetes Authors. Licensed under Apache-2.0.
package host

import (
 "context"
 "fmt"
 "net"
 "net/http"
 "net/http/httptest"
 "net/url"
 "sync"
 "gofx/rt"
)
type scriptedReply struct {status int; body string; headers [][2]string}
func StartMetricServer(replies any) any {return rt.IO(func(ctx context.Context)(any,error){
 var script []scriptedReply
 for {entry,next,ok:=rt.Shapes.Uncons(replies);if !ok{break};status,rest:=rt.Shapes.Untuple2(entry);body,headers:=rt.Shapes.Untuple2(rest);reply:=scriptedReply{status:int(status.(int64)),body:body.(string)};for {header,next,ok:=rt.Shapes.Uncons(headers);if !ok{break};key,value:=rt.Shapes.Untuple2(header);reply.headers=append(reply.headers,[2]string{key.(string),value.(string)});headers=next};script=append(script,reply);replies=next}
 if len(script)==0{return nil,fmt.Errorf("empty HTTP reply script")}
 var lock sync.Mutex;index:=0
 server:=httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter,r *http.Request){lock.Lock();i:=index;if i>=len(script){i=len(script)-1};reply:=script[i];index++;lock.Unlock();for _,header:=range reply.headers{w.Header().Add(header[0],header[1])};w.WriteHeader(reply.status);_,_=w.Write([]byte(reply.body))}))
 parsed,_:=url.Parse(server.URL);_,port,_:=net.SplitHostPort(parsed.Host);parsed.Host=net.JoinHostPort("localhost",port)
 return rt.Tuple2Of(server,rt.Tuple2Of(parsed.String(),boxed(server.Client()))),nil
})}
func StopMetricServer(server any) any {return rt.IO(func(context.Context)(any,error){server.(*httptest.Server).Close();return rt.Unit{},nil})}
