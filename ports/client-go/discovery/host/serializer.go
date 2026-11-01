// Copyright The Kubernetes Authors. SPDX-License-Identifier: Apache-2.0
package host
import (
 "context"
 "reflect"
 "gofx/rt"
 deps "github.com/reoring/gofx/ports/client-go/host/deps"
 meta "k8s.io/apimachinery/pkg/apis/meta/v1"
 "k8s.io/apimachinery/pkg/runtime"
 "k8s.io/apimachinery/pkg/runtime/schema"
 "k8s.io/apimachinery/pkg/runtime/serializer"
)
func DiscoverySerializer()rt.IO{return func(ctx context.Context)(any,error){s:=runtime.NewScheme();if e:=meta.AddMetaToScheme(s);e!=nil{return nil,e};s.AddUnversionedTypes(schema.GroupVersion{Version:"v1"},&meta.Status{},&meta.APIVersions{},&meta.APIGroupList{},&meta.APIGroup{},&meta.APIResourceList{});codecs:=serializer.NewCodecFactory(s);var negotiated runtime.NegotiatedSerializer=serializer.NegotiatedSerializerWrapper(runtime.SerializerInfo{Serializer:runtime.NoopEncoder{Decoder:codecs.UniversalDecoder()}});v,_:=rt.OptionValue(deps.From(reflect.ValueOf(&negotiated).Elem()));return v,nil}}
