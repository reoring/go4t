// Copyright 2024 The Kubernetes Authors. Licensed under Apache-2.0.
package host

import (
 "context"
 "log"
 "os"
 "reflect"
 "go4t/rt"
 "github.com/reoring/gofx/ports/client-go/host/deps"
 "k8s.io/apimachinery/pkg/util/version"
)

func LookupEnvironment(name string) rt.IO { return func(context.Context)(any,error) { value,ok:=os.LookupEnv(name); if !ok{return rt.NoneOf(),nil};return rt.SomeOf(value),nil } }
func ReportWarning(message string) rt.IO { return func(context.Context)(any,error) {log.Print(message);return rt.Unit{},nil} }
func ParseVersion(text string) rt.IO {return func(context.Context)(any,error){v,err:=version.ParseGeneric(text);if err!=nil{return nil,err};return hostdeps.From(reflect.ValueOf(*v)),nil}}
