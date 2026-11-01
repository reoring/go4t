// Copyright 2018 The Kubernetes Authors. Licensed under Apache-2.0.
// Generic JSON token and external-value codecs, not credential field policy.
package host
import("context";"encoding/base64";"encoding/json";"reflect";"go4t/rt";meta "k8s.io/apimachinery/pkg/apis/meta/v1";kruntime "k8s.io/apimachinery/pkg/runtime";"github.com/reoring/gofx/ports/client-go/host/deps")
func SplitObject(text string) rt.IO {return func(context.Context)(any,error){var fields map[string]json.RawMessage;if err:=json.Unmarshal([]byte(text),&fields);err!=nil{return nil,err};out:=rt.Shapes.Nil();for key,value:=range fields{out=rt.Shapes.Cons(rt.Tuple2Of(key,string(value)),out)};return out,nil}}
func StringValue(text string) rt.IO{return func(context.Context)(any,error){var value string;err:=json.Unmarshal([]byte(text),&value);return value,err}}
func BoolValue(text string) rt.IO{return func(context.Context)(any,error){var value bool;err:=json.Unmarshal([]byte(text),&value);return value,err}}
func EncodeBytes(text string) string{return base64.StdEncoding.EncodeToString([]byte(text))}
func DecodeBytes(text string) rt.IO{return func(context.Context)(any,error){b,e:=base64.StdEncoding.DecodeString(text);return string(b),e}}
func EncodeTime(v any) any{return rt.IO(func(ctx context.Context)(any,error){t:=value[meta.Time](ctx,v);b,e:=t.MarshalJSON();return string(b),e})}
func DecodeTime(text string) rt.IO{return func(context.Context)(any,error){var t meta.Time;if e:=t.UnmarshalJSON([]byte(text));e!=nil{return nil,e};return hostdeps.From(reflect.ValueOf(t)),nil}}
func EncodeRaw(v any) any{return rt.IO(func(ctx context.Context)(any,error){r:=raw(ctx,v);b,e:=r.MarshalJSON();return string(b),e})}
func DecodeRaw(text string) rt.IO{return func(context.Context)(any,error){var r kruntime.RawExtension;if e:=r.UnmarshalJSON([]byte(text));e!=nil{return nil,e};return hostdeps.From(reflect.ValueOf(r)),nil}}
func QuoteJSON(text string) string{b,_:=json.Marshal(text);return string(b)}
