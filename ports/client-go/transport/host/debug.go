// Copyright 2015 The Kubernetes Authors. Licensed under Apache-2.0.
package host

import (
 "context"
 "crypto/tls"
 "net/http/httptrace"
 "sync"
 "time"
 "gofx/rt"
 "k8s.io/klog/v2"
)
func LogVerbosity()any{return effect(func(ctx context.Context)(any,error){logger:=klog.FromContext(ctx);for n:=9;n>=6;n--{if logger.V(n).Enabled(){return int64(n),nil}};return int64(0),nil})}
func LogFields(message,fields any)any{return effect(func(ctx context.Context)(any,error){values:=[]any{};for rest:=fields;;{h,t,ok:=rt.Shapes.Uncons(rest);if !ok{break};k,v:=tuple(h);values=append(values,k,v);rest=t};klog.FromContext(ctx).Info(message.(string),values...);return rt.Unit{},nil})}
func LogEvent(name,value any)any{return effect(func(ctx context.Context)(any,error){klog.FromContext(ctx).V(4).Info(name.(string),"detail",value);return rt.Unit{},nil})}
func ResponseHeaders(v any)any{return effect(func(ctx context.Context)(any,error){h:=response(ctx,v).Header;xs:=make([]any,0,len(h));for k,vs:=range h{xs=append(xs,rt.Tuple2Of(k,rt.ListFromStrings(vs)))};return listOf(xs),nil})}
func ResponseStatusText(v any)any{return effect(func(ctx context.Context)(any,error){return response(ctx,v).Status,nil})}
type trace struct{mu sync.Mutex;starts map[string]time.Time;durations map[string]time.Duration;reused bool}
func(t *trace)start(key string){t.mu.Lock();defer t.mu.Unlock();t.starts[key]=time.Now()}
func(t *trace)end(key string){t.mu.Lock();defer t.mu.Unlock();if start,ok:=t.starts[key];ok{t.durations[key]=time.Since(start)}}
func TraceRequest(v any)any{return effect(func(ctx context.Context)(any,error){t:=&trace{starts:make(map[string]time.Time),durations:make(map[string]time.Duration)};hooks:=&httptrace.ClientTrace{
 DNSStart:func(httptrace.DNSStartInfo){t.start("dnsLookup")},DNSDone:func(httptrace.DNSDoneInfo){t.end("dnsLookup")},
 ConnectStart:func(string,string){t.start("dial")},ConnectDone:func(string,string,error){t.end("dial")},
 TLSHandshakeStart:func(){t.start("tlsHandshake")},TLSHandshakeDone:func(tls.ConnectionState,error){t.end("tlsHandshake")},
 GetConn:func(string){t.start("getConnection")},GotConn:func(info httptrace.GotConnInfo){t.end("getConnection");t.mu.Lock();t.reused=info.Reused;t.mu.Unlock()},
 WroteRequest:func(httptrace.WroteRequestInfo){t.start("serverProcessing")},GotFirstResponseByte:func(){t.end("serverProcessing")},
 };r:=request(ctx,v);cloned:=r.WithContext(httptrace.WithClientTrace(r.Context(),hooks));out,_:=rt.OptionValue(external(cloned));return rt.Tuple2Of(out,t),nil})}
func TraceValues(v any)any{return effect(func(context.Context)(any,error){t:=v.(*trace);t.mu.Lock();defer t.mu.Unlock();xs:=[]any{};for key,d:=range t.durations{xs=append(xs,rt.Tuple2Of(key,int64(d)))};if t.reused{xs=append(xs,rt.Tuple2Of("connectionReused",int64(1)))};return listOf(xs),nil})}
