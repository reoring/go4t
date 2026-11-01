// Copyright 2014 The Kubernetes Authors. Licensed under Apache-2.0.
package host
import("context";"reflect";"gofx/rt";"github.com/reoring/gofx/ports/client-go/host/deps";"k8s.io/apimachinery/pkg/runtime/schema")
func GroupVersionKind(version,kind string)any{return hostdeps.From(reflect.ValueOf(schema.FromAPIVersionAndKind(version,kind)))}
func VersionAndKind(value any)any{v:=hostdeps.To(reflect.TypeFor[schema.GroupVersionKind](),value,context.Background()).Interface().(schema.GroupVersionKind);version,kind:=v.ToAPIVersionAndKind();return rt.Tuple2Of(version,kind)}
