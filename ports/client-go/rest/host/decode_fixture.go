// Copyright 2026 The Kubernetes Authors. Licensed under Apache-2.0.
package host

import (
 "context"
 "go4t/rt"
 metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
 "k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
 "k8s.io/apimachinery/pkg/runtime"
 "k8s.io/apimachinery/pkg/runtime/schema"
)
// This fixture models the Decoder contract allowing a result distinct from into.
type alternateDecoder struct { status string }
func (d alternateDecoder) Decode(_ []byte,_ *schema.GroupVersionKind,_ runtime.Object)(runtime.Object,*schema.GroupVersionKind,error){return &metav1.Status{Status:d.status},nil,nil}
func AlternateDecoderFixture(status string) rt.IO {return func(context.Context)(any,error){var decoder runtime.Decoder=alternateDecoder{status:status};target:=&unstructured.Unstructured{Object:map[string]any{"marker":"destination"}};return rt.Tuple2Of(boxed(decoder),boxedObject(target)),nil}}
