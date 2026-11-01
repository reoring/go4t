// Copyright 2014 The Kubernetes Authors. Licensed under Apache-2.0.
// Generic serialization primitives only. Kubeconfig field policy is native gofx.
package host
import("context";"encoding/json";"gofx/rt";"sigs.k8s.io/yaml")
func SplitArray(text string) rt.IO {return func(context.Context)(any,error){var values []json.RawMessage;if err:=json.Unmarshal([]byte(text),&values);err!=nil{return nil,err};out:=rt.Shapes.Nil();for i:=len(values)-1;i>=0;i--{out=rt.Shapes.Cons(string(values[i]),out)};return out,nil}}
func YAMLToJSON(text string) rt.IO {return func(context.Context)(any,error){b,e:=yaml.YAMLToJSON([]byte(text));return string(b),e}}
func JSONToYAML(text string) rt.IO {return func(context.Context)(any,error){b,e:=yaml.JSONToYAML([]byte(text));return string(b),e}}
