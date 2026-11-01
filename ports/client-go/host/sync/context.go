package hostsync

import (
 "context"
 "time"
 "go4t/rt"
)

type capturedContext struct { context context.Context }
type cancelHandle struct { cancel context.CancelFunc }
func CaptureContext() any { return effect(func(ctx context.Context)(any,error){return &capturedContext{ctx},nil}) }
func ContextCancelled(handle any) any { return effect(func(context.Context)(any,error){return handle.(*capturedContext).context.Err()!=nil,nil}) }
func ContextRemaining(handle any) any { return effect(func(context.Context)(any,error){
 deadline,ok:=handle.(*capturedContext).context.Deadline();if !ok{return rt.NoneOf(),nil}
 return rt.SomeOf(int64(time.Until(deadline))),nil
}) }
func WithContext(handle,action any) any { return rt.IO(func(caller context.Context)(any,error){
 // Restore the captured cancellation/value environment, never its ownership.
 // A typed nil shadows a captured identity when the caller has no identity.
 identity,_:=caller.Value(executionKey{}).(*execution)
 restored:=context.WithValue(handle.(*capturedContext).context,executionKey{},identity)
 return effect(func(ctx context.Context)(any,error){return action.(rt.IO)(ctx)}).(rt.IO)(restored)
}) }

func NewContext(parent,timeoutNanos any) any { return effect(func(context.Context)(any,error){
 var child context.Context
 var cancel context.CancelFunc
 nanos,hasTimeout:=rt.Shapes.OptionValue(timeoutNanos)
 if hasTimeout {child,cancel=context.WithTimeout(parent.(*capturedContext).context,time.Duration(nanos.(int64)))} else {child,cancel=context.WithCancel(parent.(*capturedContext).context)}
 return rt.Tuple2Of(&capturedContext{child},&cancelHandle{cancel}),nil
}) }
func CancelContext(handle any) any { return effect(func(context.Context)(any,error){handle.(*cancelHandle).cancel();return rt.Unit{},nil}) }

// invoke contains both callback construction and execution in the same panic
// boundary. Bracket must not use rt.Bind between acquisition and registration.
func invoke(ctx context.Context, construct func() any) (any,error) {
 return effect(func(ctx context.Context)(any,error){return construct().(rt.IO)(ctx)}).(rt.IO)(ctx)
}
func Ensuring(action,cleanup any) any { return rt.IO(func(ctx context.Context)(any,error){
 value,err:=invoke(ctx,func()any{return action})
 _,cleanupErr:=invoke(context.WithoutCancel(ctx),func()any{return cleanup})
 if err!=nil{return nil,err};if cleanupErr!=nil{return nil,cleanupErr};return value,nil
}) }
func Bracket(acquire,use,release any) any { return rt.IO(func(ctx context.Context)(any,error){
 resource,err:=invoke(ctx,func()any{return acquire});if err!=nil{return nil,err}
 // Acquisition succeeded: cleanup is now unconditional, including a context
 // cancellation observed before constructing use or a panic in either callback.
 value,err:=invoke(ctx,func()any{return use.(func(any)any)(resource)})
 _,cleanupErr:=invoke(context.WithoutCancel(ctx),func()any{return release.(func(any)any)(resource)})
 if err!=nil{return nil,err};if cleanupErr!=nil{return nil,cleanupErr};return value,nil
}) }
