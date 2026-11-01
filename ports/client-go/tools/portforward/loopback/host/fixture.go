// Copyright 2026 The gofx Authors. Licensed under Apache-2.0.
// Test server implements the wire endpoint, never client-go orchestration.
package host
import (
 "context"
 "fmt"
 "io"
 "net"
 "reflect"
 "strconv"
 "time"
 "gofx/rt"
 "github.com/reoring/gofx/ports/client-go/host/deps"
 stream "k8s.io/streaming/pkg/httpstream"
 spdy "k8s.io/streaming/pkg/httpstream/spdy"
)
type fixture struct{client,server stream.Connection;listener net.Listener;done chan error}
func iface[T any](v T)any{o:=hostdeps.From(reflect.ValueOf(&v).Elem());if x,ok:=rt.OptionValue(o);ok{return x};return o}
func forwardingPayload()string{return "forwarded / 日本語 / café / \x00bytes\xff"}
func Open(fail any)any{return rt.IO(func(ctx context.Context)(any,error){
 l,e:=net.Listen("tcp4","127.0.0.1:0");if e!=nil{return nil,e};f:=&fixture{listener:l,done:make(chan error,1)}
 type incoming struct{s stream.Stream;ready <-chan struct{}}
 streams:=make(chan incoming,2);ready:=make(chan error,1)
 go func(){c,e:=l.Accept();if e!=nil{ready<-e;return};_ = c.SetDeadline(time.Now().Add(10*time.Second));f.server,e=spdy.NewServerConnection(c,func(s stream.Stream,r <-chan struct{})error{streams<-incoming{s,r};return nil});ready<-e}()
 c,e:=net.DialTimeout("tcp4",l.Addr().String(),5*time.Second);if e!=nil{l.Close();return nil,e};_ = c.SetDeadline(time.Now().Add(10*time.Second));f.client,e=spdy.NewClientConnection(c);if e!=nil{c.Close();l.Close();return nil,e};if e=<-ready;e!=nil{f.client.Close();l.Close();return nil,e}
 go func(){defer close(f.done);byKind:=map[string]stream.Stream{};for range 2{select{case a:=<-streams:<-a.ready;if a.s.Headers().Get("port")!="80"||a.s.Headers().Get("requestID")!="0"{f.done<-fmt.Errorf("incorrect forward headers: %v",a.s.Headers());return};byKind[a.s.Headers().Get("streamType")]=a.s;case <-ctx.Done():f.done<-ctx.Err();return;case <-time.After(10*time.Second):f.done<-fmt.Errorf("forward stream timeout");return}}
  _,e:=io.CopyN(byKind["data"],byKind["data"],int64(len(forwardingPayload())));_ = byKind["data"].Close();if fail.(bool){_,we:=io.WriteString(byKind["error"],"remote connection failed");if e==nil{e=we}};_ = byKind["error"].Close();f.done<-e
 }()
 return f,nil
})}
func Connection(v any)any{return rt.IO(func(context.Context)(any,error){return iface[stream.Connection](v.(*fixture).client),nil})}
func Exchange(port any)any{return rt.IO(func(ctx context.Context)(any,error){c,e:=(&net.Dialer{Timeout:5*time.Second}).DialContext(ctx,"tcp4",net.JoinHostPort("127.0.0.1",strconv.FormatInt(port.(int64),10)));if e!=nil{return nil,e};defer c.Close();_ = c.SetDeadline(time.Now().Add(10*time.Second));want:=forwardingPayload();for remaining:=want;len(remaining)>0;{size:=min(2,len(remaining));n,e:=io.WriteString(c,remaining[:size]);if e!=nil{return nil,e};if n!=size{return nil,io.ErrShortWrite};remaining=remaining[n:]};b:=make([]byte,len(want));if _,e=io.ReadFull(c,b);e!=nil{return nil,e};return string(b)==want,nil})}
func Result(v any)any{return rt.IO(func(ctx context.Context)(any,error){select{case e:=<-v.(*fixture).done:return e==nil,e;case <-ctx.Done():return nil,ctx.Err()}})}
func Close(v any)any{return rt.IO(func(context.Context)(any,error){f:=v.(*fixture);_ = f.client.Close();_ = f.server.Close();_ = f.listener.Close();<-f.done;return rt.Unit{},nil})}
