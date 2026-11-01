// Copyright 2026 The go4t Authors. Licensed under Apache-2.0.
package host

import (
 "context"
 "fmt"
 "net/http"
 "reflect"
 "strings"
 "sync"
 "go4t/rt"
 "github.com/go-logr/logr"
 "github.com/reoring/gofx/ports/client-go/host/deps"
 "k8s.io/klog/v2"
)

type logRecord struct { level int; name,message string; fields []any }
type logCapture struct { sync.Mutex; records []logRecord; max int }
type captureSink struct { capture *logCapture; name string; fields []any }
func (s captureSink) Init(logr.RuntimeInfo) {}
func (s captureSink) Enabled(level int) bool { return level<=s.capture.max }
func (s captureSink) Info(level int,message string,fields ...any) { s.capture.Lock(); defer s.capture.Unlock(); s.capture.records=append(s.capture.records,logRecord{level,s.name,message,append(append([]any(nil),s.fields...),fields...)}) }
func (s captureSink) Error(err error,message string,fields ...any) { s.Info(0,message,append(fields,"err",err)...)}
func (s captureSink) WithName(name string) logr.LogSink { if s.name!="" {s.name+="."};s.name+=name;return s }
func (s captureSink) WithValues(fields ...any) logr.LogSink { s.fields=append(append([]any(nil),s.fields...),fields...);return s }
func OpenLogCapture(max any) any { return rt.IO(func(context.Context)(any,error){return &logCapture{max:int(max.(int64))},nil}) }
func CaptureLogger(v any) any { return rt.IO(func(context.Context)(any,error){logger:=logr.New(captureSink{capture:v.(*logCapture)});return hostdeps.From(reflect.ValueOf(logger)),nil}) }
func LogCount(v,level,name,message any) any { return rt.IO(func(context.Context)(any,error){c:=v.(*logCapture);c.Lock();defer c.Unlock();n:=int64(0);for _,r:=range c.records {if r.level==int(level.(int64))&&r.name==name.(string)&&r.message==message.(string){n++}};return n,nil}) }
func LogField(v,message,key any) any {return rt.IO(func(context.Context)(any,error){c:=v.(*logCapture);c.Lock();defer c.Unlock();for _,r:=range c.records {if r.message!=message.(string){continue};for i:=0;i+1<len(r.fields);i+=2 {if r.fields[i]==key.(string){if b,ok:=r.fields[i+1].([]byte);ok{return string(b),nil};return fmt.Sprint(r.fields[i+1]),nil}}};return "",nil})}
func LogNames(v,prefix any) any {return rt.IO(func(context.Context)(any,error){c:=v.(*logCapture);c.Lock();defer c.Unlock();for _,r:=range c.records {if !strings.HasPrefix(r.name,prefix.(string)){return false,nil}};return len(c.records)>0,nil})}
func RequestContextLogger(v any) any { return rt.IO(func(ctx context.Context)(any,error){
 request:=hostdeps.To(reflect.TypeFor[*http.Request](),rt.SomeOf(v),ctx).Interface().(*http.Request)
 return hostdeps.From(reflect.ValueOf(klog.FromContext(request.Context()))),nil
}) }
