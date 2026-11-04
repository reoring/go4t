package extern

import (
	"strings"
	"testing"
)

func TestJSONViewShim(t *testing.T) {
	var loader Loader
	pkg, err := loader.Load("github.com/reoring/go4t/extern/testdata/jsonview")
	if err != nil {
		t.Fatal(err)
	}
	decls, shim, _, err := BindWithShim(pkg, BindOptions{}, "example.test/bindings/shim", "shim")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		`import dyn "std/dyn"`,
		"type Stamp = StampHandle\n",
		"type Pair = Pair{a: Int, b: Int} deriving Eq, Debug\n",
		"type Holder = Holder{when: Stamp, n: Int}\n",
		"type Nested = Nested{holders: List[Holder], maybe: Option[Holder], stamps: Map[String, Stamp]}\n",
		"extern func stampToJson(recv Stamp) dyn.Value",
		"extern func stampFromJson(v dyn.Value) IO[Stamp]",
		"extern func pairToJson(recv Pair) dyn.Value",
		"extern func pairFromJson(v dyn.Value) IO[Pair]",
		"extern func numberToJson(recv Int) dyn.Value",
		"extern func numberFromJson(v dyn.Value) IO[Int]",
		"extern func echoHolder(p0 Holder) Holder",
	} {
		if !strings.Contains(decls, want) {
			t.Errorf("missing %q:\n%s", want, decls)
		}
	}
	for _, forbidden := range []string{"pointerOnlyToJson(", "marshalOnlyToJson(", "unmarshalOnlyFromJson(", "genericToJson("} {
		if strings.Contains(decls, forbidden) {
			t.Errorf("unexpected %q:\n%s", forbidden, decls)
		}
	}
	dir := bindingTestModule(t)
	writeBindingTestFile(t, dir, "shim/shim.go", string(shim))
	writeBindingTestFile(t, dir, "shim/shim_test.go", jsonViewRuntimeTest)
	runBindingTestGo(t, dir, "test", "./shim")
}

func TestJSONViewHandleStorage(t *testing.T) {
	dir := bindingTestModule(t)
	writeBindingTestFile(t, dir, "main.gt", `package main
import "std/dyn"
import f "go:github.com/reoring/go4t/extern/testdata/jsonview"
func keep(h f.Holder) f.Holder { h{n: h.n + 1} }
func main() IO[Unit] {
 do {
  stamp <- f.stampFromJson(dyn.Str("2026-06-24T10:02:03+09:00"))
  let holder = keep(f.Holder{when: stamp, n: 2})
  same <- f.same(stamp, holder.when)
  if same && holder.n == 3 && f.stampToJson(holder.when) == dyn.Str("2026-06-24T01:02:03Z") {
   ioPure(unit())
  } else { ioFail("Holder.When lost handle identity") }
 }
}
`)
	runBindingTestGo(t, dir, "run", "-mod=mod", "github.com/reoring/go4t/cmd/go4t", "run", ".")
}

func TestJSONViewStructuralInterfacesAndPrecedence(t *testing.T) {
	dir := bindingTestModule(t)
	// Structural implementations need not import encoding/json. Existing calls,
	// field accessors and value getters all precede generated JSON views.
	writeBindingTestFile(t, dir, "fixture/fixture.go", `package fixture
type Scalar struct { private int }
func (Scalar) MarshalJSON() ([]byte,error) { return []byte("null"),nil }
func (*Scalar) UnmarshalJSON([]byte) error { return nil }
func ScalarToJson() string { return "call" }
var ScalarFromJson = "value"
type Accessor struct { ToJson int; private int }
func (Accessor) MarshalJSON() ([]byte,error) { return []byte("null"),nil }
func (*Accessor) UnmarshalJSON([]byte) error { return nil }
type Plain int
func (Plain) MarshalJSON() ([]byte,error) { return []byte("1"),nil }
func (*Plain) UnmarshalJSON([]byte) error { return nil }
`)
	loader := Loader{Dir: dir}
	pkg, err := loader.Load("example.test/bindings/fixture")
	if err != nil {
		t.Fatal(err)
	}
	for _, prefix := range []string{"", "native"} {
		decls, shim, _, err := BindWithShim(pkg, BindOptions{Prefix: prefix}, "example.test/bindings/shim", "shim")
		if err != nil {
			t.Fatal(err)
		}
		for _, tc := range []struct{ display, name, winner string }{
			{"Scalar.ToJson", "ScalarToJson", "ScalarToJson"},
			{"Scalar.FromJson", "ScalarFromJson", "ScalarFromJson"},
			{"Accessor.ToJson", "AccessorToJson", "Accessor.ToJson"},
		} {
			name, _ := bindingName(tc.name, prefix)
			want := "// skipped: " + tc.display + " (name " + name + " taken by " + tc.winner + ")"
			if !strings.Contains(decls, want) || strings.Count(decls, "extern func "+name+"(") != 1 {
				t.Errorf("missing single winning binding and %q:\n%s", want, decls)
			}
		}
		name, _ := bindingName(methodBindingName("Plain", "FromJson", prefix), prefix)
		if !strings.Contains(decls, "extern func "+name+"(v dyn.Value) IO[Int]") {
			t.Errorf("structural JSON interfaces: %s", decls)
		}
		if prefix == "" {
			writeBindingTestFile(t, dir, "shim/shim.go", string(shim))
		}
	}
	runBindingTestGo(t, dir, "test", "./shim")
}

