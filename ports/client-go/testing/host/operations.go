// Copyright 2015 The Kubernetes Authors. Licensed under Apache-2.0.
// External Kubernetes codecs, metadata, patch and field-manager adapters only.
package host

import (
 "context"
 "encoding/json"
 "path"
 "reflect"
 "strconv"
 "gofx/rt"
 deps "github.com/reoring/gofx/ports/client-go/host/deps"
 jsonpatch "gopkg.in/evanphx/json-patch.v4"
 apierrors "k8s.io/apimachinery/pkg/api/errors"
 "k8s.io/apimachinery/pkg/api/meta"
 "k8s.io/apimachinery/pkg/api/meta/testrestmapper"
 metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
 "k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
 "k8s.io/apimachinery/pkg/fields"
 "k8s.io/apimachinery/pkg/labels"
 "k8s.io/apimachinery/pkg/runtime"
 "k8s.io/apimachinery/pkg/runtime/schema"
 "k8s.io/apimachinery/pkg/util/managedfields"
 "k8s.io/apimachinery/pkg/util/strategicpatch"
 "sigs.k8s.io/yaml"
)
func box(v any) any { if v==nil{return rt.NoneOf()};if o,ok:=v.(runtime.Object);ok{out:=deps.From(reflect.ValueOf(&o).Elem());inner,_:=rt.OptionValue(out);return inner};out:=deps.From(reflect.ValueOf(v));if reflect.TypeOf(v).Kind()==reflect.Pointer||reflect.TypeOf(v).Kind()==reflect.Interface {if inner,ok:=rt.OptionValue(out);ok{return inner}};return out }
func errOption(e error) any {if e==nil{return rt.NoneOf()};var v error=e;out:=deps.From(reflect.ValueOf(&v).Elem());if inner,ok:=rt.OptionValue(out);ok{return rt.SomeOf(inner)};return rt.SomeOf(out)}
func obj(v any,c context.Context) runtime.Object {return deps.To(reflect.TypeFor[runtime.Object](),rt.SomeOf(v),c).Interface().(runtime.Object)}
func scheme(v any,c context.Context) *runtime.Scheme {return deps.To(reflect.TypeFor[*runtime.Scheme](),rt.SomeOf(v),c).Interface().(*runtime.Scheme)}
func gvk(v any,c context.Context) schema.GroupVersionKind {return deps.To(reflect.TypeFor[schema.GroupVersionKind](),v,c).Interface().(schema.GroupVersionKind)}
func gvr(v any,c context.Context) schema.GroupVersionResource {return deps.To(reflect.TypeFor[schema.GroupVersionResource](),v,c).Interface().(schema.GroupVersionResource)}
func objectResult(v runtime.Object,e error) any {if v==nil{return rt.Tuple2Of(rt.NoneOf(),errOption(e))};return rt.Tuple2Of(rt.SomeOf(box(v)),errOption(e))}
func list(v []any) any {out:=rt.Shapes.Nil();for i:=len(v)-1;i>=0;i--{out=rt.Shapes.Cons(v[i],out)};return out}
func stringsFrom(v any) []string {var out []string;for {a,b,ok:=rt.Shapes.Uncons(v);if !ok{return out};out=append(out,a.(string));v=b}}
func JoinPath(v any) any {return path.Join(stringsFrom(v)...)}
func ParseLabels(v string) rt.IO {return func(context.Context)(any,error){s,e:=labels.Parse(v);if e!=nil{return rt.Tuple2Of("",errOption(e)),nil};return rt.Tuple2Of(s.String(),rt.NoneOf()),nil}}
func ParseFields(v string) rt.IO {return func(context.Context)(any,error){s,e:=fields.ParseSelector(v);if e!=nil{return rt.Tuple2Of("",errOption(e)),nil};return rt.Tuple2Of(s.String(),rt.NoneOf()),nil}}
func LabelSelector(v string) rt.IO {return func(context.Context)(any,error){s,e:=labels.Parse(v);if e!=nil{return nil,e};out:=deps.From(reflect.ValueOf(&s).Elem());inner,_:=rt.OptionValue(out);return inner,nil}}
func FieldSelector(v string) rt.IO {return func(context.Context)(any,error){s,e:=fields.ParseSelector(v);if e!=nil{return nil,e};out:=deps.From(reflect.ValueOf(&s).Elem());inner,_:=rt.OptionValue(out);return inner,nil}}
func MatchesLabels(selector string,v any) rt.IO {return func(c context.Context)(any,error){s,e:=labels.Parse(selector);if e!=nil{return nil,e};m,e:=meta.Accessor(obj(v,c));if e!=nil{return false,nil};return s.Matches(labels.Set(m.GetLabels())),nil}}
func ObjectIdentity(v any) rt.IO {return func(c context.Context)(any,error){m,e:=meta.Accessor(obj(v,c));if e!=nil{return rt.Tuple2Of(rt.Tuple2Of("",""),errOption(e)),nil};return rt.Tuple2Of(rt.Tuple2Of(m.GetNamespace(),m.GetName()),rt.NoneOf()),nil}}
func SetNamespace(v any,ns string) rt.IO {return func(c context.Context)(any,error){o:=obj(v,c).DeepCopyObject();m,e:=meta.Accessor(o);if e==nil{m.SetNamespace(ns)};return objectResult(o,e),nil}}
func SetName(v any,name string) rt.IO {return func(c context.Context)(any,error){o:=obj(v,c).DeepCopyObject();m,e:=meta.Accessor(o);if e==nil{m.SetName(name)};return objectResult(o,e),nil}}
func SetKind(v,k any) rt.IO {return func(c context.Context)(any,error){o:=obj(v,c).DeepCopyObject();o.GetObjectKind().SetGroupVersionKind(gvk(k,c));return box(o),nil}}
func NewObject(s,k any) rt.IO {return func(c context.Context)(any,error){o,e:=scheme(s,c).New(gvk(k,c));return objectResult(o,e),nil}}
func ObjectKinds(s,v any) rt.IO {return func(c context.Context)(any,error){ks,_,e:=scheme(s,c).ObjectKinds(obj(v,c));out:=make([]any,len(ks));for i,k:=range ks{out[i]=box(k)};return rt.Tuple2Of(list(out),errOption(e)),nil}}
func PartialMetadataKind(v any) rt.IO {return func(c context.Context)(any,error){o,ok:=obj(v,c).(*metav1.PartialObjectMetadata);if !ok{return rt.NoneOf(),nil};return rt.SomeOf(rt.Tuple2Of(o.APIVersion,box(o.GroupVersionKind()))),nil}}
func GuessResource(k any) any {r,_:=meta.UnsafeGuessKindToResource(gvk(k,context.Background()));return box(r)}
func IsList(v any) rt.IO {return func(c context.Context)(any,error){return meta.IsListType(obj(v,c)),nil}}
func ExtractList(v,d any) rt.IO {return func(c context.Context)(any,error){items,e:=meta.ExtractList(obj(v,c));if e!=nil{return rt.Tuple2Of(rt.Shapes.Nil(),errOption(e)),nil};decoder:=deps.To(reflect.TypeFor[runtime.Decoder](),rt.SomeOf(d),c).Interface().(runtime.Decoder);errs:=runtime.DecodeList(items,decoder);if len(errs)>0{return rt.Tuple2Of(rt.Shapes.Nil(),errOption(errs[0])),nil};out:=make([]any,len(items));for i,o:=range items{out[i]=box(o)};return rt.Tuple2Of(list(out),rt.NoneOf()),nil}}
func FillList(v any,rv string,items any) rt.IO {return func(c context.Context)(any,error){o:=obj(v,c);var values []runtime.Object;for{a,b,ok:=rt.Shapes.Uncons(items);if !ok{break};values=append(values,obj(a,c));items=b};if m,e:=meta.ListAccessor(o);e==nil{m.SetResourceVersion(rv)};e:=meta.SetList(o,values);if e!=nil{return objectResult(nil,e),nil};return objectResult(o.DeepCopyObject(),nil),nil}}
func NotFound(r any,name string) any {return errorBox(apierrors.NewNotFound(gvr(r,context.Background()).GroupResource(),name))}
func AlreadyExists(r any,name string) any {return errorBox(apierrors.NewAlreadyExists(gvr(r,context.Background()).GroupResource(),name))}
func BadRequest(message string) any {return errorBox(apierrors.NewBadRequest(message))}
func errorBox(e error) any {v,_:=rt.OptionValue(errOption(e));return v}
func IsNotFound(e any) bool {v:=deps.To(reflect.TypeFor[error](),rt.SomeOf(e),context.Background()).Interface().(error);return apierrors.IsNotFound(v)}
func SameObjectType(a,b any) bool {return reflect.TypeOf(obj(a,context.Background()))==reflect.TypeOf(obj(b,context.Background()))}
func MarshalObject(v any) rt.IO {return func(c context.Context)(any,error){data,e:=json.Marshal(obj(v,c));return rt.Tuple2Of(string(data),errOption(e)),nil}}
func UnmarshalObject(v any,data string) rt.IO {return func(c context.Context)(any,error){o:=obj(v,c);dst:=reflect.New(reflect.TypeOf(o).Elem()).Interface().(runtime.Object);e:=json.Unmarshal([]byte(data),dst);return objectResult(dst,e),nil}}
func JSONPatch(data,patch string) rt.IO {return func(context.Context)(any,error){p,e:=jsonpatch.DecodePatch([]byte(patch));if e!=nil{return rt.Tuple2Of("",errOption(e)),nil};out,e:=p.Apply([]byte(data));return rt.Tuple2Of(string(out),errOption(e)),nil}}
func MergePatch(data,patch string) rt.IO {return func(context.Context)(any,error){out,e:=jsonpatch.MergePatch([]byte(data),[]byte(patch));return rt.Tuple2Of(string(out),errOption(e)),nil}}
func StrategicPatch(data,patch string,v any) rt.IO {return func(c context.Context)(any,error){out,e:=strategicpatch.StrategicMergePatch([]byte(data),[]byte(patch),obj(v,c));return rt.Tuple2Of(string(out),errOption(e)),nil}}
func ApplyObject(data string) rt.IO {return func(context.Context)(any,error){o:=&unstructured.Unstructured{Object:map[string]interface{}{}};e:=yaml.Unmarshal([]byte(data),&o.Object);return objectResult(o,e),nil}}
func ParseInt(v string) any {n,e:=strconv.ParseInt(v,10,64);return rt.Tuple2Of(n,errOption(e))}
func MappedKind(s,r any) rt.IO {return func(c context.Context)(any,error){k,e:=testrestmapper.TestOnlyStaticRESTMapper(scheme(s,c)).KindFor(gvr(r,c));if e!=nil{return rt.Tuple2Of(rt.NoneOf(),errOption(e)),nil};return rt.Tuple2Of(rt.SomeOf(box(k)),rt.NoneOf()),nil}}
func FieldManager(s,t,k,d any) rt.IO {return func(c context.Context)(any,error){sc:=scheme(s,c);tc:=deps.To(reflect.TypeFor[managedfields.TypeConverter](),rt.SomeOf(t),c).Interface().(managedfields.TypeConverter);defaulter:=deps.To(reflect.TypeFor[runtime.ObjectDefaulter](),rt.SomeOf(d),c).Interface().(runtime.ObjectDefaulter);kind:=gvk(k,c);m,e:=managedfields.NewDefaultFieldManager(tc,sc,defaulter,sc,kind,kind.GroupVersion(),"",nil);if e!=nil{return rt.Tuple2Of(rt.NoneOf(),errOption(e)),nil};return rt.Tuple2Of(rt.SomeOf(box(m)),rt.NoneOf()),nil}}
func FieldUpdate(m,l,o any,name string) rt.IO {return func(c context.Context)(any,error){mgr:=deps.To(reflect.TypeFor[*managedfields.FieldManager](),rt.SomeOf(m),c).Interface().(*managedfields.FieldManager);out,e:=mgr.Update(obj(l,c),obj(o,c),name);return objectResult(out,e),nil}}
func FieldApply(m,l,o any,name string,force bool) rt.IO {return func(c context.Context)(any,error){mgr:=deps.To(reflect.TypeFor[*managedfields.FieldManager](),rt.SomeOf(m),c).Interface().(*managedfields.FieldManager);out,e:=mgr.Apply(obj(l,c),obj(o,c),name,force);return objectResult(out,e),nil}}
