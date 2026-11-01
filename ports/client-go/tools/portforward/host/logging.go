// Copyright 2026 The gofx Authors. Licensed under Apache-2.0.
// Generic logger field and HTTP context conversion; log policy lives in .gfx.
package host

import (
 "context"
 "gofx/rt"
 "net/http"
 "github.com/go-logr/logr"
 "k8s.io/klog/v2"
)

func LogString(v any) any { return iface[any](v.(string)) }
func LogBytes(v any) any { return iface[any]([]byte(v.(string))) }
func LogInt(v any) any { return iface[any](int(v.(int64))) }
func LogError(v any) any { return iface[any](value[error](context.Background(),v)) }
func RequestLogger(request,logger any) any { return effect(func(c context.Context)(any,error) {
 req:=value[*http.Request](c,rt.SomeOf(request))
 return iface[*http.Request](req.WithContext(klog.NewContext(req.Context(),value[logr.Logger](c,logger)))),nil
}) }
