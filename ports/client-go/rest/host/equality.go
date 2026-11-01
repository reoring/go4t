// Copyright 2026 The Kubernetes Authors. Licensed under Apache-2.0.
package host

import (
 "context"
 "net/http"
 "reflect"
 "go4t/rt"
 deps "github.com/reoring/gofx/ports/client-go/host/deps"
 "k8s.io/apimachinery/pkg/runtime"
)
func EqualObject(a,b any) any {return rt.IO(func(ctx context.Context)(any,error){left:=deps.To(reflect.TypeFor[runtime.Object](),rt.SomeOf(a),ctx).Interface();right:=deps.To(reflect.TypeFor[runtime.Object](),rt.SomeOf(b),ctx).Interface();return reflect.DeepEqual(left,right),nil})}
func EqualSerializer(a,b any) any {return rt.IO(func(ctx context.Context)(any,error){left:=deps.To(reflect.TypeFor[runtime.NegotiatedSerializer](),rt.SomeOf(a),ctx).Interface();right:=deps.To(reflect.TypeFor[runtime.NegotiatedSerializer](),rt.SomeOf(b),ctx).Interface();return reflect.DeepEqual(left,right),nil})}
func EqualHTTPTransport(a,b any) any {return rt.IO(func(ctx context.Context)(any,error){left:=deps.To(reflect.TypeFor[http.RoundTripper](),rt.SomeOf(a),ctx).Interface();right:=deps.To(reflect.TypeFor[http.RoundTripper](),rt.SomeOf(b),ctx).Interface();return reflect.DeepEqual(left,right),nil})}
