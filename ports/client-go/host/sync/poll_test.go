package hostsync

import (
 "context"
 "errors"
 "runtime"
 "testing"
 "time"
 "gofx/rt"
)

func poll(t *testing.T,c any)(bool,testOption){t.Helper();value:=mustExecute(t,TryReceiveRaw(c)).([2]any);return value[0].(bool),value[1].(testOption)}
func TestTryReceiveUnavailableBufferedAndClosed(t *testing.T){
 c:=mustExecute(t,NewChannel(int64(1)))
 if ready,_:=poll(t,c);ready{t.Fatal("unavailable channel was ready")}
 mustExecute(t,Send(c,int64(7)))
 ready,value:=poll(t,c);if !ready||!value.some||value.value!=int64(7){t.Fatalf("buffered poll = %v,%v",ready,value)}
 if ready,_:=poll(t,c);ready{t.Fatal("drained open channel was ready")}
 mustExecute(t,Send(c,int64(8)));mustExecute(t,Close(c))
 ready,value=poll(t,c);if !ready||!value.some||value.value!=int64(8){t.Fatalf("closed buffer poll = %v,%v",ready,value)}
 ready,value=poll(t,c);if !ready||value.some{t.Fatalf("closed drained poll = %v,%v",ready,value)}
}
func TestTryReceiveRendezvousSender(t *testing.T){
 c:=mustExecute(t,NewChannel(int64(0)))
 ctx,cancel:=context.WithCancel(context.Background());defer cancel()
 sent:=make(chan struct{});var err error
 go func(){defer close(sent);_,err=execute(ctx,Send(c,int64(42)))}()
 // The goroutine scheduler may not have reached the send yet. Each probe is
 // nonblocking; the test alone retries to establish sender readiness.
 deadline:=time.Now().Add(time.Second)
 for {
  ready,value:=poll(t,c)
  if ready {if !value.some||value.value!=int64(42){t.Fatalf("rendezvous poll = %v",value)};break}
  if time.Now().After(deadline){t.Fatal("rendezvous sender never ready")}
  runtime.Gosched()
 }
 await(t,sent);if err!=nil{t.Fatal(err)}
 if ready,_:=poll(t,c);ready{t.Fatal("rendezvous value consumed twice")}
}

func TestTrySendFullRendezvousAndClosed(t *testing.T){
 c:=mustExecute(t,NewChannel(int64(1)))
 if mustExecute(t,TrySend(c,int64(7)))!=true{t.Fatal("available buffer rejected value")}
 if mustExecute(t,TrySend(c,int64(8)))!=false{t.Fatal("full buffer accepted value")}
 if value:=<-c.(*channel).values;value!=int64(7){t.Fatalf("full trySend replaced buffered value: %v",value)}
 rendezvous:=mustExecute(t,NewChannel(int64(0)))
 if mustExecute(t,TrySend(rendezvous,int64(7)))!=false{t.Fatal("rendezvous without receiver succeeded")}
 received:=make(chan any,1)
 go func(){received<- <-rendezvous.(*channel).values}()
 deadline:=time.Now().Add(time.Second)
 for mustExecute(t,TrySend(rendezvous,int64(42)))!=true{
  if time.Now().After(deadline){t.Fatal("rendezvous receiver never ready")}
  runtime.Gosched()
 }
 select{case value:=<-received:if value!=int64(42){t.Fatal("wrong rendezvous payload")};case <-time.After(time.Second):t.Fatal("rendezvous receiver stuck")}
 mustExecute(t,Close(c))
 value,err:=execute(context.Background(),TrySend(c,int64(9)));var failure *rt.Failure
 if value!=nil||!errors.As(err,&failure)||failure.Kind!=rt.Panic{t.Fatalf("closed trySend = %v,%v",value,err)}
}

func TestTrySendCloseRace(t *testing.T){
 for i:=range 64{
  c:=mustExecute(t,NewChannel(int64(i%2)))
  gate:=make(chan struct{});sent:=make(chan struct{});closed:=make(chan struct{})
  var value any;var sendErr,closeErr error
  go func(){defer close(sent);<-gate;value,sendErr=execute(context.Background(),TrySend(c,int64(7)))}()
  go func(){defer close(closed);<-gate;_,closeErr=execute(context.Background(),Close(c))}()
  close(gate);await(t,sent);await(t,closed)
  if closeErr!=nil{t.Fatal(closeErr)}
  if sendErr!=nil{var failure *rt.Failure;if value!=nil||!errors.As(sendErr,&failure)||failure.Kind!=rt.Panic{t.Fatalf("race failure = %v,%v",value,sendErr)}}else{
   committed,ok:=value.(bool);if !ok{t.Fatalf("race success returned %T",value)}
   buffered,open:=<-c.(*channel).values
   if committed{if !open||buffered!=int64(7){t.Fatal("successful send missing after close")}}else if open{t.Fatal("failed send committed value")}
  }
 }
}
