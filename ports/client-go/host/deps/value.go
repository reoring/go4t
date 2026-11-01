// Package hostdeps bridges external dependency values. It contains no client-go policy.
package hostdeps

import (
 "context"
 "fmt"
 "reflect"
 "strings"
 "sync"

 "gofx/rt"
)

// Value owns an immutable external value, or an explicitly effectful host capability.
// Its fields are private so native callers cannot forge a differently typed value.
type Value struct { value reflect.Value }
var registry = struct { sync.RWMutex; types map[string]reflect.Type; funcs map[string]reflect.Value }{types:map[string]reflect.Type{},funcs:map[string]reflect.Value{}}
var contextType = reflect.TypeFor[context.Context]()
var errorType = reflect.TypeFor[error]()
var modes = map[string]string{}
func RegisterMode(id, mode string) {modes[id]=mode}

func RegisterType(id string, t reflect.Type) { registry.Lock(); defer registry.Unlock(); if old:=registry.types[id]; old!=nil && old!=t { panic("hostdeps: duplicate type "+id) }; registry.types[id]=t }
func RegisterFunction(id string, f any) { registry.Lock(); defer registry.Unlock(); if _,ok:=registry.funcs[id];ok {panic("hostdeps: duplicate function "+id)}; registry.funcs[id]=reflect.ValueOf(f) }
func registered(id string) reflect.Type {registry.RLock();t:=registry.types[id];registry.RUnlock();if t==nil {panic("hostdeps: unregistered type "+id)};return t}
func typeID(t reflect.Type) string {for t.Kind()==reflect.Pointer{t=t.Elem()};n:=t.Name();if i:=strings.IndexByte(n,'[');i>=0 {n=n[:i]};if t.PkgPath()=="" {return n};return t.PkgPath()+"."+n}
func checked(v any,id string) reflect.Value {x,ok:=v.(Value);if !ok||!x.value.IsValid(){panic("hostdeps: invalid value for "+id)};if typeID(x.value.Type())==id{return x.value};registry.RLock();target:=registry.types[id];registry.RUnlock();if target==x.value.Type()||(target!=nil&&target.Kind()==reflect.Interface&&x.value.Type().Implements(target)){return x.value};panic(fmt.Sprintf("hostdeps: expected %s, got %s",id,x.value.Type()))}
func New(id string) any {t:=registered(id);if modes[id]=="identity"&&t.Kind()==reflect.Struct{return Value{reflect.New(t)}};return Value{reflect.New(t).Elem()}}
func Nil(v any) any {x:=v.(Value).value;switch x.Kind(){case reflect.Interface,reflect.Map,reflect.Slice,reflect.Func,reflect.Chan,reflect.Pointer:return x.IsNil()};return false}
func IsNil(v any) any {return Nil(v)}
func option(v any)(any,bool){return rt.OptionValue(v)}
func listValues(v any) []any {out:=[]any{};for{h,t,ok:=rt.Shapes.Uncons(v);if !ok{return out};out=append(out,h);v=t}}
func result(values []reflect.Value) any {if len(values)==0{return rt.Unit{}};if len(values)==1{return From(values[0])};return rt.Tuple2Of(From(values[0]),result(values[1:]))}
func resultValues(v any,n int) []any {if n==0{return nil};if n==1{return []any{v}};h,t:=rt.Shapes.Untuple2(v);return append([]any{h},resultValues(t,n-1)...)}

