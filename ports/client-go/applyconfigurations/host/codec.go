// Copyright The Kubernetes Authors. Licensed under Apache-2.0.
// Generic structural codecs and external managedfields adapters only.
package host

import (
 "context"
 _ "embed"
 "encoding/json"
 "fmt"
 "reflect"
 "sync"
 "go4t/rt"
 "github.com/reoring/gofx/ports/client-go/host/deps"
 kruntime "k8s.io/apimachinery/pkg/runtime"
 "k8s.io/apimachinery/pkg/runtime/serializer/cbor/direct"
 "k8s.io/apimachinery/pkg/util/managedfields"
 "sigs.k8s.io/structured-merge-diff/v6/typed"
)

const hostPath = "github.com/reoring/gofx/ports/client-go/applyconfigurations/host"
var records = map[reflect.Type]string{}
//go:embed schema.yaml
var schemaYAML string
var parserOnce sync.Once
var parserValue *typed.Parser
func parser() *typed.Parser {
 parserOnce.Do(func(){var err error;parserValue,err=typed.NewParser(typed.YAMLObject(schemaYAML));if err!=nil{panic(fmt.Sprintf("Failed to parse schema: %v",err))}})
 return parserValue
}

// toRecord follows generated field positions, never guesses JSON ownership.
// Named API dependencies remain canonical snapshots through hostdeps.
func toRecord(t reflect.Type,v any,ctx context.Context) reflect.Value {
 if name,ok:=records[t];ok {
  result:=reflect.New(t).Elem()
  for i:=range t.NumField() {result.Field(i).Set(toRecord(t.Field(i).Type,rt.ForeignRecordField(hostPath+"."+name,v,i),ctx))}
  return result
 }
 switch t.Kind(){
 case reflect.Pointer:
  item,present:=rt.OptionValue(v);if !present{return reflect.Zero(t)}
  result:=reflect.New(t.Elem());result.Elem().Set(toRecord(t.Elem(),item,ctx));return result
 case reflect.Slice:
  if t.Name()!=""{return hostdeps.To(t,v,ctx)}
  item,present:=rt.OptionValue(v);if !present{return reflect.Zero(t)}
  if t.Elem().Kind()==reflect.Uint8{return reflect.ValueOf([]byte(item.(string))).Convert(t)}
  length:=0;for cursor:=item;;{_,rest,ok:=rt.Shapes.Uncons(cursor);if !ok{break};length++;cursor=rest}
  result:=reflect.MakeSlice(t,length,length)
  for i:=range length {a,rest,_:=rt.Shapes.Uncons(item);result.Index(i).Set(toRecord(t.Elem(),a,ctx));item=rest};return result
 case reflect.Map:
  if t.Name()!=""{return hostdeps.To(t,v,ctx)}
  item,present:=rt.OptionValue(v);if !present{return reflect.Zero(t)}
  if t.Key().Name()!=""&&t.Key().PkgPath()!=""{return hostdeps.To(t,item,ctx)}
  result:=reflect.MakeMap(t);pairs:=rt.MapToList(item)
  for{pair,rest,ok:=rt.Shapes.Uncons(pairs);if !ok{break};key,value:=rt.Shapes.Untuple2(pair);result.SetMapIndex(toRecord(t.Key(),key,ctx),toRecord(t.Elem(),value,ctx));pairs=rest};return result
 }
 return hostdeps.To(t,v,ctx)
}
func fromRecord(v reflect.Value,compare any) any {
 t:=v.Type()
 if name,ok:=records[t];ok {fields:=make([]any,t.NumField());for i:=range fields{fields[i]=fromRecord(v.Field(i),compare)};return rt.ForeignRecord(hostPath+"."+name,fields)}
 switch t.Kind(){
 case reflect.Pointer:if v.IsNil(){return rt.NoneOf()};return rt.SomeOf(fromRecord(v.Elem(),compare))
 case reflect.Slice:
  if t.Name()!=""{return hostdeps.From(v)}
  if v.IsNil(){return rt.NoneOf()};if t.Elem().Kind()==reflect.Uint8{return rt.SomeOf(string(v.Bytes()))}
  result:=rt.Shapes.Nil();for i:=v.Len()-1;i>=0;i--{result=rt.Shapes.Cons(fromRecord(v.Index(i),compare),result)};return rt.SomeOf(result)
 case reflect.Map:
  if t.Name()!=""{return hostdeps.From(v)}
  if t.Key().Name()!=""&&t.Key().PkgPath()!=""{if v.IsNil(){return rt.NoneOf()};return rt.SomeOf(hostdeps.From(v))}
  if v.IsNil(){return rt.NoneOf()};pairs:=rt.Shapes.Nil();iter:=v.MapRange();for iter.Next(){pairs=rt.Shapes.Cons(rt.Tuple2Of(fromRecord(iter.Key(),compare),fromRecord(iter.Value(),compare)),pairs)};return rt.SomeOf(rt.MapFromList(pairs,compare))
 }
 return hostdeps.From(v)
}
func result(value any,err error) any {if err!=nil{return rt.Tuple2Of(value,hostdeps.From(reflect.ValueOf(&err).Elem()))};return rt.Tuple2Of(value,rt.NoneOf())}
func marshal(t reflect.Type,value any,cbor bool) any {return rt.IO(func(ctx context.Context)(any,error){
 v:=toRecord(t,value,ctx).Interface();var data []byte;var err error
 if cbor{data,err=direct.Marshal(v)}else{data,err=json.Marshal(v)}
 return result(string(data),err),nil
})}
func decode(t reflect.Type,value,compare any) any {return rt.IO(func(context.Context)(any,error){v:=reflect.New(t);err:=json.Unmarshal([]byte(value.(string)),v.Interface());if err!=nil{return result(rt.NoneOf(),err),nil};return result(rt.SomeOf(fromRecord(v.Elem(),compare)),nil),nil})}
func extract(t,objectType reflect.Type,object any,schema string,manager,subresource,compare any) any {return rt.IO(func(ctx context.Context)(any,error){
 objectValue:=hostdeps.To(objectType,object,ctx);p:=reflect.New(objectType);p.Elem().Set(objectValue);obj:=p.Interface().(kruntime.Object)
 destination:=reflect.New(t);err:=managedfields.ExtractInto(obj,parser().Type(schema),manager.(string),destination.Interface(),subresource.(string))
 if err!=nil{return result(rt.NoneOf(),err),nil};return result(rt.SomeOf(fromRecord(destination.Elem(),compare)),nil),nil
})}
func NewTypeConverter(scheme any) any {return rt.IO(func(ctx context.Context)(any,error){s:=hostdeps.To(reflect.TypeFor[*kruntime.Scheme](),rt.SomeOf(scheme),ctx).Interface().(*kruntime.Scheme);c:=managedfields.NewSchemeTypeConverter(s,parser());value,_:=rt.OptionValue(hostdeps.From(reflect.ValueOf(&c).Elem()));return value,nil})}
func Panic(message any) any {panic(message.(string))}
func Parser() any {return rt.IO(func(context.Context)(any,error){return hostdeps.From(reflect.ValueOf(*parser())),nil})}
func Bytes(values any) any {var b []byte;for {value,rest,ok:=rt.Shapes.Uncons(values);if !ok{break};v:=value.(int64);if v<0||v>255{panic("byte outside 0..255")};b=append(b,byte(v));values=rest};return string(b)}
func emptyMap(t reflect.Type) any {return hostdeps.From(reflect.MakeMap(t))}
