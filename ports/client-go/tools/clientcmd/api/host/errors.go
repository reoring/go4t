// Copyright 2014 The Kubernetes Authors. Licensed under Apache-2.0.
// Generic tagged error values. Tags, messages and aggregation policy are native.
package host
import("errors";"go4t/rt";"github.com/reoring/gofx/ports/client-go/host/deps")
type taggedError struct{tag,message string; causes []error}
func(e *taggedError)Error()string{return e.message}
func(e *taggedError)Unwrap()[]error{return e.causes}
func TaggedError(tag,message string,values any)any{var causes []error;for{v,rest,ok:=rt.Shapes.Uncons(values);if !ok{break};causes=append(causes,hostdeps.ErrorToGo(v));values=rest};return hostdeps.ErrorFromGo(&taggedError{tag,message,causes})}
func ErrorTag(value any)string{e:=hostdeps.ErrorToGo(value);var tagged *taggedError;if errors.As(e,&tagged){return tagged.tag};return ""}
func ErrorCauses(value any)any{e:=hostdeps.ErrorToGo(value);out:=rt.Shapes.Nil();if tagged,ok:=e.(*taggedError);ok{for i:=len(tagged.causes)-1;i>=0;i--{out=rt.Shapes.Cons(hostdeps.ErrorFromGo(tagged.causes[i]),out)}};return out}
