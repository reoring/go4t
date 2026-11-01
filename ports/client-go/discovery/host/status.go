// Copyright The Kubernetes Authors. SPDX-License-Identifier: Apache-2.0
package host
import (
 "context"
 "fmt"
 "reflect"
 deps "github.com/reoring/gofx/ports/client-go/host/deps"
 meta "k8s.io/apimachinery/pkg/apis/meta/v1"
 apierrors "k8s.io/apimachinery/pkg/api/errors"
 version "k8s.io/apimachinery/pkg/version"
)
func StatusError(code,reason,message any)any{return boxError(&apierrors.StatusError{ErrStatus:meta.Status{Status:meta.StatusFailure,Code:int32(code.(int64)),Reason:meta.StatusReason(reason.(string)),Message:message.(string)}})}
func VersionMismatch(server,client any)any{s:=deps.To(reflect.TypeFor[version.Info](),server,context.Background()).Interface().(version.Info);c:=deps.To(reflect.TypeFor[version.Info](),client,context.Background()).Interface().(version.Info);return boxError(fmt.Errorf("server version (%#v) differs from client version (%#v)",&s,c))}
