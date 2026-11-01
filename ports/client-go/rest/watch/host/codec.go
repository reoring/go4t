// Copyright 2026 The Kubernetes Authors. Licensed under Apache-2.0.
package host

import (
 "context"
 "reflect"
 "gofx/rt"
 deps "github.com/reoring/gofx/ports/client-go/host/deps"
 metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
 "k8s.io/apimachinery/pkg/runtime"
 "k8s.io/apimachinery/pkg/runtime/serializer/streaming"
)
func errorOption(err error) any {return deps.ErrorOptionFromGo(err)}
func ReadEnvelope(value any) any {return rt.IO(func(ctx context.Context)(any,error){d:=deps.To(reflect.TypeFor[streaming.Decoder](),rt.SomeOf(value),ctx).Interface().(streaming.Decoder);var envelope metav1.WatchEvent;obj,_,e:=d.Decode(nil,&envelope);return rt.Tuple2Of(obj==&envelope,rt.Tuple2Of(envelope.Type,rt.Tuple2Of(string(envelope.Object.Raw),errorOption(e)))),nil})}
func EncodeObject(encoder,object any) any {return rt.IO(func(ctx context.Context)(any,error){e:=deps.To(reflect.TypeFor[runtime.Encoder](),rt.SomeOf(encoder),ctx).Interface().(runtime.Encoder);obj:=deps.To(reflect.TypeFor[runtime.Object](),object,ctx).Interface().(runtime.Object);data,err:=runtime.Encode(e,obj);return rt.Tuple2Of(string(data),errorOption(err)),nil})}
func WriteEnvelope(encoder,kind,data any) any {return rt.IO(func(ctx context.Context)(any,error){e:=deps.To(reflect.TypeFor[streaming.Encoder](),rt.SomeOf(encoder),ctx).Interface().(streaming.Encoder);return errorOption(e.Encode(&metav1.WatchEvent{Type:kind.(string),Object:runtime.RawExtension{Raw:[]byte(data.(string))}})),nil})}
