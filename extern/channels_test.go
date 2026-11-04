package extern

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/reoring/go4t/laws"
	"github.com/reoring/go4t/modules"
	"github.com/reoring/go4t/types"
)

func TestChannelShim(t *testing.T) {
	dir := bindingTestModule(t)
	writeBindingTestFile(t, dir, "fixture/fixture.go", channelFixture)
	loader := Loader{Dir: dir}
	pkg, err := loader.Load("example.test/bindings/fixture")
	if err != nil {
		t.Fatal(err)
	}
	decls, shim, _, err := BindWithShim(pkg, BindOptions{Float: true}, "example.test/bindings/shim", "shim")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"extern func echo(p0 Chan[Int]) IO[Chan[Int]]",
		"extern func send(p0 SendChan[Int], p1 Int) IO[Unit]",
		"extern func receive(p0 RecvChan[Int]) IO[Option[Int]]",
		"extern func done() RecvChan[Unit]",
		"extern func pointer(p0 Option[Chan[Int]]) IO[Option[Chan[Int]]]",
		"extern func nested(p0 List[Chan[Int]]) IO[List[Chan[Int]]]",
		"extern func valuesLen(recv Chan[Int]) IO[Int]",
		"extern func nestedChannel(p0 Chan[Chan[Int]]) IO[Chan[Chan[Int]]]",
		"extern func payloads(p0 Chan[Payload]) IO[Chan[Payload]]",
		"extern func invoke(p0 func(Chan[Int]) IO[Int]) IO[Int]",
		"extern func returned() func(Chan[Int]) IO[Int]",
		"extern func fromCallback(p0 func() Chan[Int]) IO[Chan[Int]]",
		"extern func newIntChan(buffer Int) IO[Chan[Int]]",
		"extern func newPayloadChan(buffer Int) IO[Chan[Payload]]",
		"// skipped: Bad (unsupported parameter type chan complex64)",
		"// skipped: BadResult (unsupported result type chan [2]int)",
		"// skipped: RecursiveEcho (unsupported parameter type example.test/bindings/fixture.Recursive)",
	} {
		if !strings.Contains(decls, want) {
			t.Errorf("missing %q:\n%s", want, decls)
		}
	}
	writeBindingTestFile(t, dir, "shim/shim.go", string(shim))
	writeBindingTestFile(t, dir, "shim/shim_test.go", channelShimRuntimeTest)
	runBindingTestGo(t, dir, "test", "./shim")
}

const channelFixture = `package fixture

type Values chan int
func (v Values) Len() int { return len(v) }
func Echo(c chan int) chan int { return c }
func Send(c chan<- int, n int) { c <- n }
func Receive(c <-chan int) (int, bool) { n, ok := <-c; return n, ok }
func Done() <-chan struct{} { c := make(chan struct{}); close(c); return c }
func Pointer(c *chan int) *chan int { return c }
func Nested(c []chan int) []chan int { return c }
func NestedChannel(c chan chan int) chan chan int { return c }
func Invoke(f func(chan int) int) int { c := make(chan int,1); c <- 8; return f(c) }
func Returned() func(chan int) int { return func(c chan int) int { return len(c) } }
func FromCallback(f func() chan int) chan int { return f() }
type Payload struct { N int; Values []string }
func Payloads(c chan Payload) chan Payload { return c }
func Bad(chan complex64) {}
func BadResult() chan [2]int { return nil }
type Recursive chan Recursive
func RecursiveEcho(c Recursive) Recursive { return c }
`

