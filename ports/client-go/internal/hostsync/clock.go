package hostsync

import (
 "context"
 "sync"
 "time"
 "gofx/rt"
)

type clock struct { mu sync.Mutex; fake bool; now time.Time; timers map[*timer]struct{} }
type instant struct { time time.Time }
// TimeValue and InstantValue preserve the full Go value, including monotonic time.
// They are host adapters, not source-level serialization or expiry policy.
func TimeValue(handle any) time.Time { return handle.(*instant).time }
func InstantValue(value time.Time) any { return &instant{time:value} }
type timer struct { clock *clock; channel *channel; due time.Time; active bool; generation uint64; real *time.Timer }
func RealClock() any { return effect(func(context.Context)(any,error){ return &clock{},nil }) }
func FakeClock(start any) any { return effect(func(context.Context)(any,error){ return &clock{fake:true,now:time.Unix(0,start.(int64)),timers:make(map[*timer]struct{})},nil }) }
func (c *clock) current() time.Time { if c.fake { return c.now }; return time.Now() }
func Now(handle any) any { return effect(func(context.Context)(any,error){ c:=handle.(*clock); c.mu.Lock(); defer c.mu.Unlock(); return &instant{c.current()},nil }) }
func UnixNanos(handle any) any { return handle.(*instant).time.UnixNano() }
func Elapsed(start,end any) any { return int64(end.(*instant).time.Sub(start.(*instant).time)) }
func Add(handle,nanos any) any { return &instant{handle.(*instant).time.Add(time.Duration(nanos.(int64)))} }
func CompareInstant(a,b any) any { return int64(a.(*instant).time.Compare(b.(*instant).time)) }
func (t *timer) fire(at time.Time) {
 t.active=false
 if t.clock.fake { delete(t.clock.timers,t) }
 t.channel.mu.Lock(); defer t.channel.mu.Unlock()
 if !t.channel.closed { select { case t.channel.values<-&instant{at}: default: } }
}
func (t *timer) schedule(nanos int64) {
 c:=t.clock; t.generation++; generation:=t.generation
 t.active=true; t.due=c.current().Add(time.Duration(nanos))
 if c.fake {
  c.timers[t]=struct{}{}
  if !t.due.After(c.now) { t.fire(c.now) }
 } else {
  t.real=time.AfterFunc(time.Duration(nanos),func(){ c.mu.Lock(); defer c.mu.Unlock(); if t.active && t.generation==generation { t.fire(t.due) } })
 }
}
func NewTimer(handle,nanos any) any { return effect(func(context.Context)(any,error){
 c:=handle.(*clock); c.mu.Lock(); defer c.mu.Unlock()
 t:=&timer{clock:c,channel:makeChannel(1)}; t.schedule(nanos.(int64)); return t,nil
}) }
func TimerChannel(handle any) any { return effect(func(context.Context)(any,error){ return handle.(*timer).channel,nil }) }
func StopTimer(handle any) any { return effect(func(context.Context)(any,error){
 t:=handle.(*timer); c:=t.clock; c.mu.Lock(); defer c.mu.Unlock()
 active:=t.active; t.active=false; t.generation++
 if t.real!=nil { t.real.Stop() }; if c.fake { delete(c.timers,t) }
 // Like Go 1.23 timers, no stale notification survives Stop or Reset.
 select { case <-t.channel.values: default: }
 return active,nil
}) }
func ResetTimer(handle,nanos any) any { return effect(func(context.Context)(any,error){
 t:=handle.(*timer); c:=t.clock; c.mu.Lock(); defer c.mu.Unlock()
 active:=t.active; if t.real!=nil { t.real.Stop() }
 select { case <-t.channel.values: default: }
 t.schedule(nanos.(int64)); return active,nil
}) }
func Advance(handle,nanos any) any { return effect(func(context.Context)(any,error){
 c:=handle.(*clock); c.mu.Lock(); defer c.mu.Unlock()
 if !c.fake { return nil,failure("advance: requires fake clock") }
 n:=nanos.(int64); if n<0 { return nil,failure("advance: negative duration") }
 c.now=c.now.Add(time.Duration(n))
 for t:=range c.timers { if !t.due.After(c.now) { t.fire(t.due) } }
 return rt.Unit{},nil
}) }
