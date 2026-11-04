package extern

import (
	"strings"
	"testing"
)

func TestGoErrorHandleShim(t *testing.T) {
	var loader Loader
	pkg, err := loader.Load("go4t/extern/testdata/errorfixture")
	if err != nil {
		t.Fatal(err)
	}
	decls, shim, _, err := BindWithShim(pkg, BindOptions{}, "example.test/bindings/shim", "shim")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"type Report = Report{err: Option[GoError], n: Int}\n",
		"type Nested = Nested{reports: List[Report], errors: Map[String, Option[GoError]]}\n",
		"type GoErrorGo = GoErrorGo{n: Int} deriving Eq, Debug\n",
		"extern func classify(p0 GoError) String",
		"extern func isSentinel(p0 GoError) Bool",
		"extern func fail() IO[Unit]",
		"extern func wrap(p0 String) IO[Unit]",
		"extern func succeed() IO[Unit]",
		"extern func errs() List[Option[GoError]]",
		"extern func echoErrs(p0 List[Option[GoError]]) List[Option[GoError]]",
		"extern func errorMap() Map[String, Option[GoError]]",
		"extern func echoErrorMap(p0 Map[String, Option[GoError]]) Map[String, Option[GoError]]",
		"extern func echoReport(p0 Report) Report",
		"extern func echoNested(p0 Nested) Nested",
		"extern func echoError(p0 GoError) (Option[GoError], Int)",
		"extern func nonTrailing(p0 Bool) (Option[GoError], Int)",
		"extern func typedNil() (Option[GoError], Int)",
		"extern func invoke(p0 func(GoError) String) String",
		"extern func returned() func(GoError) String",
		"extern func variadic(p0 List[GoError]) String",
		"extern func invokeVariadic(p0 func(List[GoError]) String) String",
		"extern func sentinel() Option[GoError]",
	} {
		if !strings.Contains(decls, want) {
			t.Errorf("missing %q:\n%s", want, decls)
		}
	}
	dir := bindingTestModule(t)
	writeBindingTestFile(t, dir, "shim/shim.go", string(shim))
	writeBindingTestFile(t, dir, "shim/shim_test.go", goErrorHandleRuntimeTest)
	runBindingTestGo(t, dir, "test", "./shim")
}

func TestGoErrorHandleCause(t *testing.T) {
	dir := bindingTestModule(t)
	writeBindingTestFile(t, dir, "main.gt", goErrorHandleCauseProgram)
	// Exercise the actual io.cause accessor, generated bindings and compiled
	// program together, not just the equivalent erased runtime representation.
	runBindingTestGo(t, dir, "run", "-mod=mod", "go4t/cmd/go4t", "run", ".")
}

