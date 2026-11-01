package hostsync

import (
 "context"
 "runtime"
 "testing"
 "time"
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