// From preserves named identities before lowering structural containers.
func From(v reflect.Value) any {
 if !v.IsValid(){return rt.NoneOf()}
 if v.Kind()==reflect.Interface {if v.IsNil(){return rt.NoneOf()};return rt.SomeOf(storedValue(v.Elem()))}
 if v.Type().Name()!=""&&v.Type().PkgPath()!="" {return storedValue(v)}
 switch v.Kind(){
 case reflect.Pointer:if v.IsNil(){return rt.NoneOf()};return rt.SomeOf(From(v.Elem()))
 case reflect.Bool:return v.Bool()
 case reflect.String:return v.String()
 case reflect.Int,reflect.Int8,reflect.Int16,reflect.Int32,reflect.Int64:return v.Int()
 case reflect.Uint,reflect.Uint64,reflect.Uintptr:return Value{reflect.ValueOf(v.Uint())}
 case reflect.Uint8,reflect.Uint16,reflect.Uint32:return int64(v.Uint())
 case reflect.Float32,reflect.Float64:return v.Float()
 case reflect.Complex64,reflect.Complex128:return rt.Tuple2Of(real(v.Complex()),imag(v.Complex()))
 case reflect.Slice:if v.IsNil(){return rt.NoneOf()};if v.Type().Elem().Kind()==reflect.Uint8 {return rt.SomeOf(string(v.Bytes()))};return rt.SomeOf(sequence(v))
 case reflect.Array:return sequence(v)
 case reflect.Struct:if v.NumField()==0{return rt.Unit{}};return Value{v}
 case reflect.Map,reflect.Chan:return Value{v}
 case reflect.Func:return callbackFrom(v)
 default:panic("hostdeps: unsupported external kind "+v.Kind().String())
 }
}
func sequence(v reflect.Value) any {r:=rt.Shapes.Nil();for i:=v.Len()-1;i>=0;i--{r=rt.Shapes.Cons(From(v.Index(i)),r)};return r}
func To(t reflect.Type,v any,ctx context.Context) reflect.Value {
 if t.Kind()==reflect.Interface {x,ok:=option(v);if !ok{return reflect.Zero(t)};s:=x.(Value).value;if s.Type().AssignableTo(t){r:=reflect.New(t).Elem();r.Set(s);return r};if s.Kind()!=reflect.Pointer&&reflect.PointerTo(s.Type()).Implements(t){var p reflect.Value;if s.CanAddr(){p=s.Addr()}else{p=reflect.New(s.Type());p.Elem().Set(s)};r:=reflect.New(t).Elem();r.Set(p);return r};panic(fmt.Sprintf("hostdeps: %s does not implement %s",s.Type(),t))}
 if x,ok:=v.(Value);ok {
  s:=x.value
  if s.Type().AssignableTo(t){return s}
  // Identity constructors retain *T storage. A by-value Go T parameter or
  // result consumes that exact pointee; Call/Set performs the Go value copy.
  // Interface conversion above deliberately retains the original pointer.
  if t.Kind()==reflect.Struct&&s.Kind()==reflect.Pointer&&!s.IsNil()&&s.Type().Elem()==t{return s.Elem()}
  if s.Kind()==reflect.Uint64&&(t.Kind()==reflect.Uint||t.Kind()==reflect.Uint64||t.Kind()==reflect.Uintptr){r:=reflect.New(t).Elem();if r.OverflowUint(s.Uint()){panic("hostdeps: unsigned integer out of range")};r.SetUint(s.Uint());return r}
  panic(fmt.Sprintf("hostdeps: value type %s is not %s",s.Type(),t))
 }
 if t.Kind()==reflect.Pointer {x,ok:=option(v);if !ok{return reflect.Zero(t)};if value,ok:=x.(Value);ok{if value.value.Type()==t{return value.value};if value.value.Type()==t.Elem()&&value.value.CanAddr()&&!snapshotType(t.Elem()){return value.value.Addr()}};r:=reflect.New(t.Elem());r.Elem().Set(To(t.Elem(),x,ctx));return r}
 r:=reflect.New(t).Elem()
 switch t.Kind(){
 case reflect.Bool:r.SetBool(v.(bool))
 case reflect.String:r.SetString(v.(string))
 case reflect.Int,reflect.Int8,reflect.Int16,reflect.Int32,reflect.Int64:n:=v.(int64);if r.OverflowInt(n){panic("hostdeps: integer out of range for "+t.String())};r.SetInt(n)
 case reflect.Uint,reflect.Uint64,reflect.Uintptr:n:=v.(Value).value.Uint();if r.OverflowUint(n){panic("hostdeps: unsigned integer out of range")};r.SetUint(n)
 case reflect.Uint8,reflect.Uint16,reflect.Uint32:n:=v.(int64);if n<0||r.OverflowUint(uint64(n)){panic("hostdeps: unsigned integer out of range")};r.SetUint(uint64(n))
 case reflect.Float32,reflect.Float64:n:=v.(float64);if r.OverflowFloat(n){panic("hostdeps: float out of range")};r.SetFloat(n)
 case reflect.Complex64,reflect.Complex128:a,b:=rt.Shapes.Untuple2(v);r.SetComplex(complex(a.(float64),b.(float64)))
 case reflect.Slice:x,ok:=option(v);if !ok{return r};if t.Elem().Kind()==reflect.Uint8 {r.SetBytes([]byte(x.(string)));return r};xs:=listValues(x);r.Set(reflect.MakeSlice(t,len(xs),len(xs)));for i,x:=range xs{r.Index(i).Set(To(t.Elem(),x,ctx))}
 case reflect.Array:xs:=listValues(v);if len(xs)!=t.Len(){panic("hostdeps: wrong array length")};for i,x:=range xs{r.Index(i).Set(To(t.Elem(),x,ctx))}
 case reflect.Func:return callbackTo(t,v,ctx)
 case reflect.Struct:if t.NumField()!=0{panic("hostdeps: expected anonymous struct snapshot")};return r
 default:panic("hostdeps: expected typed snapshot for "+t.String())
 };return r
}
func Field(v any,id string,path []int) any {x:=checked(v,id);for x.Kind()==reflect.Pointer{x=x.Elem()};for _,i:=range path{if x.Kind()==reflect.Pointer {if x.IsNil(){panic("hostdeps: nil embedded pointer")};x=x.Elem()};x=x.Field(i);if !x.CanInterface(){panic("hostdeps: inaccessible field")}};return From(x)}
func WithField(v any,id string,path []int,field any) any {x:=checked(v,id);r:=structuralCopy(x);cur:=r;for n,i:=range path{if cur.Kind()==reflect.Pointer{p:=reflect.New(cur.Type().Elem());if !cur.IsNil(){p.Elem().Set(cur.Elem())};cur.Set(p);cur=cur.Elem()};cur=cur.Field(i);if !cur.CanSet(){panic("hostdeps: inaccessible field")};if n==len(path)-1{cur.Set(To(cur.Type(),field,context.Background()))}};return Value{r}}
func Underlying(v any,id string) any {x:=checked(v,id);t:=x.Type();switch t.Kind(){case reflect.Bool:return x.Bool();case reflect.String:return x.String();case reflect.Int,reflect.Int8,reflect.Int16,reflect.Int32,reflect.Int64:return x.Int();case reflect.Uint,reflect.Uint64,reflect.Uintptr:return Value{reflect.ValueOf(x.Uint())};case reflect.Uint8,reflect.Uint16,reflect.Uint32:return int64(x.Uint());case reflect.Float32,reflect.Float64:return x.Float();case reflect.Slice:if x.IsNil(){return rt.NoneOf()};if t.Elem().Kind()==reflect.Uint8{return rt.SomeOf(string(x.Bytes()))};return rt.SomeOf(sequence(x));case reflect.Map,reflect.Chan:return Value{x};case reflect.Array:return sequence(x)};panic("hostdeps: no structural underlying conversion for "+id)}
func FromUnderlying(id string,v any) any {t:=registered(id);base:=t;switch t.Kind(){case reflect.Bool:base=reflect.TypeFor[bool]();case reflect.String:base=reflect.TypeFor[string]();case reflect.Int,reflect.Int8,reflect.Int16,reflect.Int32,reflect.Int64:base=reflect.TypeFor[int64]();case reflect.Uint,reflect.Uint64,reflect.Uintptr:base=reflect.TypeFor[uint64]();case reflect.Uint8,reflect.Uint16,reflect.Uint32:base=reflect.TypeFor[uint32]();case reflect.Float32,reflect.Float64:base=reflect.TypeFor[float64]();case reflect.Map:base=t;case reflect.Slice:base=reflect.SliceOf(t.Elem());case reflect.Array:base=reflect.ArrayOf(t.Len(),t.Elem())};x:=To(base,v,context.Background());r:=reflect.New(t).Elem();if x.Kind()>=reflect.Int&&x.Kind()<=reflect.Int64 {if r.OverflowInt(x.Int()){panic("hostdeps: named integer out of range")}};if x.Kind()>=reflect.Uint&&x.Kind()<=reflect.Uintptr {if r.OverflowUint(x.Uint()){panic("hostdeps: named unsigned integer out of range")}};r.Set(x.Convert(t));return Value{r}}
func UInt64(high,low any) any {h,l:=high.(int64),low.(int64);if h<0||h>0xffffffff||l<0||l>0xffffffff{panic("hostdeps: UInt64 component outside 0..4294967295")};return Value{reflect.ValueOf(uint64(h)<<32|uint64(l))}}
func UInt64High(v any) any{return int64(v.(Value).value.Uint()>>32)}
func UInt64Low(v any) any{return int64(v.(Value).value.Uint()&0xffffffff)}
func MapEmpty(v any) any {x:=v.(Value).value;if x.Kind()!=reflect.Map{panic("hostdeps: expected map")};return Value{reflect.MakeMap(x.Type())}}
func mapKeyFrom(v reflect.Value) any {if v.Kind()==reflect.Pointer&&v.Type().Name()==""{return Value{v}};return From(v)}
func mapKeyTo(t reflect.Type,v any) reflect.Value {if x,ok:=v.(Value);ok&&x.value.Type().AssignableTo(t){return x.value};return To(t,v,context.Background())}
func MapLookup(v,key any) any {x:=v.(Value).value;k:=mapKeyTo(x.Type().Key(),key);y:=x.MapIndex(k);if !y.IsValid(){return rt.NoneOf()};return rt.SomeOf(From(y))}
func MapEntries(v any) any {x:=v.(Value).value;out:=rt.Shapes.Nil();it:=x.MapRange();for it.Next(){out=rt.Shapes.Cons(rt.Tuple2Of(mapKeyFrom(it.Key()),From(it.Value())),out)};return out}
func MapWith(v,key,value any) any {x:=v.(Value).value;r:=reflect.MakeMapWithSize(x.Type(),x.Len()+1);it:=x.MapRange();for it.Next(){r.SetMapIndex(it.Key(),it.Value())};r.SetMapIndex(mapKeyTo(x.Type().Key(),key),To(x.Type().Elem(),value,context.Background()));return Value{r}}
func MapWithout(v,key any) any {x:=v.(Value).value;if x.IsNil(){return v};r:=reflect.MakeMapWithSize(x.Type(),x.Len());it:=x.MapRange();for it.Next(){r.SetMapIndex(it.Key(),it.Value())};r.SetMapIndex(mapKeyTo(x.Type().Key(),key),reflect.Value{});return Value{r}}
func ChannelReceive(v any) any{return rt.IO(func(ctx context.Context)(any,error){x:=v.(Value).value;chosen,y,ok:=reflect.Select([]reflect.SelectCase{{Dir:reflect.SelectRecv,Chan:x},{Dir:reflect.SelectRecv,Chan:reflect.ValueOf(ctx.Done())}});if chosen==1{return nil,ctx.Err()};if !ok{return rt.NoneOf(),nil};return rt.SomeOf(From(y)),nil})}
func ChannelSend(v,value any) any{return rt.IO(func(ctx context.Context)(any,error){x:=v.(Value).value;chosen,_,_:=reflect.Select([]reflect.SelectCase{{Dir:reflect.SelectSend,Chan:x,Send:To(x.Type().Elem(),value,ctx)},{Dir:reflect.SelectRecv,Chan:reflect.ValueOf(ctx.Done())}});if chosen==1{return nil,ctx.Err()};return rt.Unit{},nil})}
func ChannelClose(v any) any{return rt.IO(func(context.Context)(any,error){v.(Value).value.Close();return rt.Unit{},nil})}

