// Copyright 2026 The Kubernetes Authors. Licensed under Apache-2.0.
package host

import (
 "context"
 "reflect"
 "mime"
 "strconv"
 "github.com/munnerz/goautoneg"
 "gofx/rt"
 deps "github.com/reoring/gofx/ports/client-go/host/deps"
 "k8s.io/apimachinery/pkg/runtime"
 "k8s.io/apimachinery/pkg/runtime/serializer"
 "k8s.io/apimachinery/pkg/runtime/serializer/cbor"
)
func NegotiateError(value string) any {return boxError(runtime.NegotiateError{ContentType:value,Stream:true})}
func CBORCodecFactory(scheme any) any {return rt.IO(func(ctx context.Context)(any,error){s:=deps.To(reflect.TypeFor[*runtime.Scheme](),rt.SomeOf(scheme),ctx).Interface().(*runtime.Scheme);return boxed(serializer.NewCodecFactory(s,serializer.WithSerializer(cbor.NewSerializerInfo))),nil})}
func AcceptClauses(value string) any {out:=[]any{};for _,clause:=range goautoneg.ParseAccept(value){params:=[]any{};for key,value:=range clause.Params{params=append(params,rt.Tuple2Of(key,value))};out=append(out,rt.Tuple2Of(clause.Type+"/"+clause.SubType,rt.Tuple2Of(clause.Q,list(params))))};return list(out)}
func FormatMedia(media string,params any) string {values:=map[string]string{};for{pair,next,ok:=rt.Shapes.Uncons(params);if !ok{break};key,value:=rt.Shapes.Untuple2(pair);values[key.(string)]=value.(string);params=next};return mime.FormatMediaType(media,values)}
func FormatQuality(value float64) string {return strconv.FormatFloat(value,'g',3,32)}
