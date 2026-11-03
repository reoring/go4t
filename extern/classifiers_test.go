package extern

import (
	"strings"
	"testing"
)

func TestErrorClassifierShim(t *testing.T) {
	dir := bindingTestModule(t)
	writeBindingTestFile(t, dir, "fixture/fixture.go", errorClassifierFixture)
	loader := Loader{Dir: dir}
	pkg, err := loader.Load("example.test/bindings/fixture")
	if err != nil {
		t.Fatal(err)
	}
	if !HasErrorClassifier(pkg) {
		t.Fatal("classifier predicates were not discovered")
	}
	_, shim, _, err := BindWithShim(pkg, BindOptions{}, "example.test/bindings/shim", "shim")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Count(string(shim), "rt.RegisterErrorClassifier(") != 1 {
		t.Fatalf("expected one classifier registration:\n%s", shim)
	}
	writeBindingTestFile(t, dir, "shim/shim.go", string(shim))
	writeBindingTestFile(t, dir, "shim/classifiers_test.go", errorClassifierRuntimeTest)
	runBindingTestGo(t, dir, "test", "./shim")

	// A shim emitted into the bound package must call unqualified predicates.
	_, shim, _, err = BindWithShim(pkg, BindOptions{}, pkg.Path, pkg.Name)
	if err != nil {
		t.Fatal(err)
	}
	writeBindingTestFile(t, dir, "fixture/shim.go", string(shim))
	writeBindingTestFile(t, dir, "fixture/classifiers_test.go", `package fixture_test
import (
 "testing"
 "go4t/rt"
 f "example.test/bindings/fixture"
)
func TestSamePackageClassifier(t *testing.T) {
 got := rt.ClassifyError(f.NewError("not found")).(*rt.Failure)
 if got.Kind != rt.NotFound || got.Message != "not found" { t.Fatalf("classification: %#v", got) }
}
`)
	runBindingTestGo(t, dir, "test", "./fixture")
}

func TestNoErrorClassifierShim(t *testing.T) {
	dir := bindingTestModule(t)
	writeBindingTestFile(t, dir, "fixture/fixture.go", `package fixture
func Is(error) bool { return true }
func IsZero() bool { return true }
func IsText(string) bool { return true }
func IsNumber(error) int { return 1 }
func IsVariadic(...error) bool { return true }
func IsGeneric[T any](error) bool { return true }
func IsPair(error) (bool, bool) { return true, true }
type Flag bool
func IsFlag(error) Flag { return true }
var IsVariable = func(error) bool { return true }
type Checker struct{}
func (Checker) IsMethod(error) bool { return true }
`)
	loader := Loader{Dir: dir}
	pkg, err := loader.Load("example.test/bindings/fixture")
	if err != nil {
		t.Fatal(err)
	}
	if HasErrorClassifier(pkg) {
		t.Fatal("non-predicates were discovered as classifiers")
	}
	_, shim, _, err := BindWithShim(pkg, BindOptions{}, "example.test/bindings/shim", "shim")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(shim), "RegisterErrorClassifier") || strings.Contains(string(shim), "func init()") {
		t.Fatalf("non-predicates registered a classifier:\n%s", shim)
	}
	writeBindingTestFile(t, dir, "shim/shim.go", string(shim))
	runBindingTestGo(t, dir, "build", "./shim")
}

const errorClassifierFixture = `package fixture
import (
 "errors"
 "strings"
)
type statusError string
func (e statusError) Error() string { return string(e) }
func NewError(kind string) error { return statusError(kind) }
func matches(err error, kind string) bool {
 var status statusError
 return errors.As(err, &status) && strings.Contains(string(status), kind)
}
func IsMissing(err error) bool { return matches(err, "missing") }
func IsBusy(err error) bool { return matches(err, "busy") }
func IsNotFound(err error) bool { return matches(err, "not found") }
func IsAlreadyExists(err error) bool { return matches(err, "exists") }
func IsForbidden(err error) bool { return matches(err, "forbidden") }
func IsUnauthorized(err error) bool { return matches(err, "unauthorized") }
func IsTimeout(err error) bool { return matches(err, "timeout") }
func IsServerTimeout(err error) bool { return matches(err, "server timeout") }
type ErrorAlias = error
type BoolAlias = bool
func IsAlias(err ErrorAlias) BoolAlias { return matches(err, "alias") }
`

