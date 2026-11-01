// Copyright 2026 The gofx Authors. Licensed under Apache-2.0.
package host

// An immutable tagged sentinel retains errors.Is semantics without a mutable
// package-global registry. Tags and messages are supplied by native policy.
type taggedError struct{tag,message string}
func(e *taggedError)Error()string{return e.message}
func(e *taggedError)Is(target error)bool{other,ok:=target.(*taggedError);return ok&&other.tag==e.tag}
func TaggedError(tag,message any)any{return iface[error](&taggedError{tag.(string),message.(string)})}
