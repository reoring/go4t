// Copyright 2026 The go4t authors. Licensed under Apache-2.0.
// Generic local fixtures only; no client-go implementation or network service.
package testsupport
import("context";"io";"net";"os";"reflect";"runtime";"strings";"go4t/rt";"github.com/reoring/gofx/ports/client-go/host/deps")
func NewFile(data any)any{return rt.IO(func(context.Context)(any,error){f,e:=os.CreateTemp("","go4t-transport-*");if e!=nil{return nil,e};name:=f.Name();if _,e=f.WriteString(data.(string));e!=nil{f.Close();os.Remove(name);return nil,e};if e=f.Close();e!=nil{os.Remove(name);return nil,e};return name,nil})}
func WriteFile(path,data any)any{return rt.IO(func(context.Context)(any,error){e:=os.WriteFile(path.(string),[]byte(data.(string)),0600);return rt.Unit{},e})}
func RemoveFile(path any)any{return rt.IO(func(context.Context)(any,error){e:=os.Remove(path.(string));if os.IsNotExist(e){e=nil};return rt.Unit{},e})}
func Pipe()any{return rt.IO(func(context.Context)(any,error){a,b:=net.Pipe();av,_:=rt.OptionValue(hostdeps.From(reflect.ValueOf(&a).Elem()));bv,_:=rt.OptionValue(hostdeps.From(reflect.ValueOf(&b).Elem()));return rt.Tuple2Of(av,bv),nil})}
func SameLifetime(a,b any)any{return rt.IO(func(context.Context)(any,error){return a==b,nil})}
func Collect()any{return rt.IO(func(context.Context)(any,error){runtime.GC();runtime.GC();runtime.Gosched();return rt.Unit{},nil})}
type observedBody struct{io.Reader;ctx context.Context;onClose any}
func(b *observedBody)Close()error{action:=reflect.ValueOf(b.onClose).Call(nil)[0].Interface().(rt.IO);_,err:=action(context.WithoutCancel(b.ctx));return err}
func Body(onClose any)any{return rt.IO(func(ctx context.Context)(any,error){var body io.ReadCloser=&observedBody{strings.NewReader("payload"),ctx,onClose};v,_:=rt.OptionValue(hostdeps.From(reflect.ValueOf(&body).Elem()));return v,nil})}
