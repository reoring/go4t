// Package hostsync supplies generic effects, not client-go algorithms.
package hostsync

import (
 "context"
 "fmt"
 "reflect"
 "sync"
 "go4t/rt"
)

func effect(f func(context.Context) (any, error)) any {
 return rt.IO(func(ctx context.Context) (value any, err error) {
  defer func() { if p := recover(); p != nil { value, err = nil, &rt.Failure{Kind: rt.Panic, Message: fmt.Sprint(p)} } }()
  if err = ctx.Err(); err != nil { return nil, rt.ClassifyError(err) }
  value, err = f(ctx)
  return value, rt.ClassifyError(err)
 })
}
func failure(message string) error { return &rt.Failure{Kind: rt.FailureKind, Message: message} }

type reference struct { mu sync.RWMutex; value any }
func NewRef(value any) any { return effect(func(context.Context) (any,error) { return &reference{value:value},nil }) }
func ReadRef(ref any) any { return effect(func(context.Context) (any,error) { r:=ref.(*reference); r.mu.RLock(); defer r.mu.RUnlock(); return r.value,nil }) }
func WriteRef(ref, value any) any { return effect(func(context.Context) (any,error) { r:=ref.(*reference); r.mu.Lock(); defer r.mu.Unlock(); r.value=value; return rt.Unit{},nil }) }
func SameRef(a,b any) any { return a.(*reference)==b.(*reference) }

type mutex struct { token chan struct{} }
type executionKey struct{}
type execution struct { marker byte }
type ownership struct { mu sync.Mutex; held bool; execution *execution }
func NewMutex() any { return effect(func(context.Context) (any,error) { m:=&mutex{token:make(chan struct{},1)}; m.token<-struct{}{}; return m,nil }) }
func acquire(ctx context.Context, m *mutex) error {
 select { case <-ctx.Done(): return ctx.Err(); case <-m.token: }
 if err:=ctx.Err(); err!=nil { m.token<-struct{}{}; return err }; return nil
}
func WithLock(handle, action any) any { return effect(func(ctx context.Context) (any,error) {
 m:=handle.(*mutex)
 if err:=acquire(ctx,m); err!=nil { return nil,err }
 identity,_:=ctx.Value(executionKey{}).(*execution)
 if identity==nil { identity=&execution{}; ctx=context.WithValue(ctx,executionKey{},identity) }
 owner:=&ownership{held:true,execution:identity}
 defer func() { owner.mu.Lock(); defer owner.mu.Unlock(); if owner.held { owner.held=false; m.token<-struct{}{} } }()
 return action.(rt.IO)(context.WithValue(ctx,m,owner))
}) }

type condition struct { mutex *mutex; mu sync.Mutex; waiters []*waiter }
type waiter struct { ready chan struct{} }
func NewCondition(handle any) any { return effect(func(context.Context) (any,error) { return &condition{mutex:handle.(*mutex)},nil }) }
func owned(ctx context.Context, c *condition) (*ownership,error) {
 owner,_:=ctx.Value(c.mutex).(*ownership)
 identity,_:=ctx.Value(executionKey{}).(*execution)
 if owner==nil || identity!=owner.execution { return nil,failure("condition: associated mutex is not held") }
 owner.mu.Lock(); held:=owner.held; owner.mu.Unlock()
 if !held { return nil,failure("condition: associated mutex is not held") }; return owner,nil
}
func Wait(handle any) any { return effect(func(ctx context.Context) (any,error) {
 c:=handle.(*condition); owner,err:=owned(ctx,c); if err!=nil { return nil,err }
 w:=&waiter{ready:make(chan struct{})}
 c.mu.Lock(); c.waiters=append(c.waiters,w); c.mu.Unlock()
 owner.mu.Lock(); owner.held=false; c.mutex.token<-struct{}{}; owner.mu.Unlock()
 select { case <-w.ready: case <-ctx.Done(): }
 c.mu.Lock()
 for i,v:=range c.waiters { if v==w { copy(c.waiters[i:],c.waiters[i+1:]); c.waiters[len(c.waiters)-1]=nil; c.waiters=c.waiters[:len(c.waiters)-1]; break } }
 c.mu.Unlock()
 // Reacquisition is mandatory even when cancellation won the wait.
 <-c.mutex.token
 owner.mu.Lock(); owner.held=true; owner.mu.Unlock()
 return rt.Unit{},ctx.Err()
}) }
func notify(handle any, all bool) any { return effect(func(ctx context.Context) (any,error) {
 c:=handle.(*condition); if _,err:=owned(ctx,c); err!=nil { return nil,err }
 c.mu.Lock(); defer c.mu.Unlock()
 n:=len(c.waiters); if !all && n>1 { n=1 }
 for _,w:=range c.waiters[:n] { close(w.ready) }
 copy(c.waiters,c.waiters[n:]); rest:=len(c.waiters)-n
 clear(c.waiters[rest:]); c.waiters=c.waiters[:rest]
 return rt.Unit{},nil
}) }
func Signal(handle any) any { return notify(handle,false) }
func Broadcast(handle any) any { return notify(handle,true) }