const channelShimRuntimeTest = `package shim_test
import (
 "context"
 "reflect"
 "strings"
 "testing"
 "github.com/reoring/go4t/rt"
 f "example.test/bindings/fixture"
 s "example.test/bindings/shim"
)
type option struct { value any; present bool }
func TestChannelBoundary(t *testing.T) {
 rt.Shapes = rt.ShapeFuncs{
  Some: func(v any) any { return option{v,true} }, None: func() any { return option{} },
  OptionValue: func(v any) (any,bool) { x := v.(option); return x.value,x.present },
  Nil: func() any { return []any{} }, Cons: func(h,t any) any { return append([]any{h},t.([]any)...) },
  Uncons: func(v any) (any,any,bool) { xs := v.([]any); if len(xs)==0 { return nil,nil,false }; return xs[0],xs[1:],true },
 }
 rt.RegisterForeignRecord("example.test/bindings/fixture.Payload",func(v []any) any { return v },func(v any,i int) any { return v.([]any)[i] })
 run := func(action any) any { t.Helper(); v,err := action.(rt.IO)(context.Background()); if err != nil { t.Fatal(err) }; return v }
 equal := func(got,want any) { t.Helper(); if !reflect.DeepEqual(got,want) { t.Fatalf("got %#v, want %#v",got,want) } }
 c := run(s.Go4t_NewIntChan(int64(2)))
 raw := rt.GoChannel[chan int](c)
 if rt.GoChannel[chan int](run(s.Echo(c))) != raw { t.Fatal("round trip lost identity") }
 run(s.Send(rt.ChanAsSend(c),int64(19)))
 equal(run(rt.ChanRecv(rt.ChanAsRecv(c))),option{int64(19),true})
 run(rt.ChanSend(rt.ChanAsSend(c),int64(23)))
 equal(run(s.Values_Len(c)),int64(1))
 equal(run(s.Returned().(func(any) any)(c)),int64(1))
 called := false
 invocation := s.Invoke(func(c any) any { return rt.IO(func(ctx context.Context) (any,error) {
  called = true
  received,err := rt.ChanRecv(c).(rt.IO)(ctx)
  if err != nil { return nil,err }
  return received.(option).value.(int64)*2,nil
 }) })
 if called { t.Fatal("effectful callback ran before IO") }
 equal(run(invocation),int64(16))
 equal(run(s.Receive(rt.ChanAsRecv(c))),option{int64(23),true})
 pointer := run(s.Pointer(option{c,true})).(option)
 if rt.GoChannel[chan int](pointer.value) != raw { t.Fatal("pointer lost identity") }
 nested := run(s.Nested([]any{c})).([]any)
 if rt.GoChannel[chan int](nested[0]) != raw { t.Fatal("slice lost identity") }
 outer := run(s.Go4t_NewChanIntChan(int64(1)))
 run(rt.ChanSend(outer,c))
 inner := run(rt.ChanRecv(run(s.NestedChannel(outer)))).(option).value
 if rt.GoChannel[chan int](inner) != raw { t.Fatal("nested channel lost identity") }
 equal(run(rt.ChanRecv(s.Done())),option{})
 run(rt.ChanClose(rt.ChanAsSend(c)))
 equal(run(s.Receive(rt.ChanAsRecv(c))),option{})
 wrong := run(rt.ChanMake(int64(1)))
 if rt.GoChannel[chan int](run(s.FromCallback(func() any { return c }))) != raw { t.Fatal("callback return lost identity") }
 for _,action := range []any{s.Echo(wrong),s.Pointer(option{wrong,true}),s.Nested([]any{wrong}),rt.ChanSend(outer,wrong),s.FromCallback(func() any { return wrong })} {
  _,err := action.(rt.IO)(context.Background())
  failure,ok := err.(*rt.Failure)
  if !ok || failure.Kind != rt.FailureKind || !strings.Contains(failure.Message,"typed channel constructor") { t.Fatalf("mismatch = %#v",err) }
 }
 records := run(s.Go4t_NewPayloadChan(int64(1)))
 original := []any{int64(7),[]any{"before"}}
 run(rt.ChanSend(records,original))
 nativeRecord := <-rt.GoChannel[chan f.Payload](records)
 nativeRecord.Values[0] = "after"
 equal(original,[]any{int64(7),[]any{"before"}})
 nativeRecord.N = 9
 rt.GoChannel[chan f.Payload](records) <- nativeRecord
 copied := run(rt.ChanRecv(records)).(option).value
 nativeRecord.Values[0] = "later"
 equal(copied,[]any{int64(9),[]any{"after"}})
}
`

func TestChannelCapabilities(t *testing.T) {
	for _, tc := range []struct{ name, source, want string }{
		{"opaque", `func keep[A](c Chan[A]) Chan[A] { c }`, ""},
		{"qualified", `func keep(c ch.Chan[Int]) ch.RecvChan[Int] { ch.asRecv(c) }`, ""},
		{"send receive-only", `func bad(c RecvChan[Int]) IO[Unit] { ch.send(c, 1) }`, "type mismatch"},
		{"close receive-only", `func bad(c RecvChan[Int]) IO[Unit] { ch.close(c) }`, "type mismatch"},
		{"receive send-only", `func bad(c SendChan[Int]) IO[Option[Int]] { ch.recv(c) }`, "type mismatch"},
		{"recover send capability", `func bad(c RecvChan[Int]) SendChan[Int] { ch.asSend(c) }`, "type mismatch"},
		{"wrong element", `func bad(c Chan[Int]) Chan[String] { c }`, "type mismatch"},
		{"arity", `func bad(c Chan[Int, Bool]) Int { 0 }`, "kind mismatch"},
		{"show", `func bad(c Chan[Int]) String { show(c) }`, "no instance Show[Chan[Int]]"},
		{"equality", `func bad(c Chan[Int]) Bool { c == c }`, "no instance Eq[Chan[Int]]"},
		{"debug", `func bad(c Chan[Int]) String { debug(c) }`, "no instance Debug[Chan[Int]]"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			writeBindingTestFile(t, dir, "main.gt", "package main\nimport ch \"std/chan\"\n"+tc.source+"\nfunc main() IO[Unit] { ioPure(unit()) }\n")
			loaded, err := modules.Load([]string{dir})
			if err != nil {
				t.Fatal(err)
			}
			_, err = types.Check(loaded)
			if tc.want == "" && err != nil || tc.want != "" && (err == nil || !strings.Contains(err.Error(), tc.want)) {
				t.Fatalf("error = %v, want %q", err, tc.want)
			}
		})
	}
}

func TestChannelUpcastLawIsIneligible(t *testing.T) {
	dir := t.TempDir()
	writeBindingTestFile(t, dir, "main.gt", `package main
import ch "std/chan"
func receive(c Chan[Int]) RecvChan[Int] { ch.asRecv(c) }
law receiveLaw(c Chan[Int]) { receive(c) == receive(c) }
proof receiveLaw(c) { refl }
func main() IO[Unit] { ioPure(unit()) }
`)
	loaded, err := modules.Load([]string{filepath.Join(dir, "main.gt")})
	if err != nil {
		t.Fatal(err)
	}
	program, err := types.Check(loaded)
	if err != nil {
		t.Fatal(err)
	}
	diagnostics := laws.CheckProgram(program)
	if len(diagnostics) != 1 || !strings.Contains(diagnostics[0].Msg, "law depends on extern github.com/reoring/go4t/rt.ChanAsRecv") {
		t.Fatalf("law diagnostics = %v", diagnostics)
	}
}
