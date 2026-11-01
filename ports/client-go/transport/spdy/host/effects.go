// Copyright 2015 The Kubernetes Authors. Licensed under Apache-2.0.
// Generic stream effects and external SPDY framing. No client-go dependency.
package host

import (
 "context"
 "crypto/tls"
 "errors"
 "io"
 "net/http"
 "net/url"
 "reflect"
 "sync"
 "time"
 "gofx/rt"
 "github.com/reoring/gofx/ports/client-go/host/deps"
 stream "k8s.io/streaming/pkg/httpstream"
 spdy "k8s.io/streaming/pkg/httpstream/spdy"
)
func effect(f func(context.Context)(any,error)) any { return rt.IO(func(c context.Context)(any,error){if e:=c.Err();e!=nil{return nil,e};return f(c)}) }
func value[T any](c context.Context,v any)T{t:=reflect.TypeFor[T]();if t.Kind()==reflect.Interface{if _,ok:=v.(hostdeps.Value);ok{v=rt.SomeOf(v)}};return hostdeps.To(t,v,c).Interface().(T)}
func external(v any)any{return hostdeps.From(reflect.ValueOf(v))}
func iface[T any](v T)any{o:=hostdeps.From(reflect.ValueOf(&v).Elem());if x,ok:=rt.OptionValue(o);ok{return x};return o}
func ferr(e error)any{if e==nil{return rt.NoneOf()};return rt.SomeOf(iface[error](e))}
func pair(v any,e error)any{return rt.Tuple2Of(v,ferr(e))}
var readBuffers=sync.Pool{New:func()any{return new([32768]byte)}}
func Read(v,n any)any{return effect(func(c context.Context)(any,error){r:=value[io.Reader](c,v);size:=n.(int64);var b []byte;if size<=32768{buffer:=readBuffers.Get().(*[32768]byte);defer readBuffers.Put(buffer);b=buffer[:size]}else{b=make([]byte,size)};if closer,ok:=r.(io.Closer);ok{stop:=context.AfterFunc(c,func(){_ = closer.Close()});defer stop()};count,e:=r.Read(b);return pair(string(b[:count]),e),nil})}
func Write(v,b any)any{return effect(func(c context.Context)(any,error){w:=value[io.Writer](c,v);if closer,ok:=w.(io.Closer);ok{stop:=context.AfterFunc(c,func(){_ = closer.Close()});defer stop()};n,e:=io.WriteString(w,b.(string));return pair(int64(n),e),nil})}
func CloseReader(v any)any{return effect(func(c context.Context)(any,error){if x,ok:=value[io.Reader](c,v).(io.Closer);ok{_ = x.Close()};return rt.Unit{},nil})}
func CloseWriter(v any)any{return effect(func(c context.Context)(any,error){if x,ok:=value[io.Writer](c,v).(io.Closer);ok{_ = x.Close()};return rt.Unit{},nil})}
func StreamReader(v any)any{return effect(func(c context.Context)(any,error){return iface[io.Reader](value[stream.Stream](c,v)),nil})}
func StreamWriter(v any)any{return effect(func(c context.Context)(any,error){return iface[io.Writer](value[stream.Stream](c,v)),nil})}
func CreateStream(v,h any)any{return effect(func(c context.Context)(any,error){headers:=http.Header{};for {x,t,ok:=rt.Shapes.Uncons(h);if !ok{break};k,v:=rt.Shapes.Untuple2(x);headers.Add(k.(string),v.(string));h=t};s,e:=value[stream.Connection](c,v).CreateStream(headers);if s==nil{return pair(rt.NoneOf(),e),nil};return pair(rt.SomeOf(iface[stream.Stream](s)),e),nil})}
func AwaitClose(v any)any{return effect(func(c context.Context)(any,error){select{case <-value[stream.Connection](c,v).CloseChan():return rt.Unit{},nil;case <-c.Done():return nil,c.Err()}})}
func RemoveStreams(v,xs any)any{return effect(func(c context.Context)(any,error){var ss []stream.Stream;for{x,t,ok:=rt.Shapes.Uncons(xs);if !ok{break};ss=append(ss,value[stream.Stream](c,x));xs=t};value[stream.Connection](c,v).RemoveStreams(ss...);return rt.Unit{},nil})}
func IsEOF(v any)any{return errors.Is(value[error](context.Background(),v),io.EOF)}
func ByteLength(v any)any{return int64(len(v.(string)))}
func ByteSlice(v,n any)any{return v.(string)[n.(int64):]}
func NewUpgrade(tc,proxy,ping any)any{return effect(func(c context.Context)(any,error){var p func(*http.Request)(*url.URL,error)=http.ProxyFromEnvironment;if fn,ok:=rt.OptionValue(proxy);ok{p=func(r *http.Request)(*url.URL,error){arg,_:=rt.OptionValue(external(r));f:=reflect.ValueOf(fn);o:=f.Call([]reflect.Value{reflect.ValueOf(arg)})[0].Interface().(rt.IO);out,e:=o(r.Context());if e!=nil{return nil,e};a,b:=rt.Shapes.Untuple2(out);return value[*url.URL](c,a),value[error](c,b)}};u,e:=spdy.NewRoundTripperWithConfig(spdy.RoundTripperConfig{TLS:value[*tls.Config](c,tc),Proxier:p,PingPeriod:time.Duration(ping.(int64))});if e!=nil{return pair(rt.NoneOf(),e),nil};return pair(rt.SomeOf(iface[stream.UpgradeRoundTripper](u)),nil),nil})}
func UpgradeTransport(v any)any{return effect(func(c context.Context)(any,error){return iface[http.RoundTripper](value[stream.UpgradeRoundTripper](c,v)),nil})}
func UpgradeConnection(v,r any)any{return effect(func(c context.Context)(any,error){conn,e:=value[stream.UpgradeRoundTripper](c,v).NewConnection(value[*http.Response](c,rt.SomeOf(r)));if conn==nil{return pair(rt.NoneOf(),e),nil};return pair(rt.SomeOf(iface[stream.Connection](conn)),e),nil})}
func NewRequest(method,address any)any{return effect(func(c context.Context)(any,error){r,e:=http.NewRequestWithContext(c,method.(string),address.(string),nil);return pair(external(r),e),nil})}
func ClientDo(v,r any)any{return effect(func(c context.Context)(any,error){resp,e:=value[*http.Client](c,rt.SomeOf(v)).Do(value[*http.Request](c,rt.SomeOf(r)));return pair(external(resp),e),nil})}
func NewClient(v,reject any)any{return effect(func(c context.Context)(any,error){client:=&http.Client{Transport:value[http.RoundTripper](c,v)};if reject.(bool){client.CheckRedirect=func(*http.Request,[]*http.Request)error{return errors.New("redirect not allowed")}};out,_:=rt.OptionValue(external(client));return out,nil})}
func CloseResponse(v any)any{return effect(func(c context.Context)(any,error){r:=value[*http.Response](c,rt.SomeOf(v));if r.Body!=nil{return ferr(r.Body.Close()),nil};return rt.NoneOf(),nil})}
func ResponseHeader(v,k any)any{return effect(func(c context.Context)(any,error){return value[*http.Response](c,rt.SomeOf(v)).Header.Get(k.(string)),nil})}
func Headers(v any)any{return effect(func(c context.Context)(any,error){xs:=rt.Shapes.Nil();for k,values:=range value[stream.Stream](c,v).Headers(){for _,s:=range values{xs=rt.Shapes.Cons(rt.Tuple2Of(k,s),xs)}};return xs,nil})}
func Idle(v,nanos any)any{return effect(func(c context.Context)(any,error){value[stream.Connection](c,v).SetIdleTimeout(time.Duration(nanos.(int64)));return rt.Unit{},nil})}
func ShortWrite()any{return iface[error](io.ErrShortWrite)}
