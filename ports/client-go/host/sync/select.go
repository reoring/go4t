package hostsync

import (
 "context"
 "reflect"
 "go4t/rt"
)

type selectCase struct { channel *channel; send bool; value any; result any; mapper func(any) any }
func ReceiveCase(handle,mapper any) any { return &selectCase{channel:handle.(*channel),mapper:mapper.(func(any) any)} }
func SendCase(handle,value,result any) any { return &selectCase{channel:handle.(*channel),send:true,value:value,result:result} }

func SelectCase(choices any) any { return effect(func(ctx context.Context)(any,error){
 var descriptions []*selectCase
 var sending []*channel
 seen:=make(map[*channel]bool)
 for xs:=choices;; { h,t,ok:=rt.Shapes.Uncons(xs); if !ok {break}; c:=h.(*selectCase); descriptions=append(descriptions,c); if c.send && !seen[c.channel] {seen[c.channel]=true;sending=append(sending,c.channel)};xs=t }
 // No native close can begin while this batch acquires all read barriers.
 // Each channel is acquired once even if several send cases refer to it.
 sendRegistration.Lock()
 for _,c:=range sending {c.sends.RLock()}
 sendRegistration.Unlock()
 locked:=true
 unlock:=func(){if locked {for _,c:=range sending{c.sends.RUnlock()};locked=false}}
 defer unlock()
 cases:=make([]reflect.SelectCase,1,1+len(descriptions)+len(sending))
 cases[0]=reflect.SelectCase{Dir:reflect.SelectRecv,Chan:reflect.ValueOf(ctx.Done())}
 for _,c:=range descriptions {
  if !c.send {cases=append(cases,reflect.SelectCase{Dir:reflect.SelectRecv,Chan:reflect.ValueOf(c.channel.values)});continue}
  // A completed close disables native send; its readiness is represented by
  // the closing notification below. A concurrent close cannot pass our barrier.
  closed:=false;select{case <-c.channel.closing:closed=true;default:}
  if closed {cases=append(cases,reflect.SelectCase{Dir:reflect.SelectSend});continue}
  value:=reflect.ValueOf(c.value);if !value.IsValid(){value=reflect.Zero(reflect.TypeFor[any]())}
  cases=append(cases,reflect.SelectCase{Dir:reflect.SelectSend,Chan:reflect.ValueOf(c.channel.values),Send:value})
 }
 for _,c:=range sending {cases=append(cases,reflect.SelectCase{Dir:reflect.SelectRecv,Chan:reflect.ValueOf(c.closing)})}
 index,value,ok:=reflect.Select(cases)
 // Mapping is user code and may itself close a channel: leave barriers first.
 unlock()
 if index==0{return nil,ctx.Err()}
 if index>len(descriptions){return nil,&rt.Failure{Kind:rt.Panic,Message:"send on closed channel"}}
 winner:=descriptions[index-1]
 if winner.send{return winner.result,nil}
 option:=rt.NoneOf();if ok{option=rt.SomeOf(value.Interface())}
 return winner.mapper(option),nil
}) }
