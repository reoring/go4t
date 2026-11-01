package hostdeps_test

import (
 "context"
 "reflect"
 "testing"

 "go4t/rt"
 "github.com/reoring/gofx/ports/client-go/host/deps"
 "k8s.io/apimachinery/pkg/runtime"
)

type distinctSerializerInfo runtime.SerializerInfo

func TestIdentityStructValueConsumption(t *testing.T) {
 installErrorShapes(t)
 ctx:=context.Background()
 boxed,err:=hostdeps.NewIdentity("k8s.io/apimachinery/pkg/runtime.SerializerInfo").(rt.IO)(ctx)
 if err!=nil{t.Fatal(err)}
 source:=hostdeps.To(reflect.TypeFor[any](),rt.SomeOf(boxed),ctx).Interface().(*runtime.SerializerInfo)
 stream:=&runtime.StreamSerializerInfo{EncodesAsText:true}
 source.MediaType="application/json"
 source.StreamSerializer=stream
 converted:=hostdeps.To(reflect.TypeFor[runtime.SerializerInfo](),boxed,ctx)
 if converted.Type()!=reflect.TypeFor[runtime.SerializerInfo](){t.Fatal("wrong by-value type")}
 consume:=reflect.ValueOf(func(value runtime.SerializerInfo) string {
  if value.StreamSerializer!=stream{t.Fatal("by-value consumption deep-copied nested identity")}
  original:=value.MediaType
  value.MediaType="changed copy"
  return original
 })
 if got:=consume.Call([]reflect.Value{converted})[0].String();got!="application/json"{t.Fatalf("Call value = %q",got)}
 var assigned runtime.SerializerInfo
 reflect.ValueOf(&assigned).Elem().Set(converted)
 assigned.MediaType="assigned copy"
 if source.MediaType!="application/json"{t.Fatal("Go by-value mutation changed canonical storage")}
 if assigned.StreamSerializer!=stream{t.Fatal("Set deep-copied nested identity")}
 if got:=hostdeps.To(reflect.TypeFor[any](),rt.SomeOf(boxed),ctx).Interface();got!=source{t.Fatal("interface path replaced pointer identity")}
 if got:=hostdeps.To(reflect.TypeFor[*runtime.SerializerInfo](),rt.SomeOf(boxed),ctx).Interface();got!=source{t.Fatal("pointer path replaced identity")}
}

func TestIdentityStructValueRejectsCoercions(t *testing.T) {
 installErrorShapes(t)
 ctx:=context.Background()
 boxed,err:=hostdeps.NewIdentity("k8s.io/apimachinery/pkg/runtime.SerializerInfo").(rt.IO)(ctx)
 if err!=nil{t.Fatal(err)}
 var nilPointer *runtime.SerializerInfo
 var nilInterface any=nilPointer
 nilBox,present:=rt.OptionValue(hostdeps.From(reflect.ValueOf(&nilInterface).Elem()))
 if !present{t.Fatal("typed-nil fixture lost interface presence")}
 if got:=hostdeps.To(reflect.TypeFor[*runtime.SerializerInfo](),rt.SomeOf(nilBox),ctx).Interface();got!=nilPointer{t.Fatal("typed-nil pointer conversion changed")}
 if got:=hostdeps.To(reflect.TypeFor[*runtime.SerializerInfo](),rt.NoneOf(),ctx);!got.IsNil(){t.Fatal("absent pointer conversion changed")}
 cases:=[]struct{name string; target reflect.Type; value any}{
  {"nil pointer to value",reflect.TypeFor[runtime.SerializerInfo](),nilBox},
  {"distinct named struct",reflect.TypeFor[distinctSerializerInfo](),boxed},
  {"different struct",reflect.TypeFor[runtime.StreamSerializerInfo](),boxed},
  {"nonstruct target",reflect.TypeFor[string](),boxed},
  {"Option is not struct",reflect.TypeFor[runtime.SerializerInfo](),rt.SomeOf(boxed)},
 }
 for _,tc:=range cases{t.Run(tc.name,func(t *testing.T){
  defer func(){if recover()==nil{t.Fatal("invalid value coercion accepted")}}()
  hostdeps.To(tc.target,tc.value,ctx)
 })}
}
