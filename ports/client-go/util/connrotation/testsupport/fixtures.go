// Copyright 2026 The Kubernetes Authors. Licensed under Apache-2.0.
package testsupport
import("context";"net";"reflect";"gofx/rt";"github.com/reoring/gofx/ports/client-go/host/deps")
func external(c net.Conn)any{v:=hostdeps.From(reflect.ValueOf(&c).Elem());v,_=rt.OptionValue(v);return v}
func Pipe()any{return rt.IO(func(context.Context)(any,error){a,b:=net.Pipe();return rt.Tuple2Of(external(a),external(b)),nil})}
func Closed(c any)any{return rt.IO(func(ctx context.Context)(any,error){conn:=hostdeps.To(reflect.TypeFor[net.Conn](),rt.SomeOf(c),ctx).Interface().(net.Conn);_,e:=conn.Write([]byte{1});return e!=nil,nil})}
