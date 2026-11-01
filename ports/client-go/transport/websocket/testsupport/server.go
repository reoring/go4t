// Copyright 2026 The go4t Authors. Licensed under Apache-2.0.
package testsupport
import (
 "context"
 "net/http"
 "net/http/httptest"
 "sync"
 "go4t/rt"
 ws "github.com/gorilla/websocket"
)
type server struct{http *httptest.Server;mu sync.Mutex;connections []*ws.Conn}
func New(protocol,status any)any{return rt.IO(func(context.Context)(any,error){s:=&server{};s.http=httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter,r *http.Request){if status.(int64)!=101{w.Header().Set("Content-Type","application/json");w.WriteHeader(int(status.(int64)));_,_=w.Write([]byte(`{"kind":"Status","apiVersion":"v1","status":"Failure","reason":"Forbidden","message":"denied","code":403}`));return};u:=ws.Upgrader{Subprotocols:[]string{protocol.(string)}};c,e:=u.Upgrade(w,r,nil);if e!=nil{return};s.mu.Lock();s.connections=append(s.connections,c);s.mu.Unlock()}));return s,nil})}
func URL(v any)any{return rt.IO(func(context.Context)(any,error){return v.(*server).http.URL,nil})}
func Close(v any)any{return rt.IO(func(context.Context)(any,error){s:=v.(*server);s.mu.Lock();for _,c:=range s.connections{_ = c.Close()};s.mu.Unlock();s.http.Close();return rt.Unit{},nil})}
