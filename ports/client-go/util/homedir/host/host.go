// Copyright 2016 The Kubernetes Authors. Licensed under Apache-2.0.
package host
import("context";"os";"path/filepath";"runtime";"gofx/rt")
func Platform() string{return runtime.GOOS}
func Getenv(name string) rt.IO{return func(context.Context)(any,error){return os.Getenv(name),nil}}
func Join(a,b,c string) string{return filepath.Join(a,b,c)}
func Stat(path string) rt.IO{return func(context.Context)(any,error){s,e:=os.Stat(path);if e!=nil{return rt.NoneOf(),nil};return rt.SomeOf(rt.Tuple2Of(s.IsDir(),s.Mode().Perm()&0200!=0)),nil}}
