package hostdeps_test

import (
 "errors"
 "fmt"
 "os"
 "testing"

 "go4t/rt"
 "github.com/reoring/gofx/ports/client-go/host/deps"
 apierrors "k8s.io/apimachinery/pkg/api/errors"
 "k8s.io/apimachinery/pkg/runtime/schema"
)

type valueError struct { code int }
func (e valueError) Error() string {return fmt.Sprintf("value error %d",e.code)}
type pointerError struct { code int }

type testOption struct { value any; some bool }
func installErrorShapes(t *testing.T) {
 t.Helper()
 previous:=rt.Shapes
 t.Cleanup(func(){rt.Shapes=previous})
 rt.Shapes.Some=func(value any)any{return testOption{value,true}}
 rt.Shapes.None=func()any{return testOption{}}
 rt.Shapes.OptionValue=func(value any)(any,bool){option:=value.(testOption);return option.value,option.some}
}
func (e *pointerError) Error() string {return fmt.Sprintf("pointer error %d",e.code)}

func TestErrorBridge(t *testing.T) {
 installErrorShapes(t)
 pointer:=&pointerError{code:7}
 value:=valueError{code:9}
 wrapped:=fmt.Errorf("opening configuration: %w",os.ErrNotExist)
 status:=apierrors.NewNotFound(schema.GroupResource{Group:"apps",Resource:"deployments"},"missing")
 wrappedStatus:=fmt.Errorf("request: %w",status)
 var nilPointer *pointerError
 cases:=[]struct{name string; input error}{
  {"pointer",pointer},{"value",value},{"wrapped filesystem",wrapped},
  {"API status",status},{"wrapped API status",wrappedStatus},{"typed nil",nilPointer},
 }
 for _,tc:=range cases {t.Run(tc.name,func(t *testing.T){
  boxed:=hostdeps.ErrorFromGo(tc.input)
  got:=hostdeps.ErrorToGo(boxed)
  if got!=tc.input {t.Fatal("bare error lost original dynamic value or identity")}
  optional,present:=rt.OptionValue(hostdeps.ErrorOptionFromGo(tc.input))
  if !present||hostdeps.ErrorToGo(optional)!=tc.input {t.Fatal("optional error lost presence or identity")}
  if hostdeps.ErrorIs(boxed,hostdeps.ErrorFromGo(tc.input))!=true {t.Fatal("native errors.Is bridge changed identity")}
 })}
 if _,present:=rt.OptionValue(hostdeps.ErrorOptionFromGo(nil));present {t.Fatal("nil error must be None")}
 if !errors.Is(hostdeps.ErrorToGo(hostdeps.ErrorFromGo(wrapped)),os.ErrNotExist) {t.Fatal("wrapped sentinel lost")}
 var recoveredPointer *pointerError
 if !errors.As(hostdeps.ErrorToGo(hostdeps.ErrorFromGo(pointer)),&recoveredPointer)||recoveredPointer!=pointer {t.Fatal("pointer errors.As lost")}
 var recoveredValue valueError
 if !errors.As(hostdeps.ErrorToGo(hostdeps.ErrorFromGo(value)),&recoveredValue)||recoveredValue!=value {t.Fatal("value errors.As lost")}
 var recoveredStatus apierrors.APIStatus
 if !errors.As(hostdeps.ErrorToGo(hostdeps.ErrorFromGo(wrappedStatus)),&recoveredStatus)||recoveredStatus!=status {t.Fatal("APIStatus errors.As lost")}
 if recoveredStatus.Status().Details.Name!="missing" {t.Fatal("APIStatus details lost")}
 if !apierrors.IsNotFound(hostdeps.ErrorToGo(hostdeps.ErrorFromGo(wrappedStatus))) {t.Fatal("wrapped API predicate changed")}
}

func TestErrorBridgeRejectsMalformedShapes(t *testing.T) {
 installErrorShapes(t)
 t.Run("nil Go interface",func(t *testing.T){
  defer func(){if got:=recover();got!="hostdeps: cannot box nil error" {t.Fatalf("unexpected panic: %v",got)}}()
  hostdeps.ErrorFromGo(nil)
 })
 boxed:=hostdeps.ErrorFromGo(os.ErrNotExist)
 cases:=[]struct{name string; value any}{
  {"nil",nil},{"message","not a canonical error"},{"None",rt.NoneOf()},
  {"Some",rt.SomeOf(boxed)},{"nested Some",rt.SomeOf(rt.SomeOf(boxed))},
 }
 for _,tc:=range cases {t.Run(tc.name,func(t *testing.T){
  defer func(){if recover()==nil {t.Fatal("malformed shape accepted as bare Error")}}()
  hostdeps.ErrorToGo(tc.value)
 })}
}
