// Copyright 2026 The go4t Authors. Licensed under Apache-2.0.
// Real TCP/SPDY peer and an explicitly closable blocking stdin for lifecycle proof.
package host

import (
 "context"
 "fmt"
 "io"
 "net"
 "sync"
 "sync/atomic"
 "time"
 "go4t/rt"
 stream "k8s.io/streaming/pkg/httpstream"
 spdy "k8s.io/streaming/pkg/httpstream/spdy"
)

type enteredReader struct{reader *io.PipeReader;entered chan struct{};once sync.Once;closed atomic.Bool}
func(r *enteredReader)Read(p []byte)(int,error){r.once.Do(func(){close(r.entered)});return r.reader.Read(p)}
func(r *enteredReader)Close()error{r.closed.Store(true);return r.reader.Close()}
type cancellationFixture struct{client,server stream.Connection;listener net.Listener;input *enteredReader;writer *io.PipeWriter;ready chan struct{};done chan struct{};serverError error;close sync.Once}
func OpenCancellation()any{return rt.IO(func(ctx context.Context)(any,error){
 listener,e:=net.Listen("tcp4","127.0.0.1:0");if e!=nil{return nil,e}
 reader,writer:=io.Pipe()
 f:=&cancellationFixture{listener:listener,input:&enteredReader{reader:reader,entered:make(chan struct{})},writer:writer,ready:make(chan struct{}),done:make(chan struct{})}
 type incoming struct{stream stream.Stream;reply <-chan struct{}}
 arrivals:=make(chan incoming,3);connected:=make(chan error,1)
 go func(){socket,e:=listener.Accept();if e!=nil{connected<-e;return};_ = socket.SetDeadline(time.Now().Add(10*time.Second));f.server,e=spdy.NewServerConnection(socket,func(s stream.Stream,reply <-chan struct{})error{arrivals<-incoming{s,reply};return nil});connected<-e}()
 socket,e:=(&net.Dialer{Timeout:5*time.Second}).DialContext(ctx,"tcp4",listener.Addr().String());if e!=nil{listener.Close();reader.Close();writer.Close();return nil,e};_ = socket.SetDeadline(time.Now().Add(10*time.Second))
 f.client,e=spdy.NewClientConnection(socket);if e!=nil{socket.Close();listener.Close();reader.Close();writer.Close();return nil,e};if e=<-connected;e!=nil{f.client.Close();listener.Close();reader.Close();writer.Close();return nil,e}
 go func(){defer close(f.done);seen:=map[string]bool{};timer:=time.NewTimer(10*time.Second);defer timer.Stop();for range 3{select{case a:=<-arrivals:select{case <-a.reply:case <-timer.C:f.serverError=fmt.Errorf("stream reply timeout");return};seen[a.stream.Headers().Get("streamType")]=true;case <-timer.C:f.serverError=fmt.Errorf("stream creation timeout");return;case <-f.server.CloseChan():f.serverError=fmt.Errorf("connection closed before streams were ready");return}}
  if !seen["error"]||!seen["stdin"]||!seen["stdout"]{f.serverError=fmt.Errorf("unexpected cancellation stream set: %v",seen);return};close(f.ready);<-f.server.CloseChan()
 }()
 return f,nil
})}
func CancellationConnection(v any)any{return rt.IO(func(context.Context)(any,error){return iface[stream.Connection](v.(*cancellationFixture).client),nil})}
func CancellationInput(v any)any{return rt.IO(func(context.Context)(any,error){return iface[io.Reader](v.(*cancellationFixture).input),nil})}
func CancellationReady(v any)any{return rt.IO(func(ctx context.Context)(any,error){f:=v.(*cancellationFixture);timer:=time.NewTimer(10*time.Second);defer timer.Stop();for _,ch:=range []<-chan struct{}{f.ready,f.input.entered}{select{case <-ch:case <-f.done:if f.serverError!=nil{return nil,f.serverError};return nil,fmt.Errorf("server stopped before stdin entered");case <-timer.C:return nil,fmt.Errorf("cancellation fixture readiness timeout");case <-ctx.Done():return nil,ctx.Err()}};return rt.Unit{},nil})}
func CancellationComplete(v any)any{return rt.IO(func(ctx context.Context)(any,error){f:=v.(*cancellationFixture);select{case <-f.done:if f.serverError!=nil{return nil,f.serverError};return f.input.closed.Load(),nil;case <-ctx.Done():return nil,ctx.Err()}})}
func CloseCancellation(v any)any{return rt.IO(func(context.Context)(any,error){f:=v.(*cancellationFixture);f.close.Do(func(){_ = f.input.Close();_ = f.writer.Close();_ = f.client.Close();_ = f.server.Close();_ = f.listener.Close();<-f.done});return rt.Unit{},nil})}
