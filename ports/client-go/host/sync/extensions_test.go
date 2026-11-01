package hostsync

import (
 "context"
 "errors"
 "sync/atomic"
 "testing"
 "time"
 "go4t/rt"
)

type testOption struct { value any; some bool }
func init(){
 rt.Shapes.Uncons=func(value any)(any,any,bool){xs:=value.([]any);if len(xs)==0{return nil,nil,false};return xs[0],xs[1:],true}
 rt.Shapes.Some=func(value any)any{return testOption{value,true}}
 rt.Shapes.None=func()any{return testOption{}}
 rt.Shapes.Tuple2=func(a,b any)any{return [2]any{a,b}}
 rt.Shapes.OptionValue=func(value any)(any,bool){option:=value.(testOption);return option.value,option.some}
}
func TestMixedSelectionOnlyMapsWinner(t *testing.T){
 a:=mustExecute(t,NewChannel(int64(1)));b:=mustExecute(t,NewChannel(int64(1)))
 mustExecute(t,Send(a,int64(7)))
 var mapped atomic.Int32
 win:=mustExecute(t,SelectCase([]any{ReceiveCase(a,func(value any)any{mapped.Add(1);return value.(testOption).value}),SendCase(b,int64(9),int64(9))}))
 if mapped.Load()>1{t.Fatal("mapped loser")}
 if win==int64(7){if mapped.Load()!=1{t.Fatal("receive mapper missing")};select{case <-b.(*channel).values:t.Fatal("losing send committed");default:}}else{if win!=int64(9)||mapped.Load()!=0{t.Fatal("wrong send result")};if <-a.(*channel).values!=int64(7){t.Fatal("losing receive consumed")}}
}
func TestDuplicateAndCrossChannelSelectClose(t *testing.T){
 for range 25 {
  a:=mustExecute(t,NewChannel(int64(0)));b:=mustExecute(t,NewChannel(int64(0)))
  done:=make(chan struct{});other:=make(chan struct{});var err1,err2 error
  ctx,cancel:=context.WithTimeout(context.Background(),time.Second)
  go func(){defer close(done);_,err1=execute(ctx,SelectCase([]any{SendCase(a,1,1),SendCase(a,2,2),SendCase(b,3,3)}))}()
  go func(){defer close(other);_,err2=execute(ctx,SelectCase([]any{SendCase(b,4,4),SendCase(a,5,5)}))}()
  closedA:=make(chan struct{});closedB:=make(chan struct{})
  go func(){defer close(closedA);mustExecute(t,Close(a))}()
  go func(){defer close(closedB);mustExecute(t,Close(b))}()
  await(t,done);await(t,other);await(t,closedA);await(t,closedB);cancel()
  for _,err:=range []error{err1,err2}{var failure *rt.Failure;if !errors.As(err,&failure)||failure.Kind!=rt.Panic{t.Fatalf("select close result %v",err)}}
 }
}
func TestSelectCancellation(t *testing.T){
 c:=mustExecute(t,NewChannel(int64(0)));ctx,cancel:=context.WithCancel(context.Background());done:=make(chan struct{});var err error
 go func(){defer close(done);_,err=execute(ctx,SelectCase([]any{SendCase(c,1,1),ReceiveCase(c,func(any)any{return 2})}))}()
 cancel();await(t,done);if !errors.Is(err,context.Canceled){t.Fatalf("select cancel: %v",err)}
 mustExecute(t,Close(c))
}
func TestBracketCancellationAfterAcquire(t *testing.T){
 ctx,cancel:=context.WithCancel(context.Background());defer cancel();used,released:=false,0
 value,err:=execute(ctx,Bracket(rt.IO(func(context.Context)(any,error){cancel();return int64(42),nil}),func(any)any{used=true;return rt.Pure(1)},func(resource any)any{return rt.IO(func(clean context.Context)(any,error){released++;if clean.Err()!=nil||resource!=int64(42){t.Error("release context/resource")};return rt.Unit{},nil})}))
 if value!=nil||!errors.Is(err,context.Canceled)||used||released!=1{t.Fatalf("bracket = %v,%v used=%v releases=%d",value,err,used,released)}
}
func TestFinalizerFailurePrecedence(t *testing.T){
 for _,panicUse:=range []bool{false,true}{
  calls:=0
  _,err:=execute(context.Background(),Bracket(rt.Pure(1),func(any)any{if panicUse{panic("use panic")};return rt.Fail("use error")},func(any)any{calls++;panic("release panic")}))
  var f *rt.Failure;want:=rt.FailureKind;if panicUse{want=rt.Panic}
  if !errors.As(err,&f)||f.Kind!=want||f.Message=="release panic"||calls!=1{t.Fatalf("precedence %v calls=%d",err,calls)}
 }
 value,err:=execute(context.Background(),Ensuring(rt.Pure(1),rt.Fail("cleanup error")))
 if value!=nil||err==nil||err.Error()!="cleanup error"{t.Fatalf("cleanup failure = %v,%v",value,err)}
 cleaned:=false
 _,err=execute(context.Background(),Ensuring(rt.IO(func(context.Context)(any,error){panic("action panic")}),rt.IO(func(ctx context.Context)(any,error){cleaned=true;return rt.Unit{},ctx.Err()})))
 var f *rt.Failure;if !cleaned||!errors.As(err,&f)||f.Kind!=rt.Panic{t.Fatalf("panic cleanup %v cleaned=%v",err,cleaned)}
}
func TestCapturedContextObservationsAndOwnership(t *testing.T){
 type key struct{}
 original,cancel:=context.WithDeadline(context.WithValue(context.Background(),key{},"kept"),time.Now().Add(-time.Second));defer cancel()
 captured:=&capturedContext{original}
 if mustExecute(t,ContextCancelled(captured))!=true{t.Fatal("original cancellation lost")}
 remaining:=mustExecute(t,ContextRemaining(captured)).(testOption);if !remaining.some||remaining.value.(int64)>=0{t.Fatal("expired deadline not preserved")}
 if mustExecute(t,ContextRemaining(&capturedContext{context.Background()})).(testOption).some{t.Fatal("invented deadline")}
 _,err:=execute(context.Background(),WithContext(captured,rt.Pure(1)));if !errors.Is(err,context.DeadlineExceeded){t.Fatalf("restored context: %v",err)}
 live:=mustExecute(t,CaptureContext()).(*capturedContext)
 if mustExecute(t,ContextCancelled(live))!=false{t.Fatal("live context cancelled")}
 m:=mustExecute(t,NewMutex());c:=mustExecute(t,NewCondition(m))
 mustExecute(t,WithLock(m,rt.IO(func(ctx context.Context)(any,error){
  saved,_:=execute(ctx,CaptureContext())
  task,_:=execute(ctx,Start(rt.IO(func(child context.Context)(any,error){return execute(child,WithContext(saved,Signal(c)))})))
  _,err:=execute(ctx,Join(task));var f *rt.Failure;if !errors.As(err,&f)||f.Kind!=rt.FailureKind{t.Errorf("captured ownership transferred: %v",err)}
  return rt.Unit{},nil
 })))
}
