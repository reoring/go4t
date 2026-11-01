// Generic HTTP serialization, header conversion, and EOF callback effects.
// Cache policy lives exclusively in ../cache.gt.
package host
import (
 "bufio"
 "bytes"
 "context"
 "errors"
 "io"
 "net/http"
 "net/http/httputil"
 "reflect"
 "strings"
 "time"
 "go4t/rt"
 deps "github.com/reoring/gofx/ports/client-go/host/deps"
)
func value[T any](ctx context.Context,v any) T {return deps.To(reflect.TypeFor[T](),rt.SomeOf(v),ctx).Interface().(T)}
func boxed(v any) any {out,_:=rt.OptionValue(deps.From(reflect.ValueOf(v)));return out}
func errValue(e error) any {return deps.ErrorOptionFromGo(e)}
func Client(v any) any {return rt.IO(func(ctx context.Context)(any,error){return boxed(&http.Client{Transport:value[http.RoundTripper](ctx,v)}),nil})}
func Request(v any) any {return rt.IO(func(ctx context.Context)(any,error){r:=value[*http.Request](ctx,v);return rt.Tuple2Of(r.Method,rt.Tuple2Of(r.URL.String(),headers(r.Header))),nil})}
func Response(v any) any {return rt.IO(func(ctx context.Context)(any,error){r:=value[*http.Response](ctx,v);return rt.Tuple2Of(int64(r.StatusCode),headers(r.Header)),nil})}
func headers(h http.Header) any {entries:=make([]any,0,len(h));for key,values:=range h {list:=rt.Shapes.Nil();for i:=len(values)-1;i>=0;i--{list=rt.Shapes.Cons(values[i],list)};entries=append(entries,rt.Tuple2Of(key,list))};list:=rt.Shapes.Nil();for i:=len(entries)-1;i>=0;i--{list=rt.Shapes.Cons(entries[i],list)};return list}
func Headers(v,h any) any {return rt.IO(func(ctx context.Context)(any,error){r:=value[*http.Response](ctx,v);r.Header=make(http.Header);entries:=rt.MapToList(h);for {entry,next,ok:=rt.Shapes.Uncons(entries);if !ok{break};key,values:=rt.Shapes.Untuple2(entry);var strings []string;for {v,n,ok:=rt.Shapes.Uncons(values);if !ok{break};strings=append(strings,v.(string));values=n};r.Header[key.(string)]=strings;entries=next};return rt.Unit{},nil})}
func Decode(data,v any) any {return rt.IO(func(ctx context.Context)(any,error){response,err:=http.ReadResponse(bufio.NewReader(strings.NewReader(data.(string))),value[*http.Request](ctx,v));return rt.Tuple2Of(deps.From(reflect.ValueOf(response)),errValue(err)),nil})}
func Dump(v any) any {return rt.IO(func(ctx context.Context)(any,error){data,err:=httputil.DumpResponse(value[*http.Response](ctx,v),true);return rt.Tuple2Of(string(data),errValue(err)),nil})}
func Timeout(v any) any {return rt.IO(func(ctx context.Context)(any,error){r,err:=http.ReadResponse(bufio.NewReader(strings.NewReader("HTTP/1.1 504 Gateway Timeout\r\n\r\n")),value[*http.Request](ctx,v));if err!=nil{return nil,err};return boxed(r),nil})}
func Date(v any) any {t,err:=time.Parse(time.RFC1123,v.(string));if err!=nil{return rt.NoneOf()};return rt.SomeOf(t.UnixNano())}
func ParseDate(v any) any {t,err:=time.Parse(time.RFC1123,v.(string));return rt.Tuple2Of(t.UnixNano(),errValue(err))}
var errNoDate=errors.New("no Date header")
func NoDate() any {return deps.ErrorFromGo(errNoDate)}
func Duration(v any) any {t,err:=time.ParseDuration(v.(string));if err!=nil{return rt.NoneOf()};return rt.SomeOf(int64(t))}
func Canonical(v string) string {return http.CanonicalHeaderKey(v)}
func TrimSpaces(v string) string {return strings.Trim(v," ")}
type eofReader struct {body io.ReadCloser; buffer bytes.Buffer; response *http.Response; callback any; ctx context.Context}
func (r *eofReader) Read(p []byte)(int,error){n,err:=r.body.Read(p);r.buffer.Write(p[:n]);if err==io.EOF {copy:=*r.response;copy.Body=io.NopCloser(bytes.NewReader(r.buffer.Bytes()));data,e:=httputil.DumpResponse(&copy,true);if e==nil{call:=reflect.ValueOf(r.callback).Call([]reflect.Value{reflect.ValueOf(string(data))})[0].Interface().(rt.IO);if _,e=call(r.ctx);e!=nil{return n,e}}};return n,err}
func (r *eofReader) Close() error{return r.body.Close()}
func OnEOF(v,callback any) any {return rt.IO(func(ctx context.Context)(any,error){r:=value[*http.Response](ctx,v);r.Body=&eofReader{body:r.Body,response:r,callback:callback,ctx:ctx};return rt.Unit{},nil})}
