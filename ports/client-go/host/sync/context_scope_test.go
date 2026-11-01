package hostsync

import (
 "context"
 "errors"
 "testing"
 "time"
 "go4t/rt"
)

func TestDerivedContextLifetimeAndTimeout(t *testing.T){
 parent:=mustExecute(t,CaptureContext())
 pair:=mustExecute(t,NewContext(parent,testOption{})).([2]any)
 child,cancel:=pair[0].(*capturedContext),pair[1]
 if mustExecute(t,ContextCancelled(child))!=false{t.Fatal("no-timeout child cancelled")}
 mustExecute(t,CancelContext(cancel));mustExecute(t,CancelContext(cancel))
 if mustExecute(t,ContextCancelled(child))!=true{t.Fatal("cancel handle did not cancel child")}
 if mustExecute(t,ContextCancelled(parent))!=false{t.Fatal("child cancellation affected parent")}
 for _,nanos:=range []int64{-1,0,1000000}{
  pair=mustExecute(t,NewContext(parent,testOption{nanos,true})).([2]any)
  child=pair[0].(*capturedContext)
  select{case <-child.context.Done():case <-time.After(time.Second):t.Fatal("derived timeout never expired")}
  if !errors.Is(child.context.Err(),context.DeadlineExceeded){t.Fatalf("timeout kind %v",child.context.Err())}
  mustExecute(t,CancelContext(pair[1]))
 }
 original,stop:=context.WithCancel(context.Background())
 pair=mustExecute(t,NewContext(&capturedContext{original},testOption{})).([2]any)
 stop();child=pair[0].(*capturedContext)
 select{case <-child.context.Done():case <-time.After(time.Second):t.Fatal("parent cancellation not inherited")}
 if !errors.Is(child.context.Err(),context.Canceled){t.Fatal(child.context.Err())}
 mustExecute(t,CancelContext(pair[1]))
}
func TestDerivedContextPreservesValuesNotTaskOwnership(t *testing.T){
 type valueKey struct{}
 base:=context.WithValue(context.Background(),valueKey{},"kept")
 m:=mustExecute(t,NewMutex());c:=mustExecute(t,NewCondition(m))
 _,err:=execute(base,WithLock(m,rt.IO(func(ctx context.Context)(any,error){
  parent,_:=execute(ctx,CaptureContext())
  pair,_:=execute(ctx,NewContext(parent,testOption{}));values:=pair.([2]any)
  defer func(){_,_ = execute(context.Background(),CancelContext(values[1]))}()
  child:=values[0]
  value,err:=execute(ctx,WithContext(child,rt.IO(func(restored context.Context)(any,error){return restored.Value(valueKey{}),nil})))
  if value!="kept"||err!=nil{t.Errorf("context value = %v,%v",value,err)}
  task,err:=execute(ctx,Start(WithContext(child,Signal(c))));if err!=nil{return nil,err}
  _,err=execute(ctx,Join(task));var failure *rt.Failure
  if !errors.As(err,&failure)||failure.Kind!=rt.FailureKind{t.Errorf("derived context transferred ownership: %v",err)}
  return rt.Unit{},nil
 })))
 if err!=nil{t.Fatal(err)}
}
