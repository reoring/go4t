// Copyright 2017 The Kubernetes Authors. Licensed under Apache-2.0.
package host
import (
 "context"
 "reflect"
 "go4t/rt"
 deps "github.com/reoring/gofx/ports/client-go/host/deps"
 metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)
func ParseSelector(value any) any {return rt.IO(func(context.Context)(any,error){selector,err:=metav1.ParseToLabelSelector(value.(string));return rt.Tuple2Of(deps.From(reflect.ValueOf(selector)),deps.ErrorOptionFromGo(err)),nil})}
func RenderSelector(value any) any {return rt.IO(func(ctx context.Context)(any,error){v:=deps.To(reflect.TypeFor[*metav1.LabelSelector](),rt.SomeOf(value),ctx).Interface().(*metav1.LabelSelector);selector,err:=metav1.LabelSelectorAsSelector(v);text:="";if err==nil{text=selector.String()};return rt.Tuple2Of(text,deps.ErrorOptionFromGo(err)),nil})}
