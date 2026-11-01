// Copyright 2026 The Kubernetes Authors.
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy at http://www.apache.org/licenses/LICENSE-2.0
// Unless required by applicable law or agreed to in writing, software distributed
// under the License is distributed on an "AS IS" BASIS, WITHOUT WARRANTIES OR
// CONDITIONS OF ANY KIND, either express or implied.
// Generic OS, crypto, JSON and dependency-value operations. No provider policy.
package host

import (
 "context"
 "crypto/tls"
 "crypto/x509"
 "encoding/base64"
 "encoding/json"
 "errors"
 "fmt"
 "io/fs"
 "net"
 "net/url"
 "os"
 "os/exec"
 "path/filepath"
 "reflect"
 "strconv"
 "time"
 "gofx/rt"
 "github.com/reoring/gofx/ports/client-go/host/deps"
 "golang.org/x/term"
 meta "k8s.io/apimachinery/pkg/apis/meta/v1"
 kruntime "k8s.io/apimachinery/pkg/runtime"
)
func value[T any](ctx context.Context,v any) T {converted:=hostdeps.To(reflect.TypeFor[T](),v,ctx).Interface();if converted==nil {var zero T;return zero};return converted.(T)}
func foreign[T any](v T) any {return hostdeps.From(reflect.ValueOf(&v).Elem())}
func identity(v any) any {x,ok:=rt.OptionValue(hostdeps.From(reflect.ValueOf(&v).Elem()));if !ok {panic("nil external identity")};return x}
func stringsFrom(v []string) any {out:=rt.Shapes.Nil();for i:=len(v)-1;i>=0;i-- {out=rt.Shapes.Cons(v[i],out)};return out}
func ErrorText(text string) any {return hostdeps.ErrorFromGo(errors.New(text))}
func WrapError(text string,cause any) any {return hostdeps.ErrorFromGo(fmt.Errorf("%s: %w",text,hostdeps.ErrorToGo(cause)))}
func Quote(text string) string {return strconv.Quote(text)}
func CleanPath(text string) string {return filepath.Clean(text)}
func BasePath(text string) string {return filepath.Base(text)}
func ObjectEqual(a,b any) bool {return reflect.DeepEqual(value[kruntime.Object](context.Background(),a),value[kruntime.Object](context.Background(),b))}
func metadataTime(v any) meta.Time {return value[meta.Time](context.Background(),v)}
func TimeIsZero(v any) bool {t:=metadataTime(v);return t.IsZero()}
func TimeAfter(now int64,v any) bool {return time.Unix(0,now).After(metadataTime(v).Time)}
func TimeBefore(a,b any) bool {return metadataTime(a).Time.Before(metadataTime(b).Time)}
func TimeSince(now int64,v any) int64 {return int64(time.Unix(0,now).Sub(metadataTime(v).Time))}
// Commands are external os/exec identities. Preparing resolves PATH exactly once.
func Prepare(command string,args []string) rt.IO {return func(context.Context)(any,error){return exec.Command(command,args...),nil}}
func CommandPath(command any) rt.IO {return func(context.Context)(any,error){return command.(*exec.Cmd).Path,nil}}
func CommandError(command any) rt.IO {return func(context.Context)(any,error){return hostdeps.ErrorOptionFromGo(command.(*exec.Cmd).Err),nil}}
func SetCommandPath(command any,path string) rt.IO {return func(context.Context)(any,error){command.(*exec.Cmd).Path=path;return rt.Unit{},nil}}
func LookPath(command string) rt.IO {return func(context.Context)(any,error){path,err:=exec.LookPath(command);return rt.Tuple2Of(path,hostdeps.ErrorOptionFromGo(err)),nil}}
func Run(command any,env []string,interactive bool) rt.IO {return func(ctx context.Context)(any,error){
 c:=command.(*exec.Cmd)
 // CommandContext supplies cancellation; retain the prepared path/error and argv.
 actual:=exec.CommandContext(ctx,c.Path);actual.Path=c.Path;actual.Args=c.Args;actual.Err=c.Err;actual.Env=env;actual.Stderr=os.Stderr;actual.WaitDelay=time.Second
 if interactive {actual.Stdin=os.Stdin}
 out,err:=actual.Output();if ctx.Err()!=nil {return nil,ctx.Err()}
 category:="";code:=int64(0)
 if err!=nil {var ee *exec.ExitError;var e *exec.Error;var pe *fs.PathError;switch {case errors.As(err,&ee):category="exit";code=int64(ee.ExitCode());case errors.As(err,&e):category="executable";case errors.As(err,&pe):category="path";default:category="other"}}
 return rt.Tuple3Of(string(out),category,rt.Tuple2Of(code,hostdeps.ErrorOptionFromGo(err))),nil
}}
func Environ() rt.IO {return func(context.Context)(any,error){return stringsFrom(os.Environ()),nil}}
func Terminal() rt.IO {return func(context.Context)(any,error){return term.IsTerminal(int(os.Stdin.Fd())),nil}}
func ParseKeyPair(cert,key string) rt.IO {return func(context.Context)(any,error){c,e:=tls.X509KeyPair([]byte(cert),[]byte(key));if e!=nil{return nil,e};return identity(&c),nil}}
func ParseLeaf(v any) rt.IO {return func(ctx context.Context)(any,error){c:=value[*tls.Certificate](ctx,v);var der []byte;if len(c.Certificate)>0 {der=c.Certificate[0]};leaf,e:=x509.ParseCertificate(der);if e!=nil{return nil,e};c.Leaf=leaf;return v,nil}}
func CertificateDates(v any) rt.IO {return func(ctx context.Context)(any,error){c:=value[*tls.Certificate](ctx,v);if c.Leaf==nil{return rt.Tuple2Of(foreign(meta.Time{}),foreign(meta.Time{})),nil};return rt.Tuple2Of(foreign(meta.NewTime(c.Leaf.NotBefore)),foreign(meta.NewTime(c.Leaf.NotAfter))),nil}}
func Dial(network,address string,timeoutNanos,keepAliveNanos int64) rt.IO {return func(ctx context.Context)(any,error){d:=net.Dialer{Timeout:time.Duration(timeoutNanos),KeepAlive:time.Duration(keepAliveNanos)};c,e:=d.DialContext(ctx,network,address);if e!=nil{return nil,e};return identity(c),nil}}
func DecodeBase64(text string,url bool) rt.IO {return func(context.Context)(any,error){enc:=base64.StdEncoding;if url {enc=base64.RawURLEncoding};b,e:=enc.DecodeString(text);return string(b),e}}
func SplitObject(text string) rt.IO {return func(context.Context)(any,error){var fields map[string]json.RawMessage;if e:=json.Unmarshal([]byte(text),&fields);e!=nil{return nil,e};out:=rt.Shapes.Nil();for k,v:=range fields {out=rt.Shapes.Cons(rt.Tuple2Of(k,string(v)),out)};return out,nil}}
func JSONString(text string) rt.IO {return func(context.Context)(any,error){var s string;e:=json.Unmarshal([]byte(text),&s);return s,e}}
func JSONSeconds(text string) rt.IO {return func(context.Context)(any,error){var n json.Number;if e:=json.Unmarshal([]byte(text),&n);e!=nil{return nil,e};i,e:=n.Int64();if e==nil{return i,nil};f,e:=n.Float64();return int64(f),e}}
func FormEncode(pairs any) string {v:=url.Values{};for {h,t,ok:=rt.Shapes.Uncons(pairs);if !ok {break};k,x:=rt.Shapes.Untuple2(h);v.Add(k.(string),x.(string));pairs=t};return v.Encode()}
