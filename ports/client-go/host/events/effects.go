// Copyright The Kubernetes Authors. SPDX-License-Identifier: Apache-2.0
// External data conversion and entropy only. Event policy lives in gofx.
package host

import (
 "context"
 "encoding/json"
 "fmt"
 "reflect"
 "os"
 "time"
 "github.com/google/uuid"
 "gofx/rt"
 "github.com/reoring/gofx/ports/client-go/host/deps"
 "k8s.io/apimachinery/pkg/api/meta"
 apierrors "k8s.io/apimachinery/pkg/api/errors"
 validation "k8s.io/apimachinery/pkg/api/validation"
 "k8s.io/apimachinery/pkg/runtime"
 "k8s.io/apimachinery/pkg/runtime/schema"
 "k8s.io/apimachinery/pkg/util/strategicpatch"
)
func from(v any) any { return hostdeps.From(reflect.ValueOf(v)) }
func errorOption(e error) any { if e==nil{return rt.NoneOf()};return rt.SomeOf(hostdeps.From(reflect.ValueOf(e))) }
func object(ctx context.Context,v any) runtime.Object { return hostdeps.To(reflect.TypeFor[runtime.Object](),rt.SomeOf(v),ctx).Interface().(runtime.Object) }
func Nanos(v any) any { return hostdeps.To(reflect.TypeFor[time.Time](),v,context.Background()).Interface().(time.Time).UnixNano() }
func Hex(n int64) string { return fmt.Sprintf("%x",n) }
func UUID() any { return rt.IO(func(context.Context)(any,error) { return uuid.NewString(),nil }) }
func ValidName(name string) bool { return len(validation.NameIsDNSSubdomain(name,false))==0 }
func CommonVersion(v any) any { return rt.IO(func(ctx context.Context)(any,error) { m,e:=meta.CommonAccessor(object(ctx,v)); version:="";if e==nil {version=m.GetResourceVersion()};return rt.Tuple2Of(version,errorOption(e)),nil }) }
func MergePatch(old,new,example any) any { return rt.IO(func(ctx context.Context)(any,error) { a,e:=json.Marshal(object(ctx,old));if e!=nil{return rt.Tuple2Of("",errorOption(e)),nil};b,e:=json.Marshal(object(ctx,new));if e!=nil{return rt.Tuple2Of("",errorOption(e)),nil};p,e:=strategicpatch.CreateTwoWayMergePatch(a,b,object(ctx,example));return rt.Tuple2Of(string(p),errorOption(e)),nil }) }
func Hostname() any { return rt.IO(func(context.Context)(any,error){name,e:=os.Hostname();return rt.Tuple2Of(name,errorOption(e)),nil}) }
func Log(message string) rt.IO { return rt.IO(func(context.Context)(any,error){fmt.Fprintln(os.Stderr,message);return rt.Unit{},nil}) }
func StatusError(v any) any { e:=hostdeps.To(reflect.TypeFor[error](),rt.SomeOf(v),context.Background()).Interface().(error);if status,ok:=e.(*apierrors.StatusError);ok{return rt.SomeOf(from(status.ErrStatus))};return rt.NoneOf() }
func TypeName(v any) any { return rt.IO(func(ctx context.Context)(any,error){return fmt.Sprintf("%T",object(ctx,v)),nil}) }
func KindsString(v any) any { kinds:=hostdeps.To(reflect.TypeFor[[]schema.GroupVersionKind](),v,context.Background()).Interface().([]schema.GroupVersionKind);return fmt.Sprint(kinds) }
