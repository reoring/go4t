// Copyright 2014 The Kubernetes Authors. Licensed under Apache-2.0.
package host
import "runtime"
func Compiler() string{return runtime.Compiler}
func Platform() string{return runtime.GOOS+"/"+runtime.GOARCH}
