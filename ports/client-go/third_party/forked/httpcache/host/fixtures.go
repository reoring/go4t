package host
import (
 "bufio"
 "context"
 "io"
 "net/http"
 "strings"
 "go4t/rt"
)
func FixtureRequest(method,address any) any {return rt.IO(func(ctx context.Context)(any,error){r,e:=http.NewRequestWithContext(ctx,method.(string),address.(string),nil);if e!=nil{return nil,e};return boxed(r),nil})}
func FixtureResponse(raw any) any {return rt.IO(func(context.Context)(any,error){r,e:=http.ReadResponse(bufio.NewReader(strings.NewReader(raw.(string))),nil);if e!=nil{return nil,e};return boxed(r),nil})}
func FixtureReadBody(v any) any {return rt.IO(func(ctx context.Context)(any,error){r:=value[*http.Response](ctx,v);data,e:=io.ReadAll(r.Body);return string(data),e})}
func FixtureClose(v any) any {return rt.IO(func(ctx context.Context)(any,error){return rt.Unit{},value[*http.Response](ctx,v).Body.Close()})}
