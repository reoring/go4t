// Copyright The Kubernetes Authors. SPDX-License-Identifier: Apache-2.0
package host
import (
 "context"
 "crypto/sha256"
 "encoding/hex"
 "os"
 "reflect"
 "gofx/rt"
 deps "github.com/reoring/gofx/ports/client-go/host/deps"
)
func SHA256(value string)string{s:=sha256.Sum256([]byte(value));return string(s[:])}
func Hex(value string)string{return hex.EncodeToString([]byte(value))}
func ByteLength(value string)int64{return int64(len(value))}
func ByteSlice(value string,start,end int64)string{return value[start:end]}
func RemoveFile(path any)rt.IO{return func(ctx context.Context)(any,error){if e:=ctx.Err();e!=nil{return nil,e};e:=os.Remove(path.(string));if e==nil{return rt.NoneOf(),nil};return deps.From(reflect.ValueOf(&e).Elem()),nil}}