const goErrorHandleRuntimeTest = `package shim_test
import (
 "context"
 "errors"
 "testing"
 "go4t/rt"
 f "go4t/extern/testdata/errorfixture"
 s "example.test/bindings/shim"
)
type option struct { value any; present bool }
type result struct { value any; failed bool }
func TestGoErrorBoundary(t *testing.T) {
 rt.Shapes = rt.ShapeFuncs{
  Nil: func() any { return []any{} },
  Cons: func(h,t any) any { return append([]any{h},t.([]any)...) },
  Uncons: func(v any) (any,any,bool) { xs := v.([]any); if len(xs)==0 { return nil,nil,false }; return xs[0],xs[1:],true },
  Some: func(v any) any { return option{v,true} }, None: func() any { return option{} },
  OptionValue: func(v any) (any,bool) { x := v.(option); return x.value,x.present },
  Tuple2: func(a,b any) any { return [2]any{a,b} },
  Untuple2: func(v any) (any,any) { p := v.([2]any); return p[0],p[1] },
  IoError: func(kind,message string) any { return &rt.Failure{Kind: rt.Kind(kind),Message: message} },
  IoErrorOf: func(v any) (string,string) { e := v.(*rt.Failure); return string(e.Kind),e.Message },
  Ok: func(v any) any { return result{v,false} }, Err: func(v any) any { return result{v,true} },
 }
 for _,key := range []string{"go4t/rt.IoFailure","go4t/extern/testdata/errorfixture.Report","go4t/extern/testdata/errorfixture.Nested"} {
  rt.RegisterForeignRecord(key,func(v []any) any { return v },func(v any,i int) any { return v.([]any)[i] })
 }
 run := func(action any) any { t.Helper(); v,err := action.(rt.IO)(context.Background()); if err != nil { t.Fatal(err) }; return v }
 equal := func(got,want any) { t.Helper(); if got != want { t.Fatalf("got %#v, want %#v",got,want) } }
 equal(run(s.Succeed()),rt.Unit{})
 for _,action := range []any{s.Fail(),s.Wrap("outer")} {
  outcome := run(rt.TryIOCause(action)).(result)
  if !outcome.failed { t.Fatal("trailing error did not fail") }
  cause := outcome.value.([]any)[1].(option).value
  equal(s.Classify(cause),"sentinel")
  equal(s.IsSentinel(cause),true)
  if !errors.Is(cause.(error),f.Sentinel) { t.Fatal("cause lost wrapping chain") }
  equal(s.EchoError(cause).([2]any)[0],option{cause,true})
  again := run(rt.TryIOCause(rt.RaiseCause(outcome.value))).(result)
  equal(again.value.([]any)[1],option{cause,true})
 }
 equal(s.Sentinel(),option{f.Sentinel,true})
 nilReport := s.New_Report().([]any)
 equal(nilReport[0],option{}); equal(nilReport[1],int64(0))
 equal(s.EchoReport(nilReport).([]any)[0],option{})
 wrapped := f.Wrap("record")
 report := []any{option{wrapped,true},int64(9)}
 equal(s.EchoReport(report).([]any)[0],option{wrapped,true})
 equal(s.EchoReport(report).([]any)[1],int64(9))
 original := s.Errs().([]any)
 copied := s.EchoErrs(original).([]any)
 if len(copied)!=3 { t.Fatalf("errors = %#v",copied) }
 for i := range original { equal(copied[i],original[i]) }
 equal(copied[0],option{}); equal(copied[1],option{f.Sentinel,true})
 equal(s.Classify(copied[2].(option).value),"sentinel")
 values := s.ErrorMap()
 checkMap := func(v any) { t.Helper(); count := 0; rt.ForeignMapEach(v,func(k,v any) { count++; if k=="nil" { equal(v,option{}) } else if k=="sentinel" { equal(v,option{f.Sentinel,true}) } else { t.Fatalf("unexpected key %v",k) } }); equal(count,2) }
 checkMap(s.EchoErrorMap(values))
 nested := s.EchoNested([]any{[]any{report},values}).([]any)
 equal(nested[0].([]any)[0].([]any)[0],option{wrapped,true}); checkMap(nested[1])
 equal(s.NonTrailing(false).([2]any)[0],option{})
 equal(s.NonTrailing(true).([2]any)[0],option{f.Sentinel,true})
 typedNil := s.TypedNil().([2]any)[0].(option)
 if !typedNil.present || typedNil.value.(error)==nil { t.Fatal("typed nil error became None") }
 equal(s.EchoError(typedNil.value).([2]any)[0],typedNil)
 equal(s.Invoke(func(err any) any { equal(err,f.Sentinel); return s.Classify(err) }),"sentinel")
 equal(s.Returned().(func(any) any)(wrapped),"sentinel")
 equal(s.Variadic([]any{wrapped}),"sentinel")
 equal(s.InvokeVariadic(func(errs any) any { equal(errs.([]any)[0],f.Sentinel); return s.Variadic(errs) }),"sentinel")
 func() {
  defer func() { equal(recover(),"go4t error: nil error parameter") }()
  s.InvokeNil(func(any) any { t.Fatal("nil reached a GoError callback"); return "" })
 }()
}
`

const goErrorHandleCauseProgram = `package main
import "std/io"
import errors "go:errors"
import f "go:go4t/extern/testdata/errorfixture"

func require(ok Bool) IO[Unit] {
 if ok { ioPure(unit()) } else { ioFail("GoError boundary contract failed") }
}

func main() IO[Unit] {
 do {
  attempt <- io.tryIOCause(f.wrap("outer"))
  match attempt {
   Err(failure) => match io.cause(failure) {
    Some(cause) => do {
     require(f.classify(cause) == "sentinel")
     require(f.isSentinel(cause))
     require(match f.sentinel() { Some(target) => errors.is(cause, target); None => false })
     require(io.goErrorMessage(cause) == "outer: sentinel")
     let report = f.echoReport(f.Report{err: Some(cause), n: 7})
     require(report.n == 7 && (match report.err { Some(e) => errors.is(e, cause); None => false }))
     again <- io.tryIOCause(io.ioRaiseCause(failure))
     require(match again {
      Err(next) => match io.cause(next) { Some(e) => errors.is(e, cause); None => false }
      Ok(_) => false
     })
    }
    None => ioFail("foreign cause missing")
   }
   Ok(_) => ioFail("trailing error did not fail")
  }
  require(match f.newReport().err { None => true; Some(_) => false })
  require(f.invoke(func(cause) => f.classify(cause)) == "sentinel")
  require(match f.errs() {
   Cons(None, Cons(Some(cause), _)) => f.isSentinel(cause)
   _ => false
  })
 }
}
`
