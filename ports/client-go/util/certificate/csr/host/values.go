// Copyright 2026 The Kubernetes Authors. Licensed under Apache-2.0.
// Typed external API conversion only; request/wait decisions are native go4t.
package host
import (
 "context"
 "reflect"
 "go4t/rt"
 "github.com/reoring/gofx/ports/client-go/host/deps"
 v1 "k8s.io/api/certificates/v1"
 beta "k8s.io/api/certificates/v1beta1"
 metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
 "k8s.io/apimachinery/pkg/runtime"
)
func value[T any](v any)T{var result T;reflect.ValueOf(&result).Elem().Set(hostdeps.To(reflect.TypeFor[T](),v,context.Background()));return result}
func external(v any)any{return hostdeps.From(reflect.ValueOf(v))}
func NewRequest(name,signer,request,expiration,usages any)any{c:=v1.CertificateSigningRequest{TypeMeta:metav1.TypeMeta{Kind:"CertificateSigningRequest"},ObjectMeta:metav1.ObjectMeta{Name:name.(string)},Spec:v1.CertificateSigningRequestSpec{SignerName:signer.(string),Request:[]byte(request.(string)),ExpirationSeconds:value[*int32](expiration)}};for _,s:=range value[[]string](rt.SomeOf(usages)){c.Spec.Usages=append(c.Spec.Usages,v1.KeyUsage(s))};if c.Name==""{c.GenerateName="csr-"};return external(c)}
func ToBeta(v any)any{a:=value[v1.CertificateSigningRequest](v);b:=beta.CertificateSigningRequest{TypeMeta:a.TypeMeta,ObjectMeta:a.ObjectMeta,Spec:beta.CertificateSigningRequestSpec{Request:a.Spec.Request,SignerName:&a.Spec.SignerName},Status:beta.CertificateSigningRequestStatus{Certificate:a.Status.Certificate}};for _,u:=range a.Spec.Usages{b.Spec.Usages=append(b.Spec.Usages,beta.KeyUsage(u))};for _,c:=range a.Status.Conditions{b.Status.Conditions=append(b.Status.Conditions,beta.CertificateSigningRequestCondition{Type:beta.RequestConditionType(c.Type),Status:c.Status,Reason:c.Reason,Message:c.Message,LastUpdateTime:c.LastUpdateTime,LastTransitionTime:c.LastTransitionTime})};return external(b)}
func convert(a beta.CertificateSigningRequest)v1.CertificateSigningRequest{b:=v1.CertificateSigningRequest{TypeMeta:a.TypeMeta,ObjectMeta:a.ObjectMeta,Spec:v1.CertificateSigningRequestSpec{Request:a.Spec.Request},Status:v1.CertificateSigningRequestStatus{Certificate:a.Status.Certificate}};if a.Spec.SignerName!=nil{b.Spec.SignerName=*a.Spec.SignerName};for _,u:=range a.Spec.Usages{b.Spec.Usages=append(b.Spec.Usages,v1.KeyUsage(u))};for _,c:=range a.Status.Conditions{b.Status.Conditions=append(b.Status.Conditions,v1.CertificateSigningRequestCondition{Type:v1.RequestConditionType(c.Type),Status:c.Status,Reason:c.Reason,Message:c.Message,LastUpdateTime:c.LastUpdateTime,LastTransitionTime:c.LastTransitionTime})};return b}
func FromBeta(v any)any{c:=convert(value[beta.CertificateSigningRequest](v));c.Status=v1.CertificateSigningRequestStatus{};return external(c)}
func ListFromBeta(v any)any{a:=value[beta.CertificateSigningRequestList](v);b:=v1.CertificateSigningRequestList{TypeMeta:a.TypeMeta,ListMeta:a.ListMeta};for _,item:=range a.Items{b.Items=append(b.Items,convert(item))};return external(b)}
func EventObject(v any)any{object:=value[runtime.Object](v);switch c:=object.(type){case *v1.CertificateSigningRequest:return rt.SomeOf(external(*c));case *beta.CertificateSigningRequest:return rt.SomeOf(external(convert(*c)));default:return rt.NoneOf()}}
func Conditions(v any)any{a:=value[v1.CertificateSigningRequest](v);r:=rt.Shapes.Nil();for i:=len(a.Status.Conditions)-1;i>=0;i--{c:=a.Status.Conditions[i];r=rt.Shapes.Cons(rt.Tuple2Of(string(c.Type),rt.Tuple2Of(c.Reason,c.Message)),r)};return r}
func Name(v any)any{return value[v1.CertificateSigningRequest](v).Name}
func UID(v any)any{return string(value[v1.CertificateSigningRequest](v).UID)}
func Request(v any)any{return string(value[v1.CertificateSigningRequest](v).Spec.Request)}
func Signer(v any)any{return value[v1.CertificateSigningRequest](v).Spec.SignerName}
func Certificate(v any)any{return string(value[v1.CertificateSigningRequest](v).Status.Certificate)}
func ListOptions(name,rv any)any{return external(metav1.ListOptions{FieldSelector:"metadata.name="+name.(string),ResourceVersion:rv.(string)})}
func ListItems(v any)any{a:=value[v1.CertificateSigningRequestList](v);r:=rt.Shapes.Nil();for i:=len(a.Items)-1;i>=0;i--{r=rt.Shapes.Cons(external(a.Items[i]),r)};return r}
func ListVersion(v any)any{return value[v1.CertificateSigningRequestList](v).ResourceVersion}