const errorClassifierRuntimeTest = `package shim_test
import (
 "context"
 "errors"
 "fmt"
 "os"
 "testing"
 "go4t/rt"
 f "example.test/bindings/fixture"
 s "example.test/bindings/shim"
)
type result struct { value any; failed bool }
func TestClassifiers(t *testing.T) {
 rt.Shapes = rt.ShapeFuncs{
  IoError: func(kind, message string) any { return &rt.Failure{Kind: rt.Kind(kind), Message: message} },
  Err: func(value any) any { return result{value, true} },
  Ok: func(value any) any { return result{value, false} },
 }
 for _, tc := range []struct{ message string; kind rt.Kind; prefix string }{
  {"missing", rt.FailureKind, "Missing: "},
  {"busy", rt.FailureKind, "Busy: "},
  {"not found", rt.NotFound, ""},
  {"exists", rt.AlreadyExists, ""},
  {"forbidden", rt.PermissionDenied, ""},
  {"unauthorized", rt.PermissionDenied, ""},
  {"timeout", rt.TimeoutKind, ""},
  {"server timeout", rt.TimeoutKind, ""},
  {"alias", rt.FailureKind, "Alias: "},
  {"unrecognized", rt.FailureKind, ""},
  {"busy missing", rt.FailureKind, "Busy: "},
  {"busy missing not found exists forbidden timeout", rt.NotFound, ""},
  {"busy exists forbidden timeout", rt.AlreadyExists, ""},
  {"busy forbidden timeout", rt.PermissionDenied, ""},
  {"busy unauthorized timeout", rt.PermissionDenied, ""},
  {"busy timeout", rt.TimeoutKind, ""},
 } {
  t.Run(tc.message, func(t *testing.T) {
   for _, wrapped := range []bool{false, true} {
    action := s.NewError(tc.message)
    message := tc.message
    if wrapped {
     action = rt.IO(func(context.Context) (any, error) { return nil, fmt.Errorf("outer: %w", f.NewError(tc.message)) })
     message = "outer: " + message
    }
    value, err := rt.TryIO(action).(rt.IO)(context.Background())
    if err != nil { t.Fatal(err) }
    outcome := value.(result)
    failure, ok := outcome.value.(*rt.Failure)
    if !outcome.failed || !ok || failure.Kind != tc.kind || failure.Message != tc.prefix+message {
     t.Fatalf("TryIO (wrapped %v) = %#v, want %s %q", wrapped, outcome, tc.kind, tc.prefix+message)
    }
   }
  })
 }
 // Ordered registration falls through non-matches, and precedes OS defaults.
 calls := ""
 rt.RegisterErrorClassifier(func(err error) (rt.Kind, string, bool) {
  calls += "a"
  return rt.AlreadyExists, "first classifier", errors.Is(err, os.ErrNotExist)
 })
 rt.RegisterErrorClassifier(func(err error) (rt.Kind, string, bool) {
  calls += "b"
  return rt.PermissionDenied, "second classifier", errors.Is(err, os.ErrNotExist)
 })
 got := rt.ClassifyError(fmt.Errorf("wrapped: %w", os.ErrNotExist)).(*rt.Failure)
 if got.Kind != rt.AlreadyExists || got.Message != "first classifier" || calls != "a" {
  t.Fatalf("registration order/default precedence: %#v, calls %q", got, calls)
 }
 calls = ""
 got = rt.ClassifyError(os.ErrPermission).(*rt.Failure)
 if got.Kind != rt.PermissionDenied || got.Message != os.ErrPermission.Error() || calls != "ab" {
  t.Fatalf("default fallthrough: %#v, calls %q", got, calls)
 }
 calls = ""
 classified := &rt.Failure{Kind: rt.NotFound, Message: "already classified"}
 if rt.ClassifyError(classified) != classified || rt.ClassifyError(fmt.Errorf("outer: %w", classified)) != classified || rt.ClassifyError(nil) != nil || calls != "" {
  t.Fatalf("nil/Failure passthrough called classifiers: %q", calls)
 }
}
`
