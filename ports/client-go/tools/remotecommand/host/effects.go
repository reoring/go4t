// Copyright 2016 The Kubernetes Authors. Licensed under Apache-2.0.
// Generic immutable status decoding and structured exit error construction.
package host
import (
 "context"
 "encoding/json"
 "errors"
 "io"
 "reflect"
 "go4t/rt"
 "github.com/reoring/gofx/ports/client-go/host/deps"
 metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)
func iface[T any](v T)any{o:=hostdeps.From(reflect.ValueOf(&v).Elem());if x,ok:=rt.OptionValue(o);ok{return x};return o}
func ferr(e error)any{if e==nil{return rt.NoneOf()};return rt.SomeOf(iface[error](e))}
func DecodeStatusPure(message any)any{var s metav1.Status;e:=json.Unmarshal([]byte(message.(string)),&s);causes:=rt.NoneOf();if s.Details!=nil{var items []any;for _,c:=range s.Details.Causes{items=append(items,rt.Tuple2Of(string(c.Type),c.Message))};xs:=rt.Shapes.Nil();for i:=len(items)-1;i>=0;i--{xs=rt.Shapes.Cons(items[i],xs)};causes=rt.SomeOf(xs)};return rt.Tuple2Of(s.Status,rt.Tuple2Of(string(s.Reason),rt.Tuple2Of(s.Message,rt.Tuple2Of(causes,ferr(e)))))}
type exitError struct{message string;code int}
func(e *exitError)Error()string{return e.message}
func(e *exitError)String()string{return e.message}
func(e *exitError)Exited()bool{return true}
func(e *exitError)ExitStatus()int{return e.code}
func ExitError(message,code any)any{return iface[error](&exitError{message.(string),int(code.(int64))})}
func ExitCode(v any)any{e:=hostdeps.To(reflect.TypeFor[error](),rt.SomeOf(v),context.Background()).Interface().(error);var target interface{ExitStatus()int};if errors.As(e,&target){return rt.SomeOf(int64(target.ExitStatus()))};return rt.NoneOf()}
func EOF()any{return iface[error](io.EOF)}
func ByteCode(s any)any{b:=s.(string);if len(b)==0{return int64(-1)};return int64(b[0])}
func BytePrefix(s,n any)any{return s.(string)[:n.(int64)]}
