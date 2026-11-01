// Copyright The Kubernetes Authors. SPDX-License-Identifier: Apache-2.0
// External apimachinery error construction/conversion, no mapper policy.
package host
import (
 "context"
 "reflect"
 "go4t/rt"
 deps "github.com/reoring/gofx/ports/client-go/host/deps"
 "k8s.io/apimachinery/pkg/api/meta"
 "k8s.io/apimachinery/pkg/runtime/schema"
)
func boxError(e error)any{v,_:=rt.OptionValue(deps.From(reflect.ValueOf(&e).Elem()));return v}
func from[T any](v any)T{return deps.To(reflect.TypeFor[T](),v,context.Background()).Interface().(T)}
func NoResourceError(v any)any{return boxError(&meta.NoResourceMatchError{PartialResource:from[schema.GroupVersionResource](v)})}
func NoKindError(v,versions any)any{return boxError(&meta.NoKindMatchError{GroupKind:from[schema.GroupKind](v),SearchedVersions:from[[]string](rt.SomeOf(versions))})}
func AmbiguousResourceError(v,rs,ks any)any{return boxError(&meta.AmbiguousResourceError{PartialResource:from[schema.GroupVersionResource](v),MatchingResources:from[[]schema.GroupVersionResource](rt.SomeOf(rs)),MatchingKinds:from[[]schema.GroupVersionKind](rt.SomeOf(ks))})}
func AmbiguousKindError(v,rs,ks any)any{return boxError(&meta.AmbiguousKindError{PartialKind:from[schema.GroupVersionKind](v),MatchingResources:from[[]schema.GroupVersionResource](rt.SomeOf(rs)),MatchingKinds:from[[]schema.GroupVersionKind](rt.SomeOf(ks))})}
func IsNoMatchError(v any)any{return meta.IsNoMatchError(from[error](rt.SomeOf(v)))}
