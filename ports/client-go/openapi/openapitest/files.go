// Copyright 2023 The Kubernetes Authors. Licensed under Apache-2.0.
package openapitest
import (
 "context"
 "embed"
 "io/fs"
 "os"
 "go4t/rt"
 deps "github.com/reoring/gofx/ports/client-go/host/deps"
)
//go:embed testdata/*_openapi.json
var embedded embed.FS
func filesystem(path string,useEmbedded bool) fs.FS {if useEmbedded {value,err:=fs.Sub(embedded,"testdata");if err!=nil{panic(err)};return value};return os.DirFS(path)}
func Entries(path,useEmbedded any) any {return rt.IO(func(context.Context)(any,error){entries,err:=fs.ReadDir(filesystem(path.(string),useEmbedded.(bool)),".");list:=rt.Shapes.Nil();for i:=len(entries)-1;i>=0;i--{list=rt.Shapes.Cons(entries[i].Name(),list)};return rt.Tuple2Of(list,deps.ErrorOptionFromGo(err)),nil})}
func Read(path,name,useEmbedded any) any {return rt.IO(func(context.Context)(any,error){data,err:=fs.ReadFile(filesystem(path.(string),useEmbedded.(bool)),name.(string));return rt.Tuple2Of(string(data),deps.ErrorOptionFromGo(err)),nil})}
