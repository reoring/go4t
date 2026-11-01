// Copyright 2023 The Kubernetes Authors. Licensed under Apache-2.0.
// Gorilla/socket and codec effects only; negotiation policy is native go4t.
package host
import (
 "context"
 "crypto/tls"
 "encoding/json"
 "errors"
 "io"
 "net/http"
 "net/url"
 "reflect"
 "time"
 "go4t/rt"
 "github.com/reoring/gofx/ports/client-go/host/deps"
 ws "github.com/gorilla/websocket"
 apierrors "k8s.io/apimachinery/pkg/api/errors"
 metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
 stream "k8s.io/streaming/pkg/httpstream"
 utilnet "k8s.io/apimachinery/pkg/util/net"
)
func effect(f func(context.Context)(any,error))any{return rt.IO(func(c context.Context)(any,error){if e:=c.Err();e!=nil{return nil,e};return f(c)})}
func value[T any](c context.Context,v any)T{t:=reflect.TypeFor[T]();if t.Kind()==reflect.Interface{if _,ok:=v.(hostdeps.Value);ok{v=rt.SomeOf(v)}};return hostdeps.To(t,v,c).Interface().(T)}
func external(v any)any{return hostdeps.From(reflect.ValueOf(v))}
func iface[T any](v T)any{o:=hostdeps.From(reflect.ValueOf(&v).Elem());if x,ok:=rt.OptionValue(o);ok{return x};return o}
func ferr(e error)any{if e==nil{return rt.NoneOf()};return rt.SomeOf(iface[error](e))}
func RequestScheme(v any)any{return effect(func(c context.Context)(any,error){return value[*http.Request](c,rt.SomeOf(v)).URL.Scheme,nil})}
func Dial(r,tc,p,protocols,scheme any)any{return effect(func(c context.Context)(any,error){req:=value[*http.Request](c,rt.SomeOf(r));u:=*req.URL;u.Scheme=scheme.(string);proxy:=utilnet.NewProxierWithNoProxyCIDR(http.ProxyFromEnvironment);if fn,ok:=rt.OptionValue(p);ok{proxy=func(req *http.Request)(*url.URL,error){a,_:=rt.OptionValue(external(req));o:=reflect.ValueOf(fn).Call([]reflect.Value{reflect.ValueOf(a)})[0].Interface().(rt.IO);v,e:=o(req.Context());if e!=nil{return nil,e};x,y:=rt.Shapes.Untuple2(v);return value[*url.URL](c,x),value[error](c,y)}};d:=ws.Dialer{TLSClientConfig:value[*tls.Config](c,tc),Proxy:proxy,Subprotocols:value[[]string](c,rt.SomeOf(protocols)),ReadBufferSize:33792,WriteBufferSize:33792};conn,resp,e:=d.DialContext(c,u.String(),req.Header);return rt.Tuple2Of(external(conn),rt.Tuple2Of(external(resp),ferr(e))),nil})}
func IsBadHandshake(e any)any{return errors.Is(value[error](context.Background(),e),ws.ErrBadHandshake)}
func UpgradeFailure(e any)any{return iface[error](&stream.UpgradeFailureError{Cause:value[error](context.Background(),e)})}
func IsUpgradeFailure(e any)any{return stream.IsUpgradeFailure(value[error](context.Background(),e))}
func IsHTTPSProxyError(e any)any{return stream.IsHTTPSProxyError(value[error](context.Background(),e))}
func StatusText(r any)any{return effect(func(c context.Context)(any,error){return value[*http.Response](c,rt.SomeOf(r)).Status,nil})}
func ErrorBody(r,limit any)any{return effect(func(c context.Context)(any,error){b,e:=io.ReadAll(io.LimitReader(value[*http.Response](c,rt.SomeOf(r)).Body,limit.(int64)));return rt.Tuple2Of(string(b),ferr(e)),nil})}
func DecodeStatus(b any)any{var s metav1.Status;if e:=json.Unmarshal([]byte(b.(string)),&s);e!=nil||s.Kind!="Status"{return rt.NoneOf()};return rt.SomeOf(iface[error](&apierrors.StatusError{ErrStatus:s}))}
func ReadMessage(v any)any{return effect(func(c context.Context)(any,error){conn:=value[*ws.Conn](c,rt.SomeOf(v));stop:=context.AfterFunc(c,func(){_ = conn.Close()});defer stop();t,b,e:=conn.ReadMessage();if ws.IsCloseError(e,ws.CloseNormalClosure){e=io.EOF};return rt.Tuple2Of(int64(t),rt.Tuple2Of(string(b),ferr(e))),nil})}
func WriteMessage(v,kind,data,deadline any)any{return effect(func(c context.Context)(any,error){conn:=value[*ws.Conn](c,rt.SomeOf(v));_ = conn.SetWriteDeadline(time.Now().Add(time.Duration(deadline.(int64))));return ferr(conn.WriteMessage(int(kind.(int64)),[]byte(data.(string)))),nil})}
func Control(v,kind,data,deadline any)any{return effect(func(c context.Context)(any,error){return ferr(value[*ws.Conn](c,rt.SomeOf(v)).WriteControl(int(kind.(int64)),[]byte(data.(string)),time.Now().Add(time.Duration(deadline.(int64))))),nil})}
func Heartbeat(v,deadline any)any{return effect(func(c context.Context)(any,error){conn:=value[*ws.Conn](c,rt.SomeOf(v));d:=time.Duration(deadline.(int64));conn.SetPongHandler(func(string)error{return conn.SetReadDeadline(time.Now().Add(d))});return ferr(conn.SetReadDeadline(time.Now().Add(d))),nil})}
func IsNormalClose(v any)any{return ws.IsCloseError(value[error](context.Background(),v),ws.CloseNormalClosure)}
func WriteFrame(v,prefix,data,deadline any)any{return effect(func(c context.Context)(any,error){conn:=value[*ws.Conn](c,rt.SomeOf(v));if deadline.(int64)>0{if e:=conn.SetWriteDeadline(time.Now().Add(time.Duration(deadline.(int64))));e!=nil{return rt.Tuple2Of(int64(0),ferr(e)),nil}};w,e:=conn.NextWriter(ws.BinaryMessage);if e!=nil{return rt.Tuple2Of(int64(0),ferr(e)),nil};if p:=prefix.(string);p!=""{if _,e=w.Write([]byte(p));e!=nil{_ = w.Close();return rt.Tuple2Of(int64(0),ferr(e)),nil}};n,e:=w.Write([]byte(data.(string)));ce:=w.Close();if e==nil{e=ce};return rt.Tuple2Of(int64(n),ferr(e)),nil})}
func UpgradeFailureCause(v any)any{var target *stream.UpgradeFailureError;if errors.As(value[error](context.Background(),v),&target){return ferr(target.Cause)};return rt.NoneOf()}
func ReadTimeout(v,nanos any)any{return effect(func(c context.Context)(any,error){deadline:=time.Time{};if nanos.(int64)>0{deadline=time.Now().Add(time.Duration(nanos.(int64)))};return ferr(value[*ws.Conn](c,rt.SomeOf(v)).SetReadDeadline(deadline)),nil})}
