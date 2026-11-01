// Copyright 2026 The Kubernetes Authors. Licensed under Apache-2.0.
package host

import (
 "context"
 "testing"
 "go4t/rt"
 "k8s.io/apimachinery/pkg/runtime"
 "k8s.io/apimachinery/pkg/runtime/schema"
)

type testOption struct { value any; present bool }
func conversionShapes(t *testing.T) {
 t.Helper()
 previous:=rt.Shapes
 t.Cleanup(func(){rt.Shapes=previous})
 rt.Shapes.Some=func(value any)any{return testOption{value,true}}
 rt.Shapes.None=func()any{return testOption{}}
 rt.Shapes.OptionValue=func(value any)(any,bool){v:=value.(testOption);return v.value,v.present}
}
func TestConversionsRetainValueAndPointerIdentity(t *testing.T) {
 conversionShapes(t)
 ctx:=context.Background()
 version:=schema.GroupVersion{Group:"example.test",Version:"v1"}
 if got:=from[schema.GroupVersion](ctx,box(version));got!=version{t.Fatalf("value conversion: %v",got)}
 scheme:=runtime.NewScheme()
 if got:=from[*runtime.Scheme](ctx,box(scheme));got!=scheme{t.Fatal("scheme pointer was copied")}
 var typer runtime.ObjectTyper=scheme
 if got:=from[runtime.ObjectTyper](ctx,box(typer));got!=typer{t.Fatal("interface pointer identity changed")}
 encoder:=runtime.WithVersionEncoder{Version:version}
 if got:=from[runtime.Encoder](ctx,box(runtime.Encoder(encoder)));got.(runtime.WithVersionEncoder).Version!=version{t.Fatal("value implementation was lost")}
 var nilScheme *runtime.Scheme
 var typedNil runtime.ObjectTyper=nilScheme
 if got:=from[runtime.ObjectTyper](ctx,box(typedNil));got!=typedNil || got==nil{t.Fatal("typed nil interface presence changed")}
 if _,present:=rt.OptionValue(box(nil));present{t.Fatal("nil interface became present")}
}
func TestJSONSerializerErasedABI(t *testing.T) {
 conversionShapes(t)
 // Mixed natural/erased arguments cannot implement the native extern ABI.
 var constructor func(any,any,any)any=JSONSerializer
 scheme:=runtime.NewScheme()
 effect,ok:=constructor(box(runtime.ObjectCreater(scheme)),box(runtime.ObjectTyper(scheme)),false).(rt.IO)
 if !ok{t.Fatal("serializer constructor did not return IO")}
 value,err:=effect(context.Background());if err!=nil{t.Fatal(err)}
 if from[runtime.Serializer](context.Background(),value)==nil{t.Fatal("serializer is absent")}
}
