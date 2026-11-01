// Copyright 2026 The Kubernetes Authors. Licensed under Apache-2.0.
// Generic HTTP/JSON effects only; authentication policy is implemented in gofx.
package host
import("context";"encoding/base64";"encoding/json";"io";"mime";"net/http";"net/url";"strings";"gofx/rt")
func NewHTTPClient(transport any) rt.IO {return func(ctx context.Context)(any,error){return identity(&http.Client{Transport:value[http.RoundTripper](ctx,rt.SomeOf(transport))}),nil}}
func HTTPRequest(client any,method,address string,headers any,body string,limit int64) rt.IO {return func(ctx context.Context)(any,error){
 request,e:=http.NewRequestWithContext(ctx,method,address,strings.NewReader(body));if e!=nil{return nil,e}
 for {pair,rest,ok:=rt.Shapes.Uncons(headers);if !ok{break};k,v:=rt.Shapes.Untuple2(pair);request.Header.Add(k.(string),v.(string));headers=rest}
 response,e:=value[*http.Client](ctx,client).Do(request);if e!=nil{return nil,e};defer response.Body.Close()
 reader:=io.Reader(response.Body);if limit>=0 {reader=io.LimitReader(reader,limit)};b,e:=io.ReadAll(reader);if e!=nil{return nil,e}
 return rt.Tuple3Of(int64(response.StatusCode),response.Status,rt.Tuple2Of(response.Header.Get("Content-Type"),string(b))),nil
}}
func ParseForm(text string) rt.IO {return func(context.Context)(any,error){values,e:=url.ParseQuery(text);if e!=nil{return nil,e};out:=rt.Shapes.Nil();for k,v:=range values {if len(v)>0 {out=rt.Shapes.Cons(rt.Tuple2Of(k,v[0]),out)}};return out,nil}}
func EncodeBasic(user,password string) string {return base64.StdEncoding.EncodeToString([]byte(url.QueryEscape(user)+":"+url.QueryEscape(password)))}
func MediaType(value string) string {media,_,_:=mime.ParseMediaType(value);return media}
func JSONInteger(value string) rt.IO {return func(context.Context)(any,error){var number json.Number;if err:=json.Unmarshal([]byte(value),&number);err!=nil{return nil,err};return number.Int64()}}
func ByteLength(value string) int64 {return int64(len(value))}
func BytePrefix(value string,limit int64) string {if limit<0 {return ""};if int64(len(value))<=limit{return value};return value[:limit]}
