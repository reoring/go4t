// Copyright 2014 The Kubernetes Authors. Licensed under Apache-2.0.
// Standard-library parsing adapters only; no kubeconfig policy.
package host
import("context";"net/url";"reflect";"strconv";"time";"unicode";"fmt";"gofx/rt";"github.com/reoring/gofx/ports/client-go/host/deps")
func ParseURL(text string)rt.IO{return func(context.Context)(any,error){v,e:=url.Parse(text);return rt.Tuple2Of(hostdeps.From(reflect.ValueOf(v)),hostdeps.ErrorOptionFromGo(e)),nil}}
func ParseRequestURI(text string)rt.IO{return func(context.Context)(any,error){v,e:=url.ParseRequestURI(text);return rt.Tuple2Of(hostdeps.From(reflect.ValueOf(v)),hostdeps.ErrorOptionFromGo(e)),nil}}
func ParseInteger(text string)(int64,bool){v,e:=strconv.ParseInt(text,10,64);if e!=nil{return 0,false};return v,true}
func ParseDuration(text string)(int64,bool){v,e:=time.ParseDuration(text);if e!=nil{return 0,false};return int64(v),true}
func CharacterCategory(text string)string{for _,r:=range text{switch{case unicode.IsLetter(r):return "L";case unicode.IsMark(r):return "M";case unicode.IsNumber(r):return "N";case unicode.IsPunct(r):return "P";case unicode.IsSymbol(r):return "S";case unicode.IsSpace(r)&&!unicode.IsControl(r):return "Z";default:return "C"}};return ""}
func CharacterCode(text string)int64{for _,r:=range text{return int64(r)};return 0}
func CharacterHex(text string)string{for _,r:=range text{return fmt.Sprintf("%U",r)};return ""}
