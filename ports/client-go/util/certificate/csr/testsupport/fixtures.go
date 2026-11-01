// Copyright 2026 The Kubernetes Authors. Licensed under Apache-2.0.
package testsupport
import (
 "context"
 "reflect"
 "go4t/rt"
 "github.com/reoring/gofx/ports/client-go/host/deps"
 v1 "k8s.io/api/certificates/v1"
 "k8s.io/apimachinery/pkg/types"
 apierrors "k8s.io/apimachinery/pkg/api/errors"
 "k8s.io/apimachinery/pkg/runtime/schema"
)
func WithStatus(object,name,uid,condition,certificate any)any{v:=hostdeps.To(reflect.TypeFor[v1.CertificateSigningRequest](),object,context.Background()).Interface().(v1.CertificateSigningRequest);v.Name=name.(string);v.UID=types.UID(uid.(string));v.Status.Certificate=[]byte(certificate.(string));if condition.(string)!=""{v.Status.Conditions=[]v1.CertificateSigningRequestCondition{{Type:v1.RequestConditionType(condition.(string)),Reason:"fixture",Message:"test"}}};return hostdeps.From(reflect.ValueOf(v))}
func APIError(kind any)any{var e error;switch kind.(string){case "AlreadyExists":e=apierrors.NewAlreadyExists(schema.GroupResource{Group:"certificates.k8s.io",Resource:"certificatesigningrequests"},"node");case "NotFound":e=apierrors.NewNotFound(schema.GroupResource{Group:"certificates.k8s.io",Resource:"certificatesigningrequests"},"node");case "Forbidden":e=apierrors.NewForbidden(schema.GroupResource{Group:"certificates.k8s.io",Resource:"certificatesigningrequests"},"node",nil);default:panic("unknown fixture error")};v:=hostdeps.From(reflect.ValueOf(&e).Elem());v,_=rt.OptionValue(v);return v}