// closing wakes blocked senders before Close takes the exclusive send barrier.
// The barrier orders every native send before the native close, including races.
// Registration and native close are serialized so multi-channel selections
// never wait for a writer while already holding another channel's read barrier.
var sendRegistration sync.Mutex
type channel struct { values chan any; mu sync.Mutex; sends sync.RWMutex; closing chan struct{}; closed bool }
func makeChannel(capacity int64) *channel { return &channel{values:make(chan any,capacity),closing:make(chan struct{})} }
func NewChannel(capacity any) any { return effect(func(context.Context) (any,error) {
 n:=capacity.(int64); if n<0 { return nil,failure("newChannel: negative capacity") }
 return makeChannel(n),nil
}) }
func Send(handle,value any) any { return effect(func(ctx context.Context) (any,error) {
 c:=handle.(*channel); c.sends.RLock(); defer c.sends.RUnlock()
 select {case <-c.closing: return nil,&rt.Failure{Kind:rt.Panic,Message:"send on closed channel"};default:}
 select {
 case c.values<-value: return rt.Unit{},nil
 case <-c.closing: return nil,&rt.Failure{Kind:rt.Panic,Message:"send on closed channel"}
 case <-ctx.Done(): return nil,ctx.Err()
 }
}) }
func TrySend(handle,value any) any { return effect(func(context.Context)(any,error){
 c:=handle.(*channel);c.sends.RLock();defer c.sends.RUnlock()
 select {
 case c.values<-value:return true,nil
 case <-c.closing:return nil,&rt.Failure{Kind:rt.Panic,Message:"send on closed channel"}
 default:return false,nil
 }
}) }
func Receive(handle any) any { return effect(func(ctx context.Context) (any,error) {
 select { case v,ok:=<-handle.(*channel).values: if !ok { return rt.NoneOf(),nil }; return rt.SomeOf(v),nil; case <-ctx.Done(): return nil,ctx.Err() }
}) }
// TryReceiveRaw is the private source adapter's (ready, optional value) bridge.
func TryReceiveRaw(handle any) any { return effect(func(context.Context)(any,error){
 select {
 case value,ok:=<-handle.(*channel).values:
  if !ok {return rt.Tuple2Of(true,rt.NoneOf()),nil}
  return rt.Tuple2Of(true,rt.SomeOf(value)),nil
 default:
  return rt.Tuple2Of(false,rt.NoneOf()),nil
 }
}) }
func Close(handle any) any { return effect(func(context.Context) (any,error) {
 c:=handle.(*channel); c.mu.Lock(); defer c.mu.Unlock()
 if c.closed { return nil,&rt.Failure{Kind:rt.Panic,Message:"close of closed channel"} }
 close(c.closing)
 sendRegistration.Lock(); defer sendRegistration.Unlock()
 c.sends.Lock(); defer c.sends.Unlock()
 close(c.values); c.closed=true
 return rt.Unit{},nil
}) }
func SelectReceive(handles any) any { return effect(func(ctx context.Context) (any,error) {
 cases:=[]reflect.SelectCase{{Dir:reflect.SelectRecv,Chan:reflect.ValueOf(ctx.Done())}}
 for xs:=handles;; { h,t,ok:=rt.Shapes.Uncons(xs); if !ok { break }; cases=append(cases,reflect.SelectCase{Dir:reflect.SelectRecv,Chan:reflect.ValueOf(h.(*channel).values)}); xs=t }
 index,v,ok:=reflect.Select(cases)
 if index==0 { return nil,ctx.Err() }
 value:=rt.NoneOf(); if ok { value=rt.SomeOf(v.Interface()) }
 return rt.Tuple2Of(int64(index-1),value),nil
}) }

type task struct { cancel context.CancelFunc; done chan struct{}; value any; err error }
func start(action any, detached bool) any { return effect(func(ctx context.Context) (any,error) {
 if detached { ctx=context.WithoutCancel(ctx) }
 child,cancel:=context.WithCancel(context.WithValue(ctx,executionKey{},&execution{})); t:=&task{cancel:cancel,done:make(chan struct{})}
 go func() { defer close(t.done); defer cancel(); t.value,t.err=effect(func(ctx context.Context)(any,error){ return action.(rt.IO)(ctx) }).(rt.IO)(child) }()
 return t,nil
}) }
func Start(action any) any { return start(action,false) }
func StartDetached(action any) any { return start(action,true) }
func CancelTask(handle any) any { return effect(func(context.Context)(any,error){ handle.(*task).cancel(); return rt.Unit{},nil }) }
func Join(handle any) any { return effect(func(ctx context.Context)(any,error){ t:=handle.(*task); select { case <-t.done: return t.value,t.err; case <-ctx.Done(): return nil,ctx.Err() } }) }
func Uninterruptible(action any) any {
 // Do not use effect: entry is permitted with an already-cancelled context.
 return rt.IO(func(ctx context.Context)(any,error){ return effect(func(clean context.Context)(any,error){ return action.(rt.IO)(clean) }).(rt.IO)(context.WithoutCancel(ctx)) })
}
