// Copyright 2018 The Kubernetes Authors. Licensed under Apache-2.0.
// External dependency value operations only; no client-go authentication policy.
package host
import("context";"reflect";"github.com/reoring/gofx/ports/client-go/host/deps";meta "k8s.io/apimachinery/pkg/apis/meta/v1";kruntime "k8s.io/apimachinery/pkg/runtime")
func value[T any](ctx context.Context,v any) T {return hostdeps.To(reflect.TypeFor[T](),v,ctx).Interface().(T)}
func raw(ctx context.Context,v any) kruntime.RawExtension {return value[kruntime.RawExtension](ctx,v)}
func CopyTime(v any) any {t:=value[meta.Time](context.Background(),v);return hostdeps.From(reflect.ValueOf(*t.DeepCopy()))}
func CopyTypeMeta(v any) any {t:=value[meta.TypeMeta](context.Background(),v);return hostdeps.From(reflect.ValueOf(t))}
