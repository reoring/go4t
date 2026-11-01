// Copyright The Kubernetes Authors. SPDX-License-Identifier: Apache-2.0
package host
import (
 "context"
 "os"
 "time"
 "gofx/rt"
)
func TempDirectory()rt.IO{return func(ctx context.Context)(any,error){if e:=ctx.Err();e!=nil{return nil,e};return os.MkdirTemp("","gofx-discovery-")}}
func RemoveDirectory(path any)rt.IO{return func(context.Context)(any,error){return rt.Unit{},os.RemoveAll(path.(string))}}
func SetFileTime(path,nanos any)rt.IO{return func(context.Context)(any,error){t:=time.Unix(0,nanos.(int64));return rt.Unit{},os.Chtimes(path.(string),t,t)}}
