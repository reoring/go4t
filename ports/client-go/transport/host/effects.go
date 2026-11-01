// Copyright 2015 The Kubernetes Authors.
// Licensed under the Apache License, Version 2.0 (http://www.apache.org/licenses/LICENSE-2.0).
// Generic HTTP/TLS/file/crypto effects only. Authentication, cache and refresh
// policy are implemented by the calling native gofx module.
package host

import (
 "bytes"
 "context"
 "crypto/tls"
 "crypto/x509"
 "encoding/base64"
 "encoding/pem"
 "errors"
 "fmt"
 "net"
 "net/http"
 "net/url"
 "os"
 "reflect"
 "runtime"
 "sync"
 "time"
 "weak"
 "gofx/rt"
 "github.com/reoring/gofx/ports/client-go/host/deps"
 utilnet "k8s.io/apimachinery/pkg/util/net"
)
func effect(f func(context.Context)(any,error)) any { return rt.IO(func(ctx context.Context)(any,error){if e:=ctx.Err(); e!=nil{return nil,e};return f(ctx)}) }
func value[T any](ctx context.Context,v any) T {t:=reflect.TypeFor[T]();if t.Kind()==reflect.Interface{if _,ok:=v.(hostdeps.Value);ok{v=rt.SomeOf(v)}};converted:=hostdeps.To(t,v,ctx);if converted.Kind()==reflect.Interface&&converted.IsNil(){var zero T;return zero};return converted.Interface().(T)}
func external(v any) any {return hostdeps.From(reflect.ValueOf(v))}
func iface[T any](v T) any {out:=hostdeps.From(reflect.ValueOf(&v).Elem());if unwrapped,ok:=rt.OptionValue(out);ok{return unwrapped};return out}
func request(ctx context.Context,v any)*http.Request{return value[*http.Request](ctx,rt.SomeOf(v))}
func response(ctx context.Context,v any)*http.Response{return value[*http.Response](ctx,rt.SomeOf(v))}
func foreignError(err error)any {return hostdeps.ErrorOptionFromGo(err)}
func pair(v any,err error)any{return rt.Tuple2Of(v,foreignError(err))}
func call(ctx context.Context,fn any,args ...any)(any,error){v:=reflect.ValueOf(fn);in:=make([]reflect.Value,len(args));for i,arg:=range args{in[i]=reflect.ValueOf(arg)};out:=v.Call(in)[0].Interface();return out.(rt.IO)(ctx)}
func tuple(v any)(any,any){return rt.Shapes.Untuple2(v)}
func listOf(xs []any)any{result:=rt.Shapes.Nil();for i:=len(xs)-1;i>=0;i--{result=rt.Shapes.Cons(xs[i],result)};return result}
func NewError(message any)any{return hostdeps.ErrorFromGo(errors.New(message.(string)))}
func WrapError(message,cause any)any{return hostdeps.ErrorFromGo(fmt.Errorf("%s: %w",message.(string),hostdeps.ErrorToGo(cause)))}
func ReadFile(path any)any{return effect(func(context.Context)(any,error){b,e:=os.ReadFile(path.(string));if e!=nil{return pair(rt.NoneOf(),e),nil};return pair(rt.SomeOf(string(b)),nil),nil})}
func CloneRequest(v any)any{return effect(func(ctx context.Context)(any,error){r:=request(ctx,v);c:=new(http.Request);*c=*r;c.Header=r.Header.Clone();if c.Header==nil{c.Header=make(http.Header)};o,_:=rt.OptionValue(external(c));return o,nil})}
func RequestHeader(v,name any)any{return effect(func(ctx context.Context)(any,error){return request(ctx,v).Header.Get(name.(string)),nil})}
func RequestHeaders(v any)any{return effect(func(ctx context.Context)(any,error){h:=request(ctx,v).Header;xs:=make([]any,0,len(h));for k,vs:=range h{xs=append(xs,rt.Tuple2Of(k,rt.ListFromStrings(vs)))};return listOf(xs),nil})}
func SetRequestHeader(v,k,s any)any{return effect(func(ctx context.Context)(any,error){r:=request(ctx,v);if r.Header==nil{r.Header=make(http.Header)};r.Header.Set(k.(string),s.(string));return rt.Unit{},nil})}
func AddRequestHeader(v,k,s any)any{return effect(func(ctx context.Context)(any,error){r:=request(ctx,v);if r.Header==nil{r.Header=make(http.Header)};r.Header.Add(k.(string),s.(string));return rt.Unit{},nil})}
func DeleteRequestHeader(v,k any)any{return effect(func(ctx context.Context)(any,error){request(ctx,v).Header.Del(k.(string));return rt.Unit{},nil})}
func BasicCredentials(u,p any)any{return "Basic "+base64.StdEncoding.EncodeToString([]byte(u.(string)+":"+p.(string)))}
func ByteValues(s any)any{b:=[]byte(s.(string));xs:=make([]int64,len(b));for i,v:=range b{xs[i]=int64(v)};return rt.ListFromInts(xs)}
func ByteString(n any)any{return string([]byte{byte(n.(int64))})}
func HexByte(n any)any{return fmt.Sprintf("%02X",byte(n.(int64)))}
func ResponseStatus(v any)any{return effect(func(ctx context.Context)(any,error){return int64(response(ctx,v).StatusCode),nil})}
func RequestURL(v any)any{return effect(func(ctx context.Context)(any,error){r:=request(ctx,v);if r.URL==nil{return "",nil};return r.URL.String(),nil})}
func RequestMethod(v any)any{return effect(func(ctx context.Context)(any,error){return request(ctx,v).Method,nil})}
func DefaultHTTPTransport()any{return effect(func(context.Context)(any,error){return iface[http.RoundTripper](http.DefaultTransport),nil})}
func CancelHTTP(v,r any)any{return effect(func(ctx context.Context)(any,error){base:=value[http.RoundTripper](ctx,v);for base!=nil{if c,ok:=base.(interface{CancelRequest(*http.Request)});ok{c.CancelRequest(request(ctx,r));break};w,ok:=base.(interface{WrappedRoundTripper()http.RoundTripper});if !ok{break};base=w.WrappedRoundTripper()};return rt.Unit{},nil})}
func CloseHTTP(v any)any{return effect(func(ctx context.Context)(any,error){base:=value[http.RoundTripper](ctx,v);if c,ok:=base.(interface{CloseIdleConnections()});ok{c.CloseIdleConnections()};return rt.Unit{},nil})}
type callbackTransport struct{run,cancel,close,unwrap any;ctx context.Context}
func(t *callbackTransport)RoundTrip(req *http.Request)(*http.Response,error){ctx:=req.Context();r,_:=rt.OptionValue(external(req));v,e:=call(ctx,t.run,r);if e!=nil{return nil,e};a,b:=tuple(v);return value[*http.Response](ctx,a),value[error](ctx,b)}
func(t *callbackTransport)CancelRequest(req *http.Request){r,_:=rt.OptionValue(external(req));if _,e:=call(context.WithoutCancel(req.Context()),t.cancel,r);e!=nil{panic(e)}}
func(t *callbackTransport)CloseIdleConnections(){if _,e:=call(context.WithoutCancel(t.ctx),t.close);e!=nil{panic(e)}}
func(t *callbackTransport)WrappedRoundTripper()http.RoundTripper{v,e:=call(context.WithoutCancel(t.ctx),t.unwrap);if e!=nil{panic(e)};return value[http.RoundTripper](t.ctx,v)}
func BridgeHTTP(run,cancel,close,unwrap any)any{return effect(func(ctx context.Context)(any,error){return iface[http.RoundTripper](&callbackTransport{run,cancel,close,unwrap,context.WithoutCancel(ctx)}),nil})}
func WrappedHTTP(v any)any{return effect(func(ctx context.Context)(any,error){base:=value[http.RoundTripper](ctx,v);if wrapper,ok:=base.(interface{WrappedRoundTripper()http.RoundTripper});ok{inner:=wrapper.WrappedRoundTripper();if inner!=nil{return rt.SomeOf(iface[http.RoundTripper](inner)),nil}};return rt.NoneOf(),nil})}
func DefaultDial(network,address any)any{return effect(func(ctx context.Context)(any,error){d:=net.Dialer{Timeout:30*time.Second,KeepAlive:30*time.Second};c,e:=d.DialContext(ctx,network.(string),address.(string));if c==nil{return pair(rt.NoneOf(),e),nil};return pair(rt.SomeOf(iface[net.Conn](c)),e),nil})}
func ConstructHTTP(config,dial,proxy,disable any)any{return effect(func(ctx context.Context)(any,error){tc:=value[*tls.Config](ctx,config);transport:=&http.Transport{TLSClientConfig:tc,TLSHandshakeTimeout:10*time.Second,MaxIdleConnsPerHost:25,DisableCompression:disable.(bool)};transport.DialContext=func(c context.Context,n,a string)(net.Conn,error){v,e:=call(c,dial,n,a);if e!=nil{return nil,e};x,y:=tuple(v);return value[net.Conn](c,x),value[error](c,y)};transport.Proxy=http.ProxyFromEnvironment;if p,ok:=rt.OptionValue(proxy);ok{transport.Proxy=func(req *http.Request)(*url.URL,error){r,_:=rt.OptionValue(external(req));v,e:=call(req.Context(),p,r);if e!=nil{return nil,e};x,y:=tuple(v);return value[*url.URL](req.Context(),x),value[error](req.Context(),y)}};return iface[http.RoundTripper](utilnet.SetTransportDefaults(transport)),nil})}
func ConfigureTLS(insecure,server,protocols,roots,callback any)any{return effect(func(ctx context.Context)(any,error){c:=&tls.Config{MinVersion:tls.VersionTLS12,InsecureSkipVerify:insecure.(bool),ServerName:server.(string),NextProtos:value[[]string](ctx,protocols),RootCAs:value[*x509.CertPool](ctx,roots)};if cb,ok:=rt.OptionValue(callback);ok{c.GetClientCertificate=func(*tls.CertificateRequestInfo)(*tls.Certificate,error){v,e:=call(context.WithoutCancel(ctx),cb);if e!=nil{return nil,e};x,y:=tuple(v);return value[*tls.Certificate](ctx,x),value[error](ctx,y)}};v,_:=rt.OptionValue(external(c));return v,nil})}
func EmptyRoots()any{return effect(func(context.Context)(any,error){v,_:=rt.OptionValue(external(x509.NewCertPool()));return v,nil})}
func ParseRoots(data any)any{return effect(func(context.Context)(any,error){b:=[]byte(data.(string));p:=x509.NewCertPool();if p.AppendCertsFromPEM(b){return pair(external(p),nil),nil};for len(b)>0{block,rest:=pem.Decode(b);if block==nil{return pair(rt.NoneOf(),errors.New("unable to parse bytes as PEM block")),nil};b=rest;if block.Type!="CERTIFICATE"||len(block.Headers)!=0{continue};if _,e:=x509.ParseCertificate(block.Bytes);e!=nil{return pair(rt.NoneOf(),fmt.Errorf("failed to parse certificate: %w",e)),nil}};return pair(rt.NoneOf(),errors.New("no valid certificate authority data seen")),nil})}
func ParseCertificate(cert,key any)any{return effect(func(context.Context)(any,error){c,e:=tls.X509KeyPair([]byte(cert.(string)),[]byte(key.(string)));return pair(external(&c),e),nil})}
func EmptyCertificate()any{return effect(func(context.Context)(any,error){v,_:=rt.OptionValue(external(&tls.Certificate{}));return v,nil})}
func equalByteMatrix(a,b [][]byte)bool{if len(a)!=len(b){return false};for i:=range a{if !bytes.Equal(a[i],b[i]){return false}};return true}
func CertificatesEqual(a,b any)any{return effect(func(ctx context.Context)(any,error){x:=value[*tls.Certificate](ctx,a);y:=value[*tls.Certificate](ctx,b);if x==nil||y==nil{return x==y,nil};return equalByteMatrix(x.Certificate,y.Certificate)&&reflect.DeepEqual(x.PrivateKey,y.PrivateKey)&&equalByteMatrix(x.SignedCertificateTimestamps,y.SignedCertificateTimestamps)&&bytes.Equal(x.OCSPStaple,y.OCSPStaple),nil})}
func CloneHTTPRoots(v,r any)any{return effect(func(ctx context.Context)(any,error){t:=value[http.RoundTripper](ctx,v).(*http.Transport).Clone();t.TLSClientConfig.RootCAs=value[*x509.CertPool](ctx,rt.SomeOf(r));return iface[http.RoundTripper](t),nil})}
func IdentityNumber(v any)any{return effect(func(context.Context)(any,error){return int64(reflect.ValueOf(v).Pointer()),nil})}
type lifetime struct{value any;marker byte}
func NewLifetime(v,cleanup any)any{return effect(func(ctx context.Context)(any,error){l:=&lifetime{value:v};background:=context.WithoutCancel(ctx);runtime.AddCleanup(l,func(action any){_,_=action.(rt.IO)(background)},cleanup);return l,nil})}
func WeakLifetime(v any)any{return effect(func(context.Context)(any,error){return weak.Make(v.(*lifetime)),nil})}
func DerefLifetime(v any)any{return effect(func(context.Context)(any,error){l:=v.(weak.Pointer[lifetime]).Value();if l==nil{return rt.NoneOf(),nil};return rt.SomeOf(l),nil})}
func LifetimeValue(v any)any{return effect(func(context.Context)(any,error){return v.(*lifetime).value,nil})}
func KeepLifetime(v any)any{return effect(func(context.Context)(any,error){runtime.KeepAlive(v);return rt.Unit{},nil})}
func CloseConnection(v any)any{return effect(func(ctx context.Context)(any,error){c:=value[net.Conn](ctx,v);if c!=nil{_ = c.Close()};return rt.Unit{},nil})}
type trackedConn struct{net.Conn;once sync.Once;closed any;ctx context.Context}
func(c *trackedConn)Close()error{err:=c.Conn.Close();c.once.Do(func(){_,_=call(context.WithoutCancel(c.ctx),c.closed)});return err}
func TrackConnection(v,closed any)any{return effect(func(ctx context.Context)(any,error){return iface[net.Conn](&trackedConn{Conn:value[net.Conn](ctx,v),closed:closed,ctx:context.WithoutCancel(ctx)}),nil})}
func CloseRequestBody(v any)any{return effect(func(ctx context.Context)(any,error){r:=request(ctx,v);if r.Body==nil{return rt.NoneOf(),nil};return foreignError(r.Body.Close()),nil})}
