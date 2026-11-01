package hostdeps

import (
 "context"
 "reflect"

 "gofx/rt"
)

type visit struct { typ reflect.Type; ptr uintptr }
type copySession struct { seen map[visit]reflect.Value; boxed map[reflect.Value]reflect.Value }
func newCopySession() *copySession {return &copySession{}}
func (s *copySession) remember(key visit,value reflect.Value) {if s.seen==nil{s.seen=make(map[visit]reflect.Value)};s.seen[key]=value}
func snapshotType(t reflect.Type) bool {for t.Kind()==reflect.Pointer{t=t.Elem()};mode:=modes[typeID(t)];if mode=="identity"||(mode==""&&t.PkgPath()!=""){return false};return true}
func (s *copySession) clone(v reflect.Value) reflect.Value {
 if !v.IsValid(){return v}
 if v.Kind()==reflect.Interface{if v.IsNil(){return reflect.Zero(v.Type())};r:=reflect.New(v.Type()).Elem();r.Set(s.clone(v.Elem()));return r}
 if !snapshotType(v.Type()){return v}
 switch v.Kind(){
 case reflect.Pointer:
  if v.IsNil(){return reflect.Zero(v.Type())};key:=visit{v.Type(),v.Pointer()};if r,ok:=s.seen[key];ok{return r};r:=reflect.New(v.Type().Elem());s.remember(key,r);s.cloneInto(r.Elem(),v.Elem());return r
 case reflect.Struct:
  r:=reflect.New(v.Type()).Elem();s.cloneInto(r,v);return r
 case reflect.Slice:
  if v.IsNil(){return reflect.Zero(v.Type())};key:=visit{v.Type(),v.Pointer()};if r,ok:=s.seen[key];ok&&r.Cap()>=v.Len(){return r.Slice(0,v.Len())};r:=reflect.MakeSlice(v.Type(),v.Cap(),v.Cap());s.remember(key,r);full:=v.Slice(0,v.Cap());for i:=range full.Len(){r.Index(i).Set(s.clone(full.Index(i)))};return r.Slice(0,v.Len())
 case reflect.Map:
  if v.IsNil(){return reflect.Zero(v.Type())};key:=visit{v.Type(),uintptr(v.UnsafePointer())};if r,ok:=s.seen[key];ok{return r};r:=reflect.MakeMapWithSize(v.Type(),v.Len());s.remember(key,r);it:=v.MapRange();for it.Next(){r.SetMapIndex(it.Key(),s.clone(it.Value()))};return r
 case reflect.Array:
  r:=reflect.New(v.Type()).Elem();for i:=range v.Len(){r.Index(i).Set(s.clone(v.Index(i)))};return r
 default:return v
 }
}
func (s *copySession) cloneInto(dst,src reflect.Value) {
 if src.CanAddr()&&dst.CanAddr(){s.remember(visit{src.Addr().Type(),src.Addr().Pointer()},dst.Addr())}
 if src.Kind()!=reflect.Struct {dst.Set(s.clone(src));return}
 dst.Set(structuralCopy(src))
 // Quantity and FieldsV1 have private mutable storage and an external deep-copy
 // contract. Public model fields are recursively copied with this session.
 hasPrivate:=false;hasPublic:=false
 for i:=range src.NumField(){if src.Type().Field(i).IsExported(){hasPublic=true}else{hasPrivate=true}}
 if hasPrivate&&!hasPublic&&src.CanAddr(){m:=src.Addr().MethodByName("DeepCopy");if m.IsValid()&&m.Type().NumIn()==0&&m.Type().NumOut()==1{v:=m.Call(nil)[0];if v.Kind()==reflect.Pointer{v=v.Elem()};if v.Type()==src.Type(){dst.Set(v);return}}}
 for i:=range src.NumField(){if src.Type().Field(i).IsExported(){dst.Field(i).Set(s.clone(src.Field(i)))}}
}
func (s *copySession) input(t reflect.Type,v any,ctx context.Context) reflect.Value {
 if t.Kind()==reflect.Interface {x,ok:=option(v);if !ok{return reflect.Zero(t)};raw:=s.clone(x.(Value).value);r:=reflect.New(t).Elem();if raw.Type().AssignableTo(t){r.Set(raw);return r};if raw.Kind()!=reflect.Pointer&&reflect.PointerTo(raw.Type()).Implements(t){var ptr reflect.Value;if raw.CanAddr(){ptr=raw.Addr()}else{ptr=reflect.New(raw.Type());ptr.Elem().Set(raw)};r.Set(ptr);return r};panic("hostdeps: updated input does not implement interface")}
 if t.Kind()==reflect.Pointer {x,ok:=option(v);if !ok{return reflect.Zero(t)};if value,ok:=x.(Value);ok {raw:=value.value;if raw.Type()==t{return s.clone(raw)};if raw.Type()==t.Elem(){var ptr reflect.Value;if raw.CanAddr(){ptr=raw.Addr()}else if cached,ok:=s.boxed[raw];ok {ptr=cached}else{ptr=reflect.New(raw.Type());ptr.Elem().Set(raw);if s.boxed==nil{s.boxed=make(map[reflect.Value]reflect.Value)};s.boxed[raw]=ptr};return s.clone(ptr)}};return s.clone(To(t,v,ctx))}
 if x,ok:=v.(Value);ok {if x.value.Type().AssignableTo(t){return s.clone(x.value)}}
 if t.Kind()==reflect.Slice {x,ok:=option(v);if !ok{return reflect.Zero(t)};if t.Elem().Kind()==reflect.Uint8{return To(t,v,ctx)};values:=listValues(x);r:=reflect.MakeSlice(t,len(values),len(values));for i,x:=range values{r.Index(i).Set(s.input(t.Elem(),x,ctx))};return r}
 return To(t,v,ctx)
}
func mutateType(t reflect.Type) bool {
 seen:=map[reflect.Type]bool{}
 var visit func(reflect.Type) bool
 visit=func(t reflect.Type) bool {if seen[t]{return false};seen[t]=true;switch t.Kind(){case reflect.Pointer:return snapshotType(t.Elem());case reflect.Slice,reflect.Map:return true;case reflect.Interface:return t.NumMethod()==0||typeID(t)=="k8s.io/apimachinery/pkg/runtime.Object"||typeID(t)=="k8s.io/apimachinery/pkg/apis/meta/v1.Object"||typeID(t)=="k8s.io/apimachinery/pkg/runtime/schema.ObjectKind";case reflect.Array:return visit(t.Elem());case reflect.Struct:if !snapshotType(t){return false};for i:=range t.NumField(){if visit(t.Field(i).Type){return true}}};return false}
 return visit(t)
}
func mutableIndices(t reflect.Type,offset int) []int {var out []int;for i:=offset;i<t.NumIn();i++{if mutateType(t.In(i)){out=append(out,i-offset)}};return out}
func packAny(values []any) any {if len(values)==0{return rt.Unit{}};if len(values)==1{return values[0]};return rt.Tuple2Of(values[0],packAny(values[1:]))}
func packUpdates(in []reflect.Value,indices []int,offset int,variadic bool) any {values:=make([]any,len(indices));for i,index:=range indices{if variadic&&index+offset==len(in)-1{values[i]=sequence(in[index+offset])}else{values[i]=From(in[index+offset])}};return packAny(values)}
func callWithUpdates(f reflect.Value,args []any,indices []int,ctx context.Context,session *copySession) any {
 t:=f.Type();offset:=0;in:=make([]reflect.Value,0,t.NumIn());if t.NumIn()>0&&t.In(0)==contextType{offset=1;in=append(in,reflect.ValueOf(ctx))};if len(args)!=t.NumIn()-offset{panic("hostdeps: wrong call arity")}
 next:=0
 for i,a:=range args{target:=t.In(i+offset);selected:=next<len(indices)&&indices[next]==i;if selected{next++};if t.IsVariadic()&&i==len(args)-1{values:=listValues(a);r:=reflect.MakeSlice(target,len(values),len(values));for j,v:=range values{r.Index(j).Set(session.input(target.Elem(),v,ctx))};in=append(in,r)}else if selected{in=append(in,session.input(target,a,ctx))}else{in=append(in,To(target,a,ctx))}}
 var out []reflect.Value;if t.IsVariadic(){out=f.CallSlice(in)}else{out=f.Call(in)}
 results:=result(out);if len(indices)==0{return results};return rt.Tuple2Of(packUpdates(in,indices,offset,t.IsVariadic()),results)
}
// ApplyCallbackUpdates writes only the locations a Go callback can mutate. A
// callback cannot replace a caller's slice header or pointer variable by value.
func applyCallbackUpdates(ctx context.Context,args []reflect.Value,indices []int,updated any,variadic bool) {
 values:=resultValues(updated,len(indices));session:=newCopySession()
 for i,index:=range indices{dst:=args[index];value:=values[i];if variadic&&index==len(args)-1{value=rt.SomeOf(value)};src:=session.input(dst.Type(),value,ctx);switch dst.Kind(){case reflect.Pointer:if dst.IsNil()!=src.IsNil(){panic("hostdeps: callback cannot replace pointer presence")};if !dst.IsNil(){dst.Elem().Set(src.Elem())};case reflect.Slice:if dst.Len()!=src.Len(){panic("hostdeps: callback cannot replace slice length")};reflect.Copy(dst,src);case reflect.Map:if dst.IsNil()&&!src.IsNil(){panic("hostdeps: callback cannot replace nil map")};dst.Clear();it:=src.MapRange();for it.Next(){dst.SetMapIndex(it.Key(),it.Value())};case reflect.Interface:if dst.IsNil(){if !src.IsNil(){panic("hostdeps: callback cannot replace nil interface")};continue};d:=dst.Elem();v:=src.Elem();if d.Kind()==reflect.Pointer&&!d.IsNil(){if v.Kind()!=reflect.Pointer||v.IsNil(){panic("hostdeps: callback changed interface pointer shape")};d.Elem().Set(v.Elem())}}}
}