const jsonViewRuntimeTest = `package shim_test
import (
 "context"
 "errors"
 "fmt"
 "math"
 "reflect"
 "strings"
 "testing"
 "time"
 "github.com/reoring/go4t/rt"
 f "github.com/reoring/go4t/extern/testdata/jsonview"
 s "example.test/bindings/shim"
)
type dynamic struct { kind string; value any }
func TestJSONViews(t *testing.T) {
 rt.Shapes = rt.ShapeFuncs{
  Nil: func() any { return []any{} },
  Cons: func(h,t any) any { return append([]any{h},t.([]any)...) },
  Uncons: func(v any) (any,any,bool) { xs:=v.([]any); if len(xs)==0 { return nil,nil,false }; return xs[0],xs[1:],true },
  DynStr: func(v any) any { return dynamic{"str",v} },
  DynFloat: func(v any) any { return dynamic{"float",v} },
  DynList: func(v any) any { return dynamic{"list",v} },
  DynView: func(v any) (string,any) { d:=v.(dynamic); return d.kind,d.value },
 }
 for _,name := range []string{"Holder","Pair"} {
  rt.RegisterForeignRecord("github.com/reoring/go4t/extern/testdata/jsonview."+name,func(v []any) any { return v },func(v any,i int) any { return v.([]any)[i] })
 }
 run := func(action any) any { t.Helper(); v,err:=action.(rt.IO)(context.Background()); if err!=nil { t.Fatal(err) }; return v }
 equal := func(got,want any) { t.Helper(); if !reflect.DeepEqual(got,want) { t.Fatalf("got %#v, want %#v",got,want) } }
 stamp:=run(s.Stamp_FromJson(dynamic{"str","2026-06-24T10:02:03+09:00"})).(*f.Stamp)
 equal(s.Stamp_ToJson(stamp),dynamic{"str","2026-06-24T01:02:03Z"})
 holder:=s.EchoHolder([]any{stamp,int64(7)}).([]any)
 copied:=holder[0].(*f.Stamp)
 equal(holder[1],int64(7))
 equal(s.Stamp_ToJson(copied),s.Stamp_ToJson(stamp))
 equal(run(s.Stamp_Location(copied)),run(s.Stamp_Location(stamp)))
 pair:=[]any{int64(3),int64(4)}
 wire:=dynamic{"list",[]any{dynamic{"float",float64(3)},dynamic{"float",float64(4)}}}
 equal(s.Pair_ToJson(pair),wire)
 equal(run(s.Pair_FromJson(wire)),pair)
 equal(s.Number_ToJson(int64(9)),dynamic{"float",float64(9)})
 equal(run(s.Number_FromJson(dynamic{"int",int64(9)})),int64(9))
 _,err:=s.Invalid_FromJson(dynamic{"str","bad"}).(rt.IO)(context.Background())
 var failure *rt.Failure
 if !errors.As(err,&failure) || failure.Cause != f.InvalidJSON { t.Fatalf("lost Go cause: %#v",err) }
 _,err=s.Stamp_FromJson(dynamic{"str","bad"}).(rt.IO)(context.Background())
 var parseError *time.ParseError
 if !errors.As(err,&parseError) { t.Fatalf("lost parse error: %#v",err) }
 _,err=s.Number_FromJson(dynamic{"float",math.Inf(1)}).(rt.IO)(context.Background())
 if !errors.As(err,&failure) || failure.Cause==nil { t.Fatalf("marshal input failure: %#v",err) }
 ctx,cancel:=context.WithCancel(context.Background()); cancel()
 _,err=s.Invalid_FromJson(dynamic{"str","bad"}).(rt.IO)(ctx)
 if !errors.Is(err,context.Canceled) { t.Fatalf("cancelled view: %v",err) }
 for _,tc:=range []struct{name string; call func()}{
  {"Invalid",func(){ s.Invalid_ToJson(&f.Invalid{}) }},
  {"Malformed",func(){ s.Malformed_ToJson(&f.Malformed{}) }},
 } {
  t.Run(tc.name,func(t *testing.T) {
   defer func(){ got:=recover(); want:="go4t json view: github.com/reoring/go4t/extern/testdata/jsonview."+tc.name+": "; if got==nil || !strings.HasPrefix(fmt.Sprint(got),want) { t.Fatalf("panic = %v, want %s",got,want) } }()
   tc.call()
  })
 }
}
`
