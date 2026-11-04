package extern

import (
	"strings"
	"testing"

	"github.com/reoring/go4t/ast"
)

func TestRefInOutShim(t *testing.T) {
	var loader Loader
	pkg, err := loader.Load("github.com/reoring/go4t/extern/testdata/refinout")
	if err != nil {
		t.Fatal(err)
	}
	decls, shim, _, err := BindWithShim(pkg, BindOptions{}, "example.test/bindings/shim", "shim")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"type Rec = Rec{count: Int, flag: Option[Bool]} deriving Eq, Debug",
		"extern func push(p0 Ref[List[String]], p1 String) IO[Unit]",
		"extern func bump(p0 Ref[Map[String, Int]], p1 String) IO[Unit]",
		"extern func setFlag(p0 Ref[Bool]) IO[Unit]",
		"extern func fill(p0 Ref[Rec]) IO[Unit]",
		"extern func peek(p0 Ref[List[String]]) IO[Int]",
		"extern func many(p0 List[Ref[Int]]) IO[Unit]",
		"extern func bytes(p0 Ref[String]) IO[Unit]",
		"extern func result() Option[Int]",
		"extern func invoke(p0 Ref[Int], p1 func(Ref[Int]) IO[Unit]) IO[Unit]",
		"extern func invokeMany(p0 func(List[Ref[Int]]) IO[Unit]) IO[(Int, Int)]",
		"extern func returned() func(Ref[Int]) IO[Unit]",
		"extern func generic[G0](p0 Option[G0]) IO[Option[G0]]",
		"extern func genericSlice[G0](p0 Ref[List[G0]], p1 G0) IO[Unit]",
	} {
		if !strings.Contains(decls, want) {
			t.Errorf("missing %q:\n%s", want, decls)
		}
	}
	dir := bindingTestModule(t)
	writeBindingTestFile(t, dir, "shim/shim.go", string(shim))
	writeBindingTestFile(t, dir, "shim/shim_test.go", refInOutRuntimeTest)
	integer := genericType("Int")
	for _, tc := range []struct {
		symbol string
		sig    *ast.FuncType
	}{
		{"Generic", genericFunc([]ast.Type{genericType("Option", integer)}, genericType("Option", integer))},
		{"GenericSlice", genericFunc([]ast.Type{genericType("Ref", genericType("List", integer)), integer}, genericType("Unit"))},
	} {
		instance, err := InstantiateGeneric(&loader, pkg, tc.symbol, tc.sig)
		if err != nil {
			t.Fatal(err)
		}
		shim, err := instance.Shim("example.test/bindings/" + tc.symbol)
		if err != nil {
			t.Fatal(err)
		}
		writeBindingTestFile(t, dir, tc.symbol+"/shim.go", string(shim))
	}
	runBindingTestGo(t, dir, "test", "./shim")
}

const refInOutRuntimeTest = `package shim_test
import (
 "context"
 "reflect"
 "testing"
 "github.com/reoring/go4t/rt"
 s "example.test/bindings/shim"
 generic "example.test/bindings/Generic"
 genericSlice "example.test/bindings/GenericSlice"
)
type option struct { value any; present bool }
func TestRefInOutBoundary(t *testing.T) {
 rt.Shapes = rt.ShapeFuncs{
  Nil: func() any { return []any{} },
  Cons: func(h,t any) any { return append([]any{h},t.([]any)...) },
  Uncons: func(v any) (any,any,bool) { xs := v.([]any); if len(xs)==0 { return nil,nil,false }; return xs[0],xs[1:],true },
  Some: func(v any) any { return option{v,true} }, None: func() any { return option{} },
  OptionValue: func(v any) (any,bool) { x := v.(option); return x.value,x.present },
  Tuple2: func(a,b any) any { return [2]any{a,b} },
  Untuple2: func(v any) (any,any) { p := v.([2]any); return p[0],p[1] },
 }
 rt.RegisterForeignRecord("github.com/reoring/go4t/extern/testdata/refinout.Rec",func(v []any) any { return v },func(v any,i int) any { return v.([]any)[i] })
 run := func(action any) any { t.Helper(); v,err := action.(rt.IO)(context.Background()); if err != nil { t.Fatal(err) }; return v }
 equal := func(got,want any) { t.Helper(); if !reflect.DeepEqual(got,want) { t.Fatalf("got %#v, want %#v",got,want) } }
 original := []any{"before"}
 xs := run(rt.RefNew(original))
 action := s.Push(xs,"after")
 equal(run(rt.RefGet(xs)),original)
 run(action)
 equal(run(rt.RefGet(xs)),[]any{"before","after"})
 equal(original,[]any{"before"})
 equal(run(s.Peek(xs)),int64(2))
 equal(run(rt.RefGet(xs)),[]any{"before","after"})
 run(s.Push(xs,"again"))
 equal(run(rt.RefGet(xs)),[]any{"before","after","again"})
 labels := run(rt.RefNew(rt.ForeignMap([][2]any{{"count",int64(2)}})))
 run(s.Bump(labels,"count")); run(s.Bump(labels,"new"))
 got := map[string]int64{}
 rt.ForeignMapEach(run(rt.RefGet(labels)),func(k,v any) { got[k.(string)] = v.(int64) })
 equal(got,map[string]int64{"count":3,"new":1})
 flag := run(rt.RefNew(false)); run(s.SetFlag(flag)); equal(run(rt.RefGet(flag)),true)
 rec := run(rt.RefNew([]any{int64(0),option{}})); run(s.Fill(rec))
 equal(run(rt.RefGet(rec)),[]any{int64(42),option{true,true}})
 a,b := run(rt.RefNew(int64(10))),run(rt.RefNew(int64(20)))
 run(s.Many([]any{a,b})); equal(run(rt.RefGet(a)),int64(11)); equal(run(rt.RefGet(b)),int64(22))
 run(s.Many([]any{}))
 bytes := run(rt.RefNew("raw")); run(s.Bytes(bytes)); equal(run(rt.RefGet(bytes)),"raw!")
 equal(s.Result(),option{int64(7),true})
 _,err := s.MutateError(a).(rt.IO)(context.Background())
 if err == nil || err.Error() != "changed before failure" { t.Fatalf("failure = %v",err) }
 equal(run(rt.RefGet(a)),int64(12))
 equal(run(s.MutateAbsent(a)),option{}); equal(run(rt.RefGet(a)),int64(13))
 run(s.Invoke(a,func(cell any) any { return rt.RefSet(cell,int64(30)) }))
 equal(run(rt.RefGet(a)),int64(30))
 equal(run(s.InvokeMany(func(cells any) any { return s.Many(cells) })),[2]any{int64(11),int64(22)})
 run(s.Returned().(func(any) any)(a)); equal(run(rt.RefGet(a)),int64(31))
 equal(run(generic.Generic(option{int64(9),true})),option{int64(9),true})
 numbers := run(rt.RefNew([]any{int64(1)}))
 run(genericSlice.GenericSlice(numbers,int64(2)))
 equal(run(rt.RefGet(numbers)),[]any{int64(1),int64(2)})
 cancelled,cancel := context.WithCancel(context.Background()); cancel()
 if _,err := s.Many([]any{a,b}).(rt.IO)(cancelled); err == nil { t.Fatal("ignored cancellation") }
 equal(run(rt.RefGet(a)),int64(31)); equal(run(rt.RefGet(b)),int64(22))
}
`