func call(f reflect.Value,args []any,ctx context.Context) any {offset:=0;if f.Type().NumIn()>0&&f.Type().In(0)==contextType{offset=1};return callWithUpdates(f,args,mutableIndices(f.Type(),offset),ctx,newCopySession())}
func Function(id string,indices []int,args ...any) any{return rt.IO(func(ctx context.Context)(any,error){registry.RLock();f:=registry.funcs[id];registry.RUnlock();if !f.IsValid(){return nil,fmt.Errorf("hostdeps: unregistered function %s",id)};return callWithUpdates(f,args,indices,ctx,newCopySession()),nil})}
func Invoke(v any,indices []int,args ...any) any{return rt.IO(func(ctx context.Context)(any,error){return callWithUpdates(v.(Value).value,args,indices,ctx,newCopySession()),nil})}
func Method(v any,id,name string,identity bool,indices []int,args ...any) any {
 return rt.IO(func(ctx context.Context)(any,error){
  session:=newCopySession()
  x:=checked(v,id)
  if !identity&&!readOnlyDataMethod(id,name){var err error;x,err=snapshotReceiver(x,session,ctx);if err!=nil{return nil,err}}
  f:=x.MethodByName(name)
  if !f.IsValid()&&x.Kind()!=reflect.Pointer{if x.CanAddr(){x=x.Addr()}else{p:=reflect.New(x.Type());p.Elem().Set(x);x=p};f=x.MethodByName(name)}
  if !f.IsValid(){return nil,fmt.Errorf("hostdeps: missing method %s.%s",id,name)}
  out:=callWithUpdates(f,args,indices,ctx,session)
  if identity{return out,nil}
  registry.RLock();target:=registry.types[id];registry.RUnlock()
  if x.Kind()==reflect.Pointer&&(target==nil||target.Kind()!=reflect.Interface){x=x.Elem()}
  return rt.Tuple2Of(Value{x},out),nil
 })
}
func readOnlyDataMethod(id,name string) bool {
 switch id {
 case "k8s.io/apimachinery/pkg/runtime.Object":
  return name=="GetObjectKind"||name=="DeepCopyObject"
 case "k8s.io/apimachinery/pkg/runtime/schema.ObjectKind":
  return name=="GroupVersionKind"
 case "k8s.io/apimachinery/pkg/apis/meta/v1.Object","k8s.io/apimachinery/pkg/apis/meta/v1.ListInterface","k8s.io/apimachinery/pkg/apis/meta/v1.ShardedListInterface","k8s.io/apimachinery/pkg/api/meta.List","k8s.io/apimachinery/pkg/api/meta.Type":
  return strings.HasPrefix(name,"Get")
 }
 return false
}
func callbackFrom(f reflect.Value) any {n:=f.Type().NumIn();if n>0&&f.Type().In(0)==contextType{n--};ins:=make([]reflect.Type,n);for i:=range ins{ins[i]=reflect.TypeFor[any]()};t:=reflect.FuncOf(ins,[]reflect.Type{reflect.TypeFor[any]()},false);return reflect.MakeFunc(t,func(xs []reflect.Value)[]reflect.Value{args:=make([]any,len(xs));for i,x:=range xs{args[i]=x.Interface()};action:=rt.IO(func(ctx context.Context)(any,error){return call(f,args,ctx),nil});return []reflect.Value{reflect.ValueOf(any(action))}}).Interface()}
func callbackTo(t reflect.Type,v any,ctx context.Context) reflect.Value {f:=reflect.ValueOf(v);return reflect.MakeFunc(t,func(xs []reflect.Value)[]reflect.Value{callCtx:=ctx;offset:=0;if t.NumIn()>0&&t.In(0)==contextType{callCtx=xs[0].Interface().(context.Context);offset=1};args:=make([]reflect.Value,len(xs)-offset);for i,x:=range xs[offset:]{if t.IsVariadic()&&i==len(args)-1{args[i]=reflect.ValueOf(sequence(x))}else{args[i]=reflect.ValueOf(From(x))}};out:=f.Call(args)[0].Interface();value,err:=out.(rt.IO)(callCtx);if err!=nil{if t.NumOut()>0&&t.Out(t.NumOut()-1)==errorType {r:=make([]reflect.Value,t.NumOut());for i:=range r{r[i]=reflect.Zero(t.Out(i))};r[len(r)-1]=reflect.ValueOf(err);return r};panic(err)};indices:=mutableIndices(t,offset);if len(indices)>0{updates,results:=rt.Shapes.Untuple2(value);applyCallbackUpdates(callCtx,xs[offset:],indices,updates,t.IsVariadic());value=results};vals:=resultValues(value,t.NumOut());r:=make([]reflect.Value,len(vals));for i,x:=range vals{r[i]=To(t.Out(i),x,callCtx)};return r})}

