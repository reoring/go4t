package hostsync

import (
 "context"
 "errors"
 "testing"
 "time"
 "go4t/rt"
)

func execute(ctx context.Context, action any) (any,error) { return action.(rt.IO)(ctx) }
func mustExecute(t *testing.T, action any) any { t.Helper(); value,err:=execute(context.Background(),action); if err!=nil { t.Fatal(err) }; return value }
func await(t *testing.T, done <-chan struct{}) { t.Helper(); select { case <-done: case <-time.After(time.Second): t.Fatal("effect did not complete") } }

// No shape registration is needed for these ownership/lifecycle regressions.
func TestCancelledConditionReacquiresBeforeRelease(t *testing.T) {
 m:=mustExecute(t,NewMutex()); c:=mustExecute(t,NewCondition(m))
 ctx,cancel:=context.WithCancel(context.Background()); defer cancel()
 entered:=make(chan struct{}); done:=make(chan struct{}); holder:=make(chan struct{}); release:=make(chan struct{})
 var waitErr error
 go func(){ defer close(done); _,waitErr=execute(ctx,WithLock(m,rt.IO(func(ctx context.Context)(any,error){ close(entered); return execute(ctx,Wait(c)) }))) }()
 await(t,entered)
 go func(){ _,_ = execute(context.Background(),WithLock(m,rt.IO(func(context.Context)(any,error){ close(holder); <-release; return rt.Unit{},nil }))) }()
 await(t,holder); cancel()
 select { case <-done: t.Fatal("cancelled wait returned without reacquiring"); case <-time.After(10*time.Millisecond): }
 close(release); await(t,done)
 if !errors.Is(waitErr,context.Canceled) { t.Fatalf("wait error = %v",waitErr) }
 mustExecute(t,WithLock(m,rt.Pure(rt.Unit{})))
}

func TestStrictJoinWaitsForCleanup(t *testing.T) {
 started:=make(chan struct{}); cleaning:=make(chan struct{}); release:=make(chan struct{})
 task:=mustExecute(t,Start(rt.IO(func(ctx context.Context)(any,error){ close(started); <-ctx.Done(); close(cleaning); return execute(ctx,Uninterruptible(rt.IO(func(context.Context)(any,error){ <-release; return int64(42),nil }))) })))
 await(t,started); mustExecute(t,CancelTask(task)); await(t,cleaning)
 done:=make(chan struct{}); var value any; var err error
 go func(){ defer close(done); value,err=execute(context.Background(),Join(task)) }()
 select { case <-done: t.Fatal("join returned before cleanup"); case <-time.After(150*time.Millisecond): }
 close(release); await(t,done)
 if value!=int64(42) || err!=nil { t.Fatalf("join = %v, %v",value,err) }
}

func TestTaskFailureAndPanic(t *testing.T) {
 for _,test:=range []struct { name string; action rt.IO; kind rt.Kind }{
  {"failure",rt.IO(func(context.Context)(any,error){return nil,failure("failed")}),rt.FailureKind},
  {"panic",rt.IO(func(context.Context)(any,error){panic("panicked")}),rt.Panic},
 } {
  t.Run(test.name,func(t *testing.T){
   task:=mustExecute(t,Start(test.action)); value,err:=execute(context.Background(),Join(task))
   var failure *rt.Failure
   if value!=nil || !errors.As(err,&failure) || failure.Kind!=test.kind {t.Fatalf("join = (%v, %v), want nil and Failure kind %s",value,err,test.kind)}
  })
 }
}

func TestFakeTimerOrderingResetStop(t *testing.T) {
 c:=mustExecute(t,FakeClock(int64(0)))
 early:=mustExecute(t,NewTimer(c,int64(25000))).(*timer)
 late:=mustExecute(t,NewTimer(c,int64(50000))).(*timer)
 mustExecute(t,Advance(c,int64(25000)))
 select { case at:=<-early.channel.values: if at.(*instant).time.UnixNano()!=25000 {t.Fatal("wrong early deadline")}; default:t.Fatal("early timer not ready") }
 select { case <-late.channel.values:t.Fatal("late timer ready early"); default: }
 mustExecute(t,Advance(c,int64(25000)))
 select { case at:=<-late.channel.values: if at.(*instant).time.UnixNano()!=50000 {t.Fatal("wrong late deadline")}; default:t.Fatal("late timer not ready") }
 if mustExecute(t,ResetTimer(early,int64(25000)))!=false {t.Fatal("expired timer reported active")}
 if mustExecute(t,StopTimer(early))!=true {t.Fatal("active timer reported stopped")}
 mustExecute(t,Advance(c,int64(25000)))
 select {case <-early.channel.values:t.Fatal("stopped timer fired");default:}
 mustExecute(t,ResetTimer(early,int64(25000))); mustExecute(t,Advance(c,int64(25000)))
 select {case at:=<-early.channel.values:if at.(*instant).time.UnixNano()!=100000{t.Fatal("reset deadline")};default:t.Fatal("reset timer did not fire")}
}

