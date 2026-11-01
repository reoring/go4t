// Copyright 2026 The Kubernetes Authors. Licensed under Apache-2.0.
package host

import (
 "context"
 "io"
 "net/http"
 "strings"
 "go4t/rt"
)
// Response constructs a standard HTTP response for injected transports.
func Response(status int64,media,body,retry string) rt.IO {return func(ctx context.Context)(any,error){r:=&http.Response{StatusCode:int(status),Header:make(http.Header),Body:io.NopCloser(strings.NewReader(body))};r.Header.Set("Content-Type",media);if retry!="" {r.Header.Set("Retry-After",retry)};return boxed(r),nil}}
func StringReader(data string) any {return boxedReader(strings.NewReader(data))}
type errorReader struct { err error }
func (r errorReader) Read([]byte) (int,error) { return 0,r.err }
func FaultResponse(status,media,prefix,cause any) any {return rt.IO(func(ctx context.Context)(any,error){body:=io.NopCloser(io.MultiReader(strings.NewReader(prefix.(string)),errorReader{unboxError(cause)}));r:=&http.Response{StatusCode:int(status.(int64)),Header:make(http.Header),Body:body};r.Header.Set("Content-Type",media.(string));return boxed(r),nil})}
