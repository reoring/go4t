// Copyright 2015 The Kubernetes Authors. Licensed under Apache-2.0.
// Generic local network and byte-stream effects. No forwarding controller.
package host
import (
 "context"
 "errors"
 "io"
 "net"
 "reflect"
 "strconv"
 "time"
 "go4t/rt"
 "github.com/reoring/gofx/ports/client-go/host/deps"
 netutils "k8s.io/utils/net"
 stream "k8s.io/streaming/pkg/httpstream"
 spdy "k8s.io/streaming/pkg/httpstream/spdy"
)
func effect(f func(context.Context)(any,error))any{return rt.IO(func(c context.Context)(any,error){if e:=c.Err();e!=nil{return nil,e};return f(c)})}
func value[T any](c context.Context,v any)T{t:=reflect.TypeFor[T]();if t.Kind()==reflect.Interface{if _,ok:=v.(hostdeps.Value);ok{v=rt.SomeOf(v)}};return hostdeps.To(t,v,c).Interface().(T)}
func iface[T any](v T)any{o:=hostdeps.From(reflect.ValueOf(&v).Elem());if x,ok:=rt.OptionValue(o);ok{return x};return o}
func ferr(e error)any{if e==nil{return rt.NoneOf()};return rt.SomeOf(iface[error](e))}
func ParsePort(s any)any{v,e:=strconv.ParseUint(s.(string),10,16);return rt.Tuple2Of(int64(v),ferr(e))}
func IPFamily(s any)any{ip:=netutils.ParseIPSloppy(s.(string));if ip==nil{return int64(0)};if ip.To4()!=nil{return int64(4)};return int64(6)}
func Listen(network,address,port any)any{return effect(func(c context.Context)(any,error){var lc net.ListenConfig;l,e:=lc.Listen(c,network.(string),net.JoinHostPort(address.(string),strconv.FormatInt(port.(int64),10)));if e!=nil{return rt.Tuple2Of(rt.NoneOf(),ferr(e)),nil};return rt.Tuple2Of(rt.SomeOf(iface[net.Listener](l)),rt.NoneOf()),nil})}
func Accept(v any)any{return effect(func(c context.Context)(any,error){l:=value[net.Listener](c,v);stop:=context.AfterFunc(c,func(){_ = l.Close()});defer stop();conn,e:=l.Accept();if e!=nil{return rt.Tuple2Of(rt.NoneOf(),ferr(e)),nil};return rt.Tuple2Of(rt.SomeOf(iface[net.Conn](conn)),rt.NoneOf()),nil})}
func ListenerPort(v any)any{return effect(func(c context.Context)(any,error){_,p,e:=net.SplitHostPort(value[net.Listener](c,v).Addr().String());if e!=nil{return nil,e};port,e:=strconv.ParseUint(p,10,16);return int64(port),e})}
func CloseListener(v any)any{return effect(func(c context.Context)(any,error){_ = value[net.Listener](c,v).Close();return rt.Unit{},nil})}
func ConnReader(v any)any{return effect(func(c context.Context)(any,error){return iface[io.Reader](value[net.Conn](c,v)),nil})}
func ConnWriter(v any)any{return effect(func(c context.Context)(any,error){return iface[io.Writer](value[net.Conn](c,v)),nil})}
func CloseConn(v any)any{return effect(func(c context.Context)(any,error){_ = value[net.Conn](c,v).Close();return rt.Unit{},nil})}
func JoinHostPort(address,port any)any{return net.JoinHostPort(address.(string),strconv.FormatInt(port.(int64),10))}
func ClientConnection(v,ping any)any{return effect(func(c context.Context)(any,error){conn,e:=spdy.NewClientConnectionWithPings(value[net.Conn](c,v),time.Duration(ping.(int64)));if conn==nil{return rt.Tuple2Of(rt.NoneOf(),ferr(e)),nil};return rt.Tuple2Of(rt.SomeOf(iface[stream.Connection](conn)),ferr(e)),nil})}
func EOF()any{return iface[error](io.EOF)}
func JoinErrors(a,b any)any{return ferr(errors.Join(value[error](context.Background(),a),value[error](context.Background(),b)))}
