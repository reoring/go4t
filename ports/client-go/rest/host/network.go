// Copyright 2026 The Kubernetes Authors. Licensed under Apache-2.0.
package host

import (
 "context"
 "fmt"
 "io"
 "mime"
 "net/http"
 "net/url"
 "os"
 "reflect"
 "strconv"
 "strings"
 "time"

 "gofx/rt"
 deps "github.com/reoring/gofx/ports/client-go/host/deps"
 apierrors "k8s.io/apimachinery/pkg/api/errors"
 metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
 kruntime "k8s.io/apimachinery/pkg/runtime"
 "k8s.io/apimachinery/pkg/runtime/schema"
 knet "k8s.io/apimachinery/pkg/util/net"
 "golang.org/x/net/http2"
)
func boxed(value any) any {v:=deps.From(reflect.ValueOf(value));if reflect.TypeOf(value).Kind()==reflect.Pointer||reflect.TypeOf(value).Kind()==reflect.Interface {if inner,ok:=rt.OptionValue(v);ok{return inner}};return v}
func errOption(err error) any {return deps.ErrorOptionFromGo(err)}
func CanonicalHeader(value string) string {return http.CanonicalHeaderKey(value)}
func ParseURL(value string) any {u,err:=url.Parse(value);if err!=nil{return rt.Tuple2Of(rt.NoneOf(),errOption(err))};return rt.Tuple2Of(rt.SomeOf(boxed(u)),rt.NoneOf())}
func URLParts(value any) any {return rt.IO(func(ctx context.Context)(any,error){u:=deps.To(reflect.TypeFor[*url.URL](),rt.SomeOf(value),ctx).Interface().(*url.URL);return rt.Tuple2Of(u.Scheme,rt.Tuple2Of(u.Host,u.Path)),nil})}
func URLWith(value,path,query,fragment any) any {return rt.IO(func(ctx context.Context)(any,error){u:=*deps.To(reflect.TypeFor[*url.URL](),rt.SomeOf(value),ctx).Interface().(*url.URL);u.Path=path.(string);u.RawPath="";u.RawQuery=query.(string);u.Fragment=fragment.(string);u.RawFragment="";return boxed(&u),nil})}
func URLString(value any) any {return rt.IO(func(ctx context.Context)(any,error){return deps.To(reflect.TypeFor[*url.URL](),rt.SomeOf(value),ctx).Interface().(*url.URL).String(),nil})}
func ReadFile(name string) rt.IO {return func(ctx context.Context)(any,error){if err:=ctx.Err();err!=nil{return nil,err};b,e:=os.ReadFile(name);return rt.Tuple2Of(string(b),errOption(e)),nil}}
func Getenv(name string) rt.IO {return func(ctx context.Context)(any,error){return os.Getenv(name),ctx.Err()}}
func QueryEscape(value string) string {return url.QueryEscape(value)}
func QueryPairs(value string) any {u,err:=url.ParseQuery(value);if err!=nil{return rt.Tuple2Of(rt.Shapes.Nil(),errOption(err))};var pairs []any;for k,vs:=range u {for _,v:=range vs {pairs=append(pairs,rt.Tuple2Of(k,v))}};return rt.Tuple2Of(list(pairs),rt.NoneOf())}
func list(values []any) any {out:=rt.Shapes.Nil();for i:=len(values)-1;i>=0;i-- {out=rt.Shapes.Cons(values[i],out)};return out}
func Duration(value int64) string {return time.Duration(value).String()}
func ParseInt(value string) (int64,bool) {n,e:=strconv.ParseInt(value,10,64);return n,e==nil}
func NewRequest(verb,location,body,reader,headers any) any {return rt.IO(func(ctx context.Context)(any,error){u:=deps.To(reflect.TypeFor[*url.URL](),rt.SomeOf(location),ctx).Interface().(*url.URL);var r io.Reader;if raw,ok:=rt.OptionValue(body);ok {r=strings.NewReader(raw.(string))};if raw,ok:=rt.OptionValue(reader);ok {r=deps.To(reflect.TypeFor[io.Reader](),rt.SomeOf(raw),ctx).Interface().(io.Reader)};req,e:=http.NewRequestWithContext(ctx,verb.(string),u.String(),r);if e!=nil {return rt.Tuple2Of(rt.NoneOf(),errOption(e)),nil};for {pair,tail,ok:=rt.Shapes.Uncons(headers);if !ok{break};key,values:=rt.Shapes.Untuple2(pair);for {value,rest,present:=rt.Shapes.Uncons(values);if !present{break};req.Header.Add(key.(string),value.(string));values=rest};headers=tail};return rt.Tuple2Of(rt.SomeOf(boxed(req)),rt.NoneOf()),nil})}
func ResponseInfo(value any) any {return rt.IO(func(ctx context.Context)(any,error){r:=deps.To(reflect.TypeFor[*http.Response](),rt.SomeOf(value),ctx).Interface().(*http.Response);return rt.Tuple2Of(int64(r.StatusCode),rt.Tuple2Of(r.Header.Get("Content-Type"),r.Header.Get("Retry-After"))),nil})}
func ResponseBody(value any) any {return rt.IO(func(ctx context.Context)(any,error){r:=deps.To(reflect.TypeFor[*http.Response](),rt.SomeOf(value),ctx).Interface().(*http.Response);if r.Body==nil{return rt.NoneOf(),nil};return rt.SomeOf(boxedReader(r.Body)),nil})}
func boxedReader(reader io.Reader) any {return boxedInterfaceReader(reader)}
func boxedInterfaceReader(reader io.Reader) any {v,_:=rt.OptionValue(deps.From(reflect.ValueOf(&reader).Elem()));return v}
func ReadBody(value any) any {return rt.IO(func(ctx context.Context)(any,error){if err:=ctx.Err();err!=nil{return nil,err};r:=deps.To(reflect.TypeFor[*http.Response](),rt.SomeOf(value),ctx).Interface().(*http.Response);if r.Body==nil{return rt.Tuple2Of("",rt.NoneOf()),nil};b,e:=io.ReadAll(r.Body);return rt.Tuple2Of(string(b),errOption(e)),nil})}
func CloseResponse(value any) any {return rt.IO(func(ctx context.Context)(any,error){r:=deps.To(reflect.TypeFor[*http.Response](),rt.SomeOf(value),ctx).Interface().(*http.Response);if r.Body!=nil {return rt.Unit{},r.Body.Close()};return rt.Unit{},nil})}
func DrainResponse(value any) any {return rt.IO(func(ctx context.Context)(any,error){r:=deps.To(reflect.TypeFor[*http.Response](),rt.SomeOf(value),ctx).Interface().(*http.Response);if r.Body!=nil {_,_=io.CopyN(io.Discard,r.Body,2048);_=r.Body.Close()};return rt.Unit{},nil})}
func WarningHeaders(value any) any {return rt.IO(func(ctx context.Context)(any,error){r:=deps.To(reflect.TypeFor[*http.Response](),rt.SomeOf(value),ctx).Interface().(*http.Response);warnings,_:=knet.ParseWarningHeaders(r.Header.Values("Warning"));out:=[]any{};for _,w:=range warnings {out=append(out,rt.Tuple2Of(int64(w.Code),rt.Tuple2Of(w.Agent,w.Text)))};return list(out),nil})}
func MediaType(value string) any {media,_,e:=mime.ParseMediaType(value);return rt.Tuple2Of(media,errOption(e))}
func ConnectionReset(value any) bool {return knet.IsConnectionReset(unboxError(value))}
func ProbableEOF(value any) bool {return knet.IsProbableEOF(unboxError(value))}
func HTTP2ConnectionLost(value any) bool {return knet.IsHTTP2ConnectionLost(unboxError(value))}
func NetworkTimeout(value any) bool {return knet.IsTimeout(unboxError(value))}
func GenericResponseError(status int64,method,group,resource,name,message string,retry int64) any {return boxError(apierrors.NewGenericServerResponse(int(status),method,schema.GroupResource{Group:group,Resource:resource},name,message,int(retry),true))}
func StatusError(value any) any {return rt.IO(func(ctx context.Context)(any,error){obj:=deps.To(reflect.TypeFor[kruntime.Object](),rt.SomeOf(value),ctx).Interface().(kruntime.Object);if status,ok:=obj.(*metav1.Status);ok {return rt.Tuple2Of(status.Status,errOption(apierrors.FromObject(status))),nil};return rt.Tuple2Of("",rt.NoneOf()),nil})}
func IsUnexpected(value any) bool {return apierrors.IsUnexpectedServerError(unboxError(value))}
func PrefixBytes(value string,size int64) string {if int64(len(value))>size{return value[:size]};return value}
func InternalError(value any) any {return boxError(apierrors.NewInternalError(unboxError(value)))}
func NewDecoder(serializer,gv,media any) any {return rt.IO(func(ctx context.Context)(any,error){s:=deps.To(reflect.TypeFor[kruntime.NegotiatedSerializer](),rt.SomeOf(serializer),ctx).Interface().(kruntime.NegotiatedSerializer);version:=deps.To(reflect.TypeFor[schema.GroupVersion](),gv,ctx).Interface().(schema.GroupVersion);m:=media.(string);info,ok:=kruntime.SerializerInfoForMediaType(s.SupportedMediaTypes(),m);if !ok{return rt.Tuple2Of(rt.NoneOf(),errOption(fmt.Errorf("no serializers registered for %s",m))),nil};decoder:=s.DecoderToVersion(info.Serializer,version);v,_:=rt.OptionValue(deps.From(reflect.ValueOf(&decoder).Elem()));return rt.Tuple2Of(rt.SomeOf(v),rt.NoneOf()),nil})}
func EncodeObject(serializer,gv,media,object any) any {return rt.IO(func(ctx context.Context)(any,error){s:=deps.To(reflect.TypeFor[kruntime.NegotiatedSerializer](),rt.SomeOf(serializer),ctx).Interface().(kruntime.NegotiatedSerializer);version:=deps.To(reflect.TypeFor[schema.GroupVersion](),gv,ctx).Interface().(schema.GroupVersion);obj:=deps.To(reflect.TypeFor[kruntime.Object](),rt.SomeOf(object),ctx).Interface().(kruntime.Object);info,ok:=kruntime.SerializerInfoForMediaType(s.SupportedMediaTypes(),media.(string));if !ok{return rt.Tuple2Of("",errOption(fmt.Errorf("no serializers registered for %s",media))),nil};b,e:=kruntime.Encode(s.EncoderForVersion(info.Serializer,version),obj);return rt.Tuple2Of(string(b),errOption(e)),nil})}
func EncodeParameters(object,codec,gv any) any {return rt.IO(func(ctx context.Context)(any,error){obj:=deps.To(reflect.TypeFor[kruntime.Object](),rt.SomeOf(object),ctx).Interface().(kruntime.Object);c:=deps.To(reflect.TypeFor[kruntime.ParameterCodec](),rt.SomeOf(codec),ctx).Interface().(kruntime.ParameterCodec);version:=deps.To(reflect.TypeFor[schema.GroupVersion](),gv,ctx).Interface().(schema.GroupVersion);values,e:=c.EncodeParameters(obj,version);pairs:=[]any{};for key,vs:=range values{for _,v:=range vs{pairs=append(pairs,rt.Tuple2Of(key,v))}};return rt.Tuple2Of(list(pairs),errOption(e)),nil})}
func URLErrorCause(value any) any {e,ok:=unboxError(value).(*url.Error);if !ok||e==nil||e.Unwrap()==nil{return rt.NoneOf()};return errOption(e.Unwrap())}
func HTTP2StreamError(value any) bool {_,ok:=unboxError(value).(http2.StreamError);return ok}
