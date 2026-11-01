// Package proofhost contains only fixtures for the assigned native dependency proof.
package proofhost

import (
 "context"
 "go4t/rt"
 "reflect"

 corev1 "k8s.io/api/core/v1"
 apimeta "k8s.io/apimachinery/pkg/api/meta"
 apierrors "k8s.io/apimachinery/pkg/api/errors"
 runtimeschema "k8s.io/apimachinery/pkg/runtime/schema"
 "k8s.io/apimachinery/pkg/runtime"
 "k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
 cbor "k8s.io/apimachinery/pkg/runtime/serializer/cbor/direct"
 "sigs.k8s.io/structured-merge-diff/v6/schema"

 "github.com/reoring/gofx/ports/client-go/host/deps"
)

func init() {
 hostdeps.RegisterFunction("proof.aliasConvert",func(a,b *corev1.Pod)(bool,error){
  scheme:=runtime.NewScheme()
  if err:=corev1.AddToScheme(scheme);err!=nil{return false,err}
  if err:=scheme.Convert(a,b,nil);err!=nil{return false,err}
  b.SetName("alias-updated")
  return a==b&&a.GetName()=="alias-updated",nil
 })
 hostdeps.RegisterFunction("proof.sameMap",func(a,b *schema.Map)bool{return a==b})
}
func AliasConvert(a,b any) any {return hostdeps.Function("proof.aliasConvert",[]int{0,1},a,b)}
func RecursiveFixture() any {m:=&schema.Map{};r:=schema.TypeRef{Inlined:schema.Atom{Map:m}};m.ElementType=r;return hostdeps.From(reflect.ValueOf(r))}
func SameMap(a,b any) any {return hostdeps.Function("proof.sameMap",[]int{0,1},a,b)}
func PointerMap() any {return rt.IO(func(context.Context)(any,error){key:=&corev1.Pod{};return hostdeps.From(reflect.ValueOf(map[*corev1.Pod]string{key:"pointer-key"})),nil})}
func MetadataView(value any) any {return rt.IO(func(ctx context.Context)(any,error){object:=hostdeps.To(reflect.TypeFor[runtime.Object](),rt.SomeOf(value),ctx).Interface().(runtime.Object);view,err:=apimeta.Accessor(object);if err!=nil{return nil,err};return hostdeps.From(reflect.ValueOf(&view).Elem()),nil})}
func SameObjectStorage(left,right any) any {typ:=reflect.TypeFor[runtime.Object]();a:=reflect.ValueOf(hostdeps.To(typ,rt.SomeOf(left),context.Background()).Interface());b:=reflect.ValueOf(hostdeps.To(typ,rt.SomeOf(right),context.Background()).Interface());return a.Type()==b.Type()&&a.Kind()==reflect.Pointer&&b.Kind()==reflect.Pointer&&a.Pointer()==b.Pointer()}
func SameDescriptor(left,right any) any {return left==right}
func TypedNilObject() any {var object runtime.Object=(*corev1.Pod)(nil);return hostdeps.From(reflect.ValueOf(&object).Elem())}
func fixtureInterface(value any) any {wrapped:=hostdeps.From(reflect.ValueOf(&value).Elem());result,_:=rt.OptionValue(wrapped);return result}
func AlreadyExists() any {return fixtureInterface(apierrors.NewAlreadyExists(runtimeschema.GroupResource{Resource:"pods"},"existing"))}
func NotRegistered() any {return fixtureInterface(runtime.NewNotRegisteredErrForKind("proof",runtimeschema.GroupVersionKind{Kind:"Missing"}))}
func LargeIntegerObject() any {return fixtureInterface(&unstructured.Unstructured{Object:map[string]any{"apiVersion":"v1","kind":"Pod","n":int64(9007199254740993)}})}
func HasLargeInteger(value any) any {
 object:=hostdeps.To(reflect.TypeFor[runtime.Object](),rt.SomeOf(value),context.Background()).Interface().(runtime.Object)
 n,ok:=object.(*unstructured.Unstructured).Object["n"].(int64)
 return ok&&n==9007199254740993
}
func CBORHasLargeInteger(value any) any {
 return rt.IO(func(context.Context)(any,error){
  var object unstructured.Unstructured
  err:=cbor.Unmarshal([]byte(value.(string)),&object)
  if err!=nil{return nil,err}
  n,ok:=object.Object["n"].(int64)
  return ok&&n==9007199254740993,nil
 })
}
type customJSON struct { corev1.Pod }
func (*customJSON) MarshalJSON() ([]byte,error) {return []byte(`{"custom":9007199254740993}`),nil}
func CustomJSON() any {return fixtureInterface(&customJSON{})}
