package hostdeps

import (
 "context"
 "math/rand/v2"

 "gofx/rt"
)

func RandomInt(bound any) any {
 return rt.IO(func(ctx context.Context)(any,error){
  if err:=ctx.Err();err!=nil{return nil,err}
  return rand.Int64N(bound.(int64)),nil
 })
}
