// Package proofhost contains only fixtures for the assigned native dependency proof.
package proofhost

import (
 "context"
 "gofx/rt"
 "reflect"

 corev1 "k8s.io/api/core/v1"
 "k8s.io/apimachinery/pkg/runtime"
 "sigs.k8s.io/structured-merge-diff/v6/schema"

 "github.com/reoring/gofx/ports/client-go/internal/hostdeps"
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
