// Copyright 2026 The Kubernetes Authors. Licensed under Apache-2.0.
package host

import (
 "context"
 "fmt"
 "net/http"
 "reflect"
 "go4t/rt"
 deps "github.com/reoring/gofx/ports/client-go/host/deps"
 "k8s.io/apimachinery/pkg/runtime"
 "k8s.io/apimachinery/pkg/runtime/schema"
 "k8s.io/apimachinery/pkg/runtime/serializer/streaming"
 apierrors "k8s.io/apimachinery/pkg/api/errors"
)
func OpenStream(serializer,gv,media,response any) any {return rt.IO(func(ctx context.Context)(any,error){s:=deps.To(reflect.TypeFor[runtime.NegotiatedSerializer](),rt.SomeOf(serializer),ctx).Interface().(runtime.NegotiatedSerializer);version:=deps.To(reflect.TypeFor[schema.GroupVersion](),gv,ctx).Interface().(schema.GroupVersion);r:=deps.To(reflect.TypeFor[*http.Response](),rt.SomeOf(response),ctx).Interface().(*http.Response);info,ok:=runtime.SerializerInfoForMediaType(s.SupportedMediaTypes(),media.(string));if !ok||info.StreamSerializer==nil {return rt.Tuple2Of(rt.NoneOf(),errOption(fmt.Errorf("no stream serializers registered for %s",media))),nil};framed:=info.StreamSerializer.Framer.NewFrameReader(r.Body);outer:=streaming.NewDecoder(framed,info.StreamSerializer.Serializer);inner:=s.DecoderToVersion(info.Serializer,version);a,_:=rt.OptionValue(deps.From(reflect.ValueOf(&outer).Elem()));b,_:=rt.OptionValue(deps.From(reflect.ValueOf(&inner).Elem()));return rt.Tuple2Of(rt.SomeOf(rt.Tuple2Of(a,b)),rt.NoneOf()),nil})}
func IsEOF(value any) bool {e:=unboxError(value);return e.Error()=="EOF"}
func WatchErrorObject(value any,status int64,verb,reason string) any {e:=unboxError(value);obj:=apierrors.NewClientErrorReporter(int(status),verb,reason).AsObject(e);return boxedObject(obj)}
