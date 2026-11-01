// Copyright The Kubernetes Authors. SPDX-License-Identifier: Apache-2.0
// Generic codecs, filesystem effects and structural error transport only.
package host
import (
 "context"
 "encoding/json"
 "errors"
 "io"
 "mime"
 "os"
 "path/filepath"
 "reflect"
 "syscall"
 "gofx/rt"
 deps "github.com/reoring/gofx/ports/client-go/host/deps"
 discovery "k8s.io/api/apidiscovery/v2"
 meta "k8s.io/apimachinery/pkg/apis/meta/v1"
 version "k8s.io/apimachinery/pkg/version"
 apierrors "k8s.io/apimachinery/pkg/api/errors"
)
func boxError(e error) any { v,_:=rt.OptionValue(deps.From(reflect.ValueOf(&e).Elem()));return v }
func unboxError(v any) error { return deps.To(reflect.TypeFor[error](),rt.SomeOf(v),context.Background()).Interface().(error) }
func errOption(e error) any {if e==nil{return rt.NoneOf()};return rt.SomeOf(boxError(e))}
func toList(xs []any) any {v:=rt.Shapes.Nil();for i:=len(xs)-1;i>=0;i--{v=rt.Shapes.Cons(xs[i],v)};return v}
func listValues(v any) []any {var xs []any;for {a,b,ok:=rt.Shapes.Uncons(v);if !ok{return xs};xs=append(xs,a);v=b}}
type tagged struct{tag,message string;keys []string;causes []error}
func(e *tagged) Error()string{return e.message}
func(e *tagged) Is(target error)bool{v,ok:=target.(*tagged);return ok&&e.tag==v.tag}
func TaggedError(tag,message,keys,causes any)any{e:=&tagged{tag:tag.(string),message:message.(string)};for _,k:=range listValues(keys){e.keys=append(e.keys,k.(string))};for _,c:=range listValues(causes){e.causes=append(e.causes,unboxError(c))};return boxError(e)}
func ErrorTag(value any)any{var e *tagged;if errors.As(unboxError(value),&e){return e.tag};return ""}
func ErrorDirectTag(value any)any{if e,ok:=unboxError(value).(*tagged);ok{return e.tag};return ""}
func ErrorEntries(value any)any{var e *tagged;xs:=[]any{};if errors.As(unboxError(value),&e){for i,k:=range e.keys{if i<len(e.causes){xs=append(xs,rt.Tuple2Of(k,boxError(e.causes[i])))}}};return toList(xs)}
func ErrorFacts(value any)any{e:=unboxError(value);var status apierrors.APIStatus;code:=int64(0);if errors.As(e,&status){code=int64(status.Status().Code)};var errno syscall.Errno;hasErrno:=errors.As(e,&errno);return rt.Tuple2Of(errors.Is(e,context.Canceled)||errors.Is(e,context.DeadlineExceeded),rt.Tuple2Of(hasErrno&&errno==syscall.ECONNREFUSED,rt.Tuple2Of(hasErrno&&errno==syscall.ECONNRESET,code)))}
func decode[T any](s string)rt.IO{return func(ctx context.Context)(any,error){if e:=ctx.Err();e!=nil{return nil,e};var v T;e:=json.Unmarshal([]byte(s),&v);if e!=nil{return rt.Tuple2Of(rt.NoneOf(),errOption(e)),nil};return rt.Tuple2Of(rt.SomeOf(deps.From(reflect.ValueOf(v))),rt.NoneOf()),nil}}
func DecodeGroups(s any)rt.IO{return decode[meta.APIGroupList](s.(string))}
func DecodeVersions(s any)rt.IO{return decode[meta.APIVersions](s.(string))}
func DecodeResources(s any)rt.IO{return decode[meta.APIResourceList](s.(string))}
func DecodeAggregated(s any)rt.IO{return decode[discovery.APIGroupDiscoveryList](s.(string))}
func DecodeVersion(s any)rt.IO{return decode[version.Info](s.(string))}
func encode[T any](v any)rt.IO{return func(ctx context.Context)(any,error){x:=deps.To(reflect.TypeFor[T](),v,ctx).Interface().(T);b,e:=json.Marshal(x);return rt.Tuple2Of(string(b),errOption(e)),nil}}
func EncodeGroups(v any)rt.IO{return encode[meta.APIGroupList](v)}
func EncodeResources(v any)rt.IO{return encode[meta.APIResourceList](v)}
func ParseMedia(s any)any{base,params,e:=mime.ParseMediaType(s.(string));xs:=[]any{};for k,v:=range params{xs=append(xs,rt.Tuple2Of(k,v))};return rt.Tuple2Of(base,rt.Tuple2Of(toList(xs),errOption(e)))}
func ReadFile(path any)rt.IO{return func(ctx context.Context)(any,error){if e:=ctx.Err();e!=nil{return nil,e};f,e:=os.Open(path.(string));if e!=nil{return rt.Tuple2Of("",rt.Tuple2Of(int64(0),errOption(e))),nil};defer f.Close();st,e:=f.Stat();if e!=nil{return rt.Tuple2Of("",rt.Tuple2Of(int64(0),errOption(e))),nil};b,e:=io.ReadAll(f);return rt.Tuple2Of(string(b),rt.Tuple2Of(st.ModTime().UnixNano(),errOption(e))),nil}}
func AtomicWrite(path,data,dirMode,fileMode any)rt.IO{return func(ctx context.Context)(any,error){if e:=ctx.Err();e!=nil{return nil,e};dir:=filepath.Dir(path.(string));if e:=os.MkdirAll(dir,os.FileMode(dirMode.(int64)));e!=nil{return errOption(e),nil};f,e:=os.CreateTemp(dir,filepath.Base(path.(string))+".");if e!=nil{return errOption(e),nil};defer os.Remove(f.Name());if _,e=f.Write([]byte(data.(string)));e==nil{e=f.Chmod(os.FileMode(fileMode.(int64)))};closeErr:=f.Close();if e==nil{e=closeErr};if e==nil{e=os.Rename(f.Name(),path.(string))};return errOption(e),nil}}
func JoinPath(a,b string)string{return filepath.Join(a,b)}
