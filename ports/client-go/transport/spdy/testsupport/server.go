// Copyright 2026 The go4t Authors. Licensed under Apache-2.0.
package testsupport
import (
 "context"
 "net/http"
 "net/http/httptest"
 "sync"
 "go4t/rt"
 stream "k8s.io/streaming/pkg/httpstream"
 spdy "k8s.io/streaming/pkg/httpstream/spdy"
)
type server struct{http *httptest.Server;mu sync.Mutex;connections []stream.Connection;offered []string}
func New(protocol,status any)any{return rt.IO(func(context.Context)(any,error){f:=&server{};f.http=httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter,r *http.Request){f.mu.Lock();f.offered=append([]string(nil),r.Header.Values("X-Stream-Protocol-Version")...);f.mu.Unlock();if status.(int64)!=101{http.Error(w,"upgrade rejected",int(status.(int64)));return};w.Header().Set("X-Stream-Protocol-Version",protocol.(string));c:=spdy.NewResponseUpgrader().UpgradeResponse(w,r,stream.NoOpNewStreamHandler);if c!=nil{f.mu.Lock();f.connections=append(f.connections,c);f.mu.Unlock()}}));return f,nil})}
func URL(v any)any{return rt.IO(func(context.Context)(any,error){return v.(*server).http.URL,nil})}
func Offered(v any)any{return rt.IO(func(context.Context)(any,error){s:=v.(*server);s.mu.Lock();defer s.mu.Unlock();return rt.ListFromStrings(s.offered),nil})}
func Close(v any)any{return rt.IO(func(context.Context)(any,error){s:=v.(*server);s.mu.Lock();for _,c:=range s.connections{_ = c.Close()};s.mu.Unlock();s.http.Close();return rt.Unit{},nil})}
