// Copyright 2026 The Kubernetes Authors. Licensed under Apache-2.0.
package host

import (
 "context"
 "errors"
 "reflect"
 "testing"
 "gofx/rt"
 "github.com/reoring/gofx/ports/client-go/host/deps"
 kruntime "k8s.io/apimachinery/pkg/runtime"
)

type testOption struct {value any; present bool}
func installTestOptions(t *testing.T) {
 previous := rt.Shapes
 t.Cleanup(func(){rt.Shapes=previous})
 rt.Shapes.Some=func(value any) any {return testOption{value,true}}
 rt.Shapes.None=func() any {return testOption{}}
 rt.Shapes.OptionValue=func(value any)(any,bool){option:=value.(testOption);return option.value,option.present}
}
func TestObjectEqualityPreservesAbsentAndTypedNil(t *testing.T) {
 installTestOptions(t)
 absent:=rt.NoneOf()
 var object kruntime.Object=(*kruntime.Unknown)(nil)
 present:=hostdeps.From(reflect.ValueOf(&object).Elem())
 if !ObjectEqual(absent,absent) {t.Fatal("absent objects must compare equal")}
 if ObjectEqual(absent,present) {t.Fatal("present typed-nil object must not collapse to absence")}
 if !ObjectEqual(present,present) {t.Fatal("same typed-nil object must compare equal")}
 if got:=value[kruntime.Object](context.Background(),absent);got!=nil {t.Fatal("absent interface must decode to its typed zero")}
 if got:=value[kruntime.Object](context.Background(),present);got==nil {t.Fatal("typed-nil dynamic object identity must survive conversion")}
}
func TestObjectEqualityRejectsMalformedOption(t *testing.T) {
 installTestOptions(t)
 defer func(){if recover()==nil {t.Fatal("malformed bare object input must not be accepted as None")}}()
 ObjectEqual("not-an-option",rt.NoneOf())
}
func TestErrorBoundaryRetainsCauseAndSingleOptionLayer(t *testing.T) {
 installTestOptions(t)
 cause:=errors.New("fixture cause")
 boxed:=hostdeps.ErrorFromGo(cause)
 wrapped:=hostdeps.ErrorToGo(WrapError("fixture context",boxed))
 if !errors.Is(wrapped,cause) {t.Fatal("wrapped error lost original cause identity")}
 absent:=hostdeps.ErrorOptionFromGo(nil)
 if _,ok:=rt.OptionValue(absent);ok {t.Fatal("nil error must be absent")}
 present:=hostdeps.ErrorOptionFromGo(cause)
 value,ok:=rt.OptionValue(present)
 if !ok || hostdeps.ErrorToGo(value)!=cause {t.Fatal("error must have exactly one Option layer")}
}
