// Copyright 2026 The go4t Authors. Licensed under Apache-2.0.
package host
import (
 "context"
 "fmt"
 "net/http"
 "net/http/httptest"
 "time"
 "go4t/rt"
 ws "github.com/gorilla/websocket"
)
type tunnelFixture struct{server *httptest.Server;done chan error}
func OpenTunnel()any{return rt.IO(func(ctx context.Context)(any,error){
 f:=&tunnelFixture{done:make(chan error,1)}
 f.server=httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter,r *http.Request){
  defer close(f.done)
  u:=ws.Upgrader{Subprotocols:[]string{"SPDY/3.1+portforward.k8s.io"}}
  c,e:=u.Upgrade(w,r,nil);if e!=nil{f.done<-e;return};defer c.Close()
  _ = c.SetReadDeadline(time.Now().Add(10*time.Second))
  if e=c.WriteMessage(ws.BinaryMessage,[]byte("abc"));e!=nil{f.done<-e;return}
  if e=c.WriteMessage(ws.BinaryMessage,[]byte("defg"));e!=nil{f.done<-e;return}
  kind,b,e:=c.ReadMessage();if e!=nil{f.done<-e;return}
  if kind!=ws.BinaryMessage||string(b)!="echo"{f.done<-fmt.Errorf("unexpected tunnel data");return}
  if e=c.WriteMessage(ws.BinaryMessage,b);e!=nil{f.done<-e;return}
  e=c.WriteControl(ws.CloseMessage,ws.FormatCloseMessage(ws.CloseNormalClosure,""),time.Now().Add(time.Second));f.done<-e
 }))
 return f,nil
})}
func TunnelAddress(v any)any{return rt.IO(func(context.Context)(any,error){return v.(*tunnelFixture).server.URL,nil})}
func TunnelResult(v any)any{return rt.IO(func(ctx context.Context)(any,error){select{case e:=<-v.(*tunnelFixture).done:return e==nil,e;case <-ctx.Done():return nil,ctx.Err()}})}
func CloseTunnel(v any)any{return rt.IO(func(context.Context)(any,error){v.(*tunnelFixture).server.Close();return rt.Unit{},nil})}
