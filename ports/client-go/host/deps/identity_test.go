package hostdeps_test

import (
 "context"
 "net/http"
 "net/url"
 "reflect"
 "testing"

 "go4t/rt"
 "github.com/reoring/gofx/ports/client-go/host/deps"
)

func TestIdentityFieldPointerLeafAndIngress(t *testing.T) {
 installErrorShapes(t)
 ctx:=context.Background()
 request:=&http.Request{}
 requestView,present:=rt.OptionValue(hostdeps.From(reflect.ValueOf(request)))
 if !present{t.Fatal("request pointer ingress absent")}
 urlField,_:=reflect.TypeFor[http.Request]().FieldByName("URL")
 path:=urlField.Index
 set:=func(value any){t.Helper();if _,err:=hostdeps.SetIdentityField(requestView,"net/http.Request",path,value).(rt.IO)(ctx);err!=nil{t.Fatal(err)}}
 first,err:=url.Parse("https://first.example/original");if err!=nil{t.Fatal(err)}
 firstView,present:=rt.OptionValue(hostdeps.From(reflect.ValueOf(first)));if !present{t.Fatal("URL pointer ingress absent")}
 set(rt.SomeOf(firstView))
 if request.URL!=first{t.Fatal("nil pointer assignment copied or discarded URL identity")}
 hostField,_:=reflect.TypeFor[url.URL]().FieldByName("Host")
 if _,err:=hostdeps.SetIdentityField(firstView,"net/url.URL",hostField.Index,"changed.example").(rt.IO)(ctx);err!=nil{t.Fatal(err)}
 if first.Host!="changed.example"||request.URL.Host!="changed.example"{t.Fatal("addressable ingress mutation was discarded")}
 replacement:=&url.URL{Host:"second.example"}
 replacementView,_:=rt.OptionValue(hostdeps.From(reflect.ValueOf(replacement)))
 set(rt.SomeOf(replacementView))
 if request.URL!=replacement||first.Host!="changed.example"{t.Fatal("replacement overwrote old pointee instead of pointer slot")}
 set(rt.NoneOf())
 if request.URL!=nil{t.Fatal("pointer clear did not retain nil")}
}

func TestIdentityFieldIntermediateAndPrivateRejection(t *testing.T) {
 installErrorShapes(t)
 ctx:=context.Background()
 request:=&http.Request{}
 view,_:=rt.OptionValue(hostdeps.From(reflect.ValueOf(request)))
 urlField,_:=reflect.TypeFor[http.Request]().FieldByName("URL")
 hostField,_:=reflect.TypeFor[url.URL]().FieldByName("Host")
 nested:=append(append([]int{},urlField.Index...),hostField.Index...)
 if _,err:=hostdeps.SetIdentityField(view,"net/http.Request",nested,"host").(rt.IO)(ctx);err==nil||err.Error()!="hostdeps: inaccessible identity field"{t.Fatalf("nil intermediate error: %v",err)}
 if request.URL!=nil{t.Fatal("nil intermediate was allocated")}
 request.URL=&url.URL{}
 if _,err:=hostdeps.SetIdentityField(view,"net/http.Request",nested,"nested.example").(rt.IO)(ctx);err!=nil{t.Fatal(err)}
 if request.URL.Host!="nested.example"{t.Fatal("nonnull intermediate mutation lost")}
 typ:=reflect.TypeFor[http.Request]()
 private:=-1
 for i:=range typ.NumField(){if !typ.Field(i).IsExported(){private=i;break}}
 if private<0{t.Fatal("request private-field fixture missing")}
 if _,err:=hostdeps.SetIdentityField(view,"net/http.Request",[]int{private},rt.NoneOf()).(rt.IO)(ctx);err==nil||err.Error()!="hostdeps: inaccessible identity field"{t.Fatalf("private field error: %v",err)}
 unaddressable:=hostdeps.From(reflect.ValueOf(http.Request{}))
 if _,err:=hostdeps.SetIdentityField(unaddressable,"net/http.Request",urlField.Index,rt.NoneOf()).(rt.IO)(ctx);err==nil||err.Error()!="hostdeps: identity field setter requires pointer for net/http.Request"{t.Fatalf("unowned value error: %v",err)}
}