// ProxyCall preserves all Go results and the executing callback's error identity.
func ProxyCall(ctx context.Context, callback any, args []reflect.Value, results []reflect.Type, variadic bool) []reflect.Value {
 in:=make([]reflect.Value,len(args));for i,a:=range args{if variadic&&i==len(args)-1{in[i]=reflect.ValueOf(sequence(a))}else{in[i]=reflect.ValueOf(From(a))}}
 x:=reflect.ValueOf(callback).Call(in)[0].Interface()
 v,err:=x.(rt.IO)(ctx)
 if err!=nil {if len(results)==0||results[len(results)-1]!=errorType{panic(err)};out:=make([]reflect.Value,len(results));for i,t:=range results{out[i]=reflect.Zero(t)};out[len(out)-1]=reflect.ValueOf(err);return out}
 indices:=[]int{};for i,a:=range args{if mutateType(a.Type()){indices=append(indices,i)}}
 if len(indices)>0{updates,results:=rt.Shapes.Untuple2(v);applyCallbackUpdates(ctx,args,indices,updates,variadic);v=results}
 values:=resultValues(v,len(results));out:=make([]reflect.Value,len(results));for i,t:=range results{out[i]=To(t,values[i],ctx)};return out
}

func IdentityField(v any,id string,path []int) any {return rt.IO(func(context.Context)(any,error){return Field(v,id,path),nil})}
func SetIdentityField(v any,id string,path []int,field any) any {
 return rt.IO(func(ctx context.Context)(any,error){
  x:=checked(v,id)
  // From(pointer) exposes an addressable view; it is the same identity, not a copy.
  if x.Kind()!=reflect.Pointer&&!x.CanAddr(){return nil,fmt.Errorf("hostdeps: identity field setter requires pointer for %s",id)}
  for _,i:=range path{
   // Dereference the owning container only. The final field may itself be a
   // nil pointer and must remain a settable slot for Some/None assignment.
   for x.Kind()==reflect.Pointer{
    if x.IsNil(){return nil,fmt.Errorf("hostdeps: inaccessible identity field")}
    x=x.Elem()
   }
   if x.Kind()!=reflect.Struct||i<0||i>=x.NumField(){return nil,fmt.Errorf("hostdeps: inaccessible identity field")}
   x=x.Field(i)
  }
  if !x.CanSet(){return nil,fmt.Errorf("hostdeps: inaccessible identity field")}
  x.Set(To(x.Type(),field,ctx))
  return rt.Unit{},nil
 })
}

