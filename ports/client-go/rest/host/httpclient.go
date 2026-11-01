// Copyright 2026 The Kubernetes Authors. Licensed under Apache-2.0.
package host

import (
 "context"
 "net/http"
 "reflect"
 "time"
 "go4t/rt"
 deps "github.com/reoring/gofx/ports/client-go/host/deps"
)
func NewHTTPClient(value any,nanos int64) rt.IO {return rt.IO(func(ctx context.Context)(any,error){transport:=deps.To(reflect.TypeFor[http.RoundTripper](),rt.SomeOf(value),ctx).Interface().(http.RoundTripper);return boxed(&http.Client{Transport:transport,Timeout:time.Duration(nanos)}),nil})}
func HTTPClientDo(client,request any) any {return rt.IO(func(ctx context.Context)(any,error){c:=deps.To(reflect.TypeFor[*http.Client](),rt.SomeOf(client),ctx).Interface().(*http.Client);r:=deps.To(reflect.TypeFor[*http.Request](),rt.SomeOf(request),ctx).Interface().(*http.Request);response,err:=c.Do(r);var value any=rt.NoneOf();if response!=nil{value=rt.SomeOf(boxed(response))};return rt.Tuple2Of(value,errOption(err)),nil})}
func HTTPClientTimeout(client any) any {return rt.IO(func(ctx context.Context)(any,error){c:=deps.To(reflect.TypeFor[*http.Client](),rt.SomeOf(client),ctx).Interface().(*http.Client);return int64(c.Timeout),nil})}
func IsDefaultHTTPTransport(value any) any {return rt.IO(func(ctx context.Context)(any,error){transport:=deps.To(reflect.TypeFor[http.RoundTripper](),rt.SomeOf(value),ctx).Interface().(http.RoundTripper);return transport==http.DefaultTransport,nil})}
func DefaultHTTPClient() any {return rt.IO(func(context.Context)(any,error){return boxed(http.DefaultClient),nil})}
