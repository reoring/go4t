// Copyright 2026 The Kubernetes Authors. Licensed under Apache-2.0.
package host

import (
 "context"
 "crypto/x509"
 "net"
 "net/url"
 "reflect"
 "time"
 "gofx/rt"
 deps "github.com/reoring/gofx/ports/client-go/host/deps"
)
func RawQuery(value any) any {return rt.IO(func(ctx context.Context)(any,error){return deps.To(reflect.TypeFor[*url.URL](),rt.SomeOf(value),ctx).Interface().(*url.URL).RawQuery,nil})}
func WithDeadline(nanos, action any) any {return rt.IO(func(ctx context.Context)(any,error){child,cancel:=context.WithTimeout(ctx,time.Duration(nanos.(int64)));defer cancel();return action.(rt.IO)(child)})}
func ValidCA(value string) bool {return x509.NewCertPool().AppendCertsFromPEM([]byte(value))}
func HostPort(host,port string) string {return net.JoinHostPort(host,port)}