// schema.Map's private fields are lazy lookup caches, not model data. Rebuild
// those caches after an immutable model update rather than copying sync.Once.
func structuralCopy(v reflect.Value) reflect.Value {r:=reflect.New(v.Type()).Elem();if typeID(v.Type())=="sigs.k8s.io/structured-merge-diff/v6/schema.Map"{for i:=range v.NumField(){if v.Type().Field(i).IsExported(){r.Field(i).Set(v.Field(i))}}}else{r.Set(v)};return r}

func As[T any](v reflect.Value) T {var zero T;if !v.IsValid(){return zero};switch v.Kind(){case reflect.Interface,reflect.Pointer,reflect.Func,reflect.Map,reflect.Slice,reflect.Chan:if v.IsNil(){return zero}};return v.Interface().(T)}

func NewIdentity(id string) any {return rt.IO(func(context.Context)(any,error){return New(id),nil})}

func ZeroLike(v any) any {t:=v.(Value).value.Type();if t.Kind()==reflect.Pointer{t=t.Elem()};if modes[typeID(t)]=="identity"{return Value{reflect.New(t)}};return Value{reflect.New(t).Elem()}}
func ZeroIdentityLike(v any) any {return rt.IO(func(context.Context)(any,error){return ZeroLike(v),nil})}
func UnderlyingLike(prototype,value any) any {t:=prototype.(Value).value.Type();return Value{To(t,value,context.Background())}}

func MapNewPointerKey(prototype,value any) any {return rt.IO(func(ctx context.Context)(any,error){key:=prototype.(Value).value.Type().Key();if key.Kind()!=reflect.Pointer{panic("hostdeps: expected pointer-keyed map")};p:=reflect.New(key.Elem());p.Elem().Set(To(key.Elem(),value,ctx));return Value{p},nil})}
func MapNilPointerKey(prototype any) any {key:=prototype.(Value).value.Type().Key();if key.Kind()!=reflect.Pointer{panic("hostdeps: expected pointer-keyed map")};return Value{reflect.Zero(key)}}
func PointerKeyValue(key any) any {p:=key.(Value).value;if p.Kind()!=reflect.Pointer{panic("hostdeps: expected pointer key")};if p.IsNil(){return rt.NoneOf()};return rt.SomeOf(From(p.Elem()))}
func PointerKeyEqual(left,right any) any {a,b:=left.(Value).value,right.(Value).value;return a.Type()==b.Type()&&a.Pointer()==b.Pointer()}