func TestCloseUnblocksConcurrentSend(t *testing.T) {
 for _,capacity:=range []int64{0,1} {
  ch:=mustExecute(t,NewChannel(capacity))
  if capacity>0 {mustExecute(t,Send(ch,int64(1)))}
  entered:=make(chan struct{}); done:=make(chan struct{}); var value any; var err error
  go func(){defer close(done);close(entered);value,err=execute(context.Background(),Send(ch,int64(2)))}()
  await(t,entered)
  select {case <-done:t.Fatal("send did not block");case <-time.After(10*time.Millisecond):}
  mustExecute(t,Close(ch)); await(t,done)
  var failure *rt.Failure
  if value!=nil || !errors.As(err,&failure) || failure.Kind!=rt.Panic {t.Fatalf("closed send = (%v, %v), want nil and Panic",value,err)}
  value,err=execute(context.Background(),Send(ch,int64(3)))
  if value!=nil || !errors.As(err,&failure) || failure.Kind!=rt.Panic {t.Fatalf("post-close send = (%v,%v)",value,err)}
  if capacity>0 {if buffered:=<-ch.(*channel).values;buffered!=int64(1){t.Fatalf("buffered value = %v",buffered)}}
  if _,ok:=<-ch.(*channel).values;ok{t.Fatal("channel did not close after draining")}
 }
}

func TestChildCannotInheritConditionOwnership(t *testing.T) {
 for _,detached:=range []bool{false,true} {
  m:=mustExecute(t,NewMutex()); c:=mustExecute(t,NewCondition(m))
  var child any; var activeError error
  release:=make(chan struct{}); activeDone:=make(chan struct{})
  mustExecute(t,WithLock(m,rt.IO(func(ctx context.Context)(any,error){
   action:=rt.IO(func(ctx context.Context)(any,error){
    _,activeError=execute(ctx,Signal(c)); close(activeDone)
    <-release
    return execute(ctx,Wait(c))
   })
   var err error
   if detached {child,err=execute(ctx,StartDetached(action))} else {child,err=execute(ctx,Start(action))}
   if err!=nil{return nil,err}; await(t,activeDone); return rt.Unit{},nil
  })))
  var activeFailure *rt.Failure
  if !errors.As(activeError,&activeFailure) || activeFailure.Kind!=rt.FailureKind {t.Errorf("child inherited active parent ownership: %v",activeError)}
  // An unrelated owner now holds the mutex; the stale child must not release it.
  mustExecute(t,WithLock(m,rt.IO(func(ctx context.Context)(any,error){
   close(release)
   value,err:=execute(ctx,Join(child)); var failure *rt.Failure
   if value!=nil || !errors.As(err,&failure) || failure.Kind!=rt.FailureKind {t.Errorf("stale child wait = (%v,%v)",value,err)}
   select {case <-m.(*mutex).token:t.Fatal("child released another owner's mutex");default:}
   return rt.Unit{},nil
  })))
 }
}

func TestCleanupAndDetachedPreserveContextValues(t *testing.T) {
 type valueKey struct{}
 parent,cancel:=context.WithCancel(context.WithValue(context.Background(),valueKey{},"retained"))
 defer cancel()
 gate:=make(chan struct{})
 task,err:=execute(parent,StartDetached(rt.IO(func(ctx context.Context)(any,error){
  <-gate
  if ctx.Err()!=nil {return nil,ctx.Err()}
  return ctx.Value(valueKey{}),nil
 })))
 if err!=nil {t.Fatal(err)}
 cancel();close(gate)
 value,err:=execute(context.Background(),Join(task))
 if value!="retained" || err!=nil {t.Fatalf("detached context = (%v,%v)",value,err)}
 value,err=execute(parent,Uninterruptible(rt.IO(func(ctx context.Context)(any,error){
  if ctx.Err()!=nil{return nil,ctx.Err()}
  return ctx.Value(valueKey{}),nil
 })))
 if value!="retained" || err!=nil {t.Fatalf("cleanup context = (%v,%v)",value,err)}
}

func TestSameRefIdentity(t *testing.T) {
 left:=mustExecute(t,NewRef(int64(7)))
 alias:=left
 right:=mustExecute(t,NewRef(int64(7)))
 if SameRef(left,alias)!=true || SameRef(left,right)!=false {t.Fatal("reference identity confused with equal contents")}
 mustExecute(t,WriteRef(left,int64(42)))
 if SameRef(left,alias)!=true || SameRef(left,right)!=false {t.Fatal("mutation changed reference identity")}
 mustExecute(t,WriteRef(right,int64(42)))
 if SameRef(left,right)!=false {t.Fatal("equal mutated contents merged identities")}
}
