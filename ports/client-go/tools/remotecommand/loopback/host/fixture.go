// Copyright 2026 The gofx Authors. Licensed under Apache-2.0.
// Test-only real TCP/SPDY server; no original client-go imports.
package host
import (
 "bytes"
 "context"
 "encoding/json"
 "fmt"
 "io"
 "net"
 "net/http"
 "net/http/httptest"
 "reflect"
 "sync"
 "time"
 "gofx/rt"
 "github.com/reoring/gofx/ports/client-go/host/deps"
 ws "github.com/gorilla/websocket"
 stream "k8s.io/streaming/pkg/httpstream"
 spdy "k8s.io/streaming/pkg/httpstream/spdy"
)
type fixture struct{client stream.Connection;server stream.Connection;listener net.Listener;wsclient *ws.Conn;httpserver *httptest.Server;input *bytes.Reader;output bytes.Buffer;stderr bytes.Buffer;done chan error;close sync.Once}
func iface[T any](v T)any{o:=hostdeps.From(reflect.ValueOf(&v).Elem());if x,ok:=rt.OptionValue(o);ok{return x};return o}
func payload()string{return "native gofx streaming / 日本語 / café / \x00partial\xff"}
type chunkReader struct{source io.Reader}
func(r *chunkReader)Read(p []byte)(int,error){if len(p)>2{p=p[:2]};return r.source.Read(p)}
func Open(version,exit,tty any)any{return rt.IO(func(ctx context.Context)(any,error){
 listener,e:=net.Listen("tcp4","127.0.0.1:0");if e!=nil{return nil,e}
 f:=&fixture{listener:listener,input:bytes.NewReader([]byte(payload())),done:make(chan error,1)}
 type arrival struct{s stream.Stream;ready <-chan struct{}}
 incoming:=make(chan arrival,5)
 serverReady:=make(chan error,1)
 go func(){conn,e:=listener.Accept();if e!=nil{serverReady<-e;return};_ = conn.SetDeadline(time.Now().Add(10*time.Second));f.server,e=spdy.NewServerConnection(conn,func(s stream.Stream,ready <-chan struct{})error{incoming<-arrival{s,ready};return nil});serverReady<-e}()
 socket,e:=net.DialTimeout("tcp4",listener.Addr().String(),5*time.Second);if e!=nil{listener.Close();return nil,e};_ = socket.SetDeadline(time.Now().Add(10*time.Second));f.client,e=spdy.NewClientConnection(socket);if e!=nil{listener.Close();socket.Close();return nil,e};if e=<-serverReady;e!=nil{f.client.Close();listener.Close();return nil,e}
 go func(){
  defer close(f.done)
  count:=3;if !tty.(bool){count++};if tty.(bool)&&version.(int64)>=3{count++}
  streams:=map[string]stream.Stream{}
  for range count{select{case a:=<-incoming:<-a.ready;streams[a.s.Headers().Get("streamType")]=a.s;case <-ctx.Done():f.done<-ctx.Err();return;case <-time.After(10*time.Second):f.done<-fmt.Errorf("stream setup timeout");return}}
  if tty.(bool)&&streams["stderr"]!=nil{f.done<-fmt.Errorf("TTY created stderr stream");return}
  if s:=streams["stderr"];s!=nil{_ = s.Close()}
  if s:=streams["resize"];s!=nil{var size struct{Width,Height uint16};if e:=json.NewDecoder(s).Decode(&size);e!=nil{f.done<-e;return};if size.Width!=123||size.Height!=45{f.done<-fmt.Errorf("unexpected resize: %v",size);return};_ = s.Close()}
  _,e:=io.CopyN(streams["stdout"],streams["stdin"],int64(len(payload())))
  _ = streams["stdin"].Close();_ = streams["stdout"].Close()
  if version.(int64)>=4{_,we:=fmt.Fprintf(streams["error"],"{\"status\":\"Failure\",\"reason\":\"NonZeroExitCode\",\"details\":{\"causes\":[{\"reason\":\"ExitCode\",\"message\":\"%d\"}]}}",exit.(int64));if e==nil{e=we}}
  _ = streams["error"].Close();f.done<-e
 }()
 return f,nil
})}
func Connection(v any)any{return rt.IO(func(context.Context)(any,error){return iface[stream.Connection](v.(*fixture).client),nil})}
func Input(v any)any{return rt.IO(func(context.Context)(any,error){return iface[io.Reader](&chunkReader{source:v.(*fixture).input}),nil})}
func Output(v any)any{return rt.IO(func(context.Context)(any,error){return iface[io.Writer](&v.(*fixture).output),nil})}
func Stderr(v any)any{return rt.IO(func(context.Context)(any,error){return iface[io.Writer](&v.(*fixture).stderr),nil})}
func Result(v any)any{return rt.IO(func(ctx context.Context)(any,error){f:=v.(*fixture);select{case e:=<-f.done:if e!=nil{return nil,e};return f.output.String()==payload()&&f.stderr.Len()==0,nil;case <-ctx.Done():return nil,ctx.Err()}})}
func Close(v any)any{return rt.IO(func(context.Context)(any,error){f:=v.(*fixture);f.close.Do(func(){if f.client!=nil{_ = f.client.Close()};if f.server!=nil{_ = f.server.Close()};if f.listener!=nil{_ = f.listener.Close()};if f.wsclient!=nil{_ = f.wsclient.Close()};if f.httpserver!=nil{f.httpserver.Close()};<-f.done});return rt.Unit{},nil})}
func OpenWebsocket(exit any)any{return rt.IO(func(ctx context.Context)(any,error){
 f:=&fixture{input:bytes.NewReader([]byte(payload())),done:make(chan error,1)}
 f.httpserver=httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter,r *http.Request){
  defer close(f.done)
  upgrade:=ws.Upgrader{Subprotocols:[]string{"v5.channel.k8s.io"}}
  c,e:=upgrade.Upgrade(w,r,nil);if e!=nil{f.done<-e;return};defer c.Close()
  _ = c.SetReadDeadline(time.Now().Add(10*time.Second))
  for{kind,b,e:=c.ReadMessage();if e!=nil{f.done<-e;return};if kind!=ws.BinaryMessage||len(b)==0{f.done<-fmt.Errorf("invalid client frame");return}
   if b[0]==0{out:=append([]byte{1},b[1:]...);if e=c.WriteMessage(ws.BinaryMessage,out);e!=nil{f.done<-e;return}}
   if len(b)==2&&b[0]==255&&b[1]==0{status:=fmt.Sprintf("{\"status\":\"Failure\",\"reason\":\"NonZeroExitCode\",\"details\":{\"causes\":[{\"reason\":\"ExitCode\",\"message\":\"%d\"}]}}",exit.(int64));e=c.WriteMessage(ws.BinaryMessage,append([]byte{3},[]byte(status)...));if e==nil{e=c.WriteControl(ws.CloseMessage,ws.FormatCloseMessage(ws.CloseNormalClosure,""),time.Now().Add(time.Second))};f.done<-e;return}
  }
 }))
 d:=ws.Dialer{Subprotocols:[]string{"v5.channel.k8s.io"}}
 c,resp,e:=d.DialContext(ctx,"ws"+f.httpserver.URL[4:],nil);if resp!=nil{_ = resp.Body.Close()};if e!=nil{f.httpserver.Close();return nil,e};f.wsclient=c
 return f,nil
})}
func Websocket(v any)any{return rt.IO(func(context.Context)(any,error){out:=hostdeps.From(reflect.ValueOf(v.(*fixture).wsclient));value,_:=rt.OptionValue(out);return value,nil})}
