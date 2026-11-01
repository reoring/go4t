// Copyright 2014 The Kubernetes Authors. Licensed under Apache-2.0.
package host
import("context";"fmt";"os";"golang.org/x/term";"go4t/rt";"github.com/reoring/gofx/ports/client-go/host/deps")
func ReadToken()rt.IO{return func(ctx context.Context)(any,error){if e:=ctx.Err();e!=nil{return nil,e};var text string;_,e:=fmt.Fscan(os.Stdin,&text);return rt.Tuple2Of(text,hostdeps.ErrorOptionFromGo(e)),nil}}
func ReadPassword()rt.IO{return func(ctx context.Context)(any,error){if e:=ctx.Err();e!=nil{return nil,e};b,e:=term.ReadPassword(int(os.Stdin.Fd()));return rt.Tuple2Of(string(b),hostdeps.ErrorOptionFromGo(e)),nil}}
func IsTerminal()rt.IO{return func(context.Context)(any,error){return term.IsTerminal(int(os.Stdin.Fd())),nil}}
func Output(text string)rt.IO{return func(ctx context.Context)(any,error){if e:=ctx.Err();e!=nil{return nil,e};_,e:=fmt.Fprint(os.Stdout,text);return hostdeps.ErrorOptionFromGo(e),nil}}
