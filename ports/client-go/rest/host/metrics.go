// Copyright 2026 The Kubernetes Authors. Licensed under Apache-2.0.
package host

import (
 "context"
 "net/http"
 "net/http/httptrace"
 "reflect"
 "strings"
 "go4t/rt"
 deps "github.com/reoring/gofx/ports/client-go/host/deps"
)
func RequestSizeInfo(request any) any {return rt.IO(func(ctx context.Context)(any,error){r:=deps.To(reflect.TypeFor[*http.Request](),rt.SomeOf(request),ctx).Interface().(*http.Request);return rt.Tuple2Of(r.ContentLength,r.Body!=nil),nil})}
func ByteLength(value string) int64 {return int64(len(value))}
func TrimPrefix(value,prefix string) string {return strings.TrimPrefix(value,prefix)}
func runTraceCallback(ctx context.Context,callback any,argument any){action:=callback.(func(any)any)(argument).(rt.IO);if _,err:=action(context.WithoutCancel(ctx));err!=nil{panic(err)}}
func AttachDNSCallbacks(request,start,done any) any {return rt.IO(func(ctx context.Context)(any,error){r:=deps.To(reflect.TypeFor[*http.Request](),rt.SomeOf(request),ctx).Interface().(*http.Request);trace:=&httptrace.ClientTrace{DNSStart:func(info httptrace.DNSStartInfo){runTraceCallback(r.Context(),start,info.Host)},DNSDone:func(httptrace.DNSDoneInfo){action:=done.(func()any)().(rt.IO);if _,err:=action(context.WithoutCancel(r.Context()));err!=nil{panic(err)}}};return boxed(r.WithContext(httptrace.WithClientTrace(r.Context(),trace))),nil})}
