// Copyright 2014 The Kubernetes Authors. Licensed under Apache-2.0.
package host
import("context";"os";"gofx/rt";"github.com/reoring/gofx/ports/client-go/host/deps")
func Readable(path string)rt.IO{return func(ctx context.Context)(any,error){if e:=ctx.Err();e!=nil{return nil,e};f,e:=os.Open(path);if e==nil{f.Close()};return hostdeps.ErrorOptionFromGo(e),nil}}
