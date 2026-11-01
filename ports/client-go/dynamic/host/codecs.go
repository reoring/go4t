// Copyright 2018 The Kubernetes Authors. Licensed under Apache-2.0.
// Only apimachinery constructors and canonical value conversions live here.
package host

import (
 "context"
 "reflect"
 "go4t/rt"
 deps "github.com/reoring/gofx/ports/client-go/host/deps"
 metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
 metascheme "k8s.io/apimachinery/pkg/apis/meta/internalversion/scheme"
 "k8s.io/apimachinery/pkg/runtime"
 "k8s.io/apimachinery/pkg/runtime/schema"
 "k8s.io/apimachinery/pkg/runtime/serializer/cbor"
 jsonserializer "k8s.io/apimachinery/pkg/runtime/serializer/json"
)

// Native non-null handles omit only the outer pointer/interface presence layer.
// Converting through an interface slot retains the original Go pointer identity.
func box(value any) any {
 if value == nil { return rt.NoneOf() }
 v, _ := rt.OptionValue(deps.From(reflect.ValueOf(&value).Elem()))
 return v
}
func from[T any](ctx context.Context,value any) T {
 t := reflect.TypeFor[T]()
 if t.Kind() == reflect.Pointer || t.Kind() == reflect.Interface { value = rt.SomeOf(value) }
 return deps.To(t,value,ctx).Interface().(T)
}
func MetaScheme(version any) any { return rt.IO(func(ctx context.Context)(any,error){ s:=runtime.NewScheme();metav1.AddToGroupVersion(s,from[schema.GroupVersion](ctx,version));return box(s),nil }) }
func SchemeCreater(value any) any {return rt.IO(func(ctx context.Context)(any,error){return box(runtime.ObjectCreater(from[*runtime.Scheme](ctx,value))),nil})}
func SchemeTyper(value any) any {return rt.IO(func(ctx context.Context)(any,error){return box(runtime.ObjectTyper(from[*runtime.Scheme](ctx,value))),nil})}
func VersionEncoder(encoder,version,typer any) any {return rt.IO(func(ctx context.Context)(any,error){return box(runtime.Encoder(runtime.WithVersionEncoder{Encoder:from[runtime.Encoder](ctx,encoder),Version:from[runtime.GroupVersioner](ctx,version),ObjectTyper:from[runtime.ObjectTyper](ctx,typer)})),nil})}
func JSONSerializer(creater,typer,pretty any) any {return rt.IO(func(ctx context.Context)(any,error){
 return box(runtime.Serializer(jsonserializer.NewSerializerWithOptions(jsonserializer.DefaultMetaFactory,from[runtime.ObjectCreater](ctx,creater),from[runtime.ObjectTyper](ctx,typer),jsonserializer.SerializerOptions{Pretty:pretty.(bool)}))),nil
})}
func CBORSerializer(creater,typer,transcode any) any {return rt.IO(func(ctx context.Context)(any,error){
 c,t:=from[runtime.ObjectCreater](ctx,creater),from[runtime.ObjectTyper](ctx,typer)
 if value,ok:=rt.OptionValue(transcode);ok{return box(runtime.Serializer(cbor.NewSerializer(c,t,cbor.Transcode(value.(bool))))),nil}
 return box(runtime.Serializer(cbor.NewSerializer(c,t))),nil
})}
func JSONFramer() any {return rt.IO(func(context.Context)(any,error){return box(jsonserializer.Framer),nil})}
func CBORFramer() any {return rt.IO(func(context.Context)(any,error){return box(cbor.NewFramer()),nil})}
func MetadataSerializer() any {return rt.IO(func(context.Context)(any,error){return box(runtime.NegotiatedSerializer(metascheme.Codecs.WithoutConversion())),nil})}
