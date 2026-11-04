package extern

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func bindingTestModule(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	root, err := filepath.Abs("..")
	if err != nil {
		t.Fatal(err)
	}
	writeBindingTestFile(t, dir, "go.mod", "module example.test/bindings\n\ngo 1.23\n\nrequire github.com/reoring/go4t v0.0.0\nreplace github.com/reoring/go4t => "+filepath.ToSlash(root)+"\n")
	return dir
}

func writeBindingTestFile(t *testing.T, dir, name, source string) {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(source), 0644); err != nil {
		t.Fatal(err)
	}
}

func runBindingTestGo(t *testing.T, dir string, args ...string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "go", args...)
	cmd.Dir = dir
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("go %s: %v\n%s", strings.Join(args, " "), err, output)
	}
}

func TestExtendedBindingShim(t *testing.T) {
	dir := bindingTestModule(t)
	writeBindingTestFile(t, dir, "fixture/fixture.go", extendedBindingFixture)
	loader := Loader{Dir: dir}
	pkg, err := loader.Load("example.test/bindings/fixture")
	if err != nil {
		t.Fatal(err)
	}
	decls, shim, _, err := BindWithShim(pkg, BindOptions{}, "example.test/bindings/shim", "shim")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"extern func pair() (String, Int)",
		"extern func triple() (String, Int, Bool)",
		"extern func pairError(p0 Bool) IO[(String, Int)]",
		"extern func names() List[String]",
		"extern func entries() List[(String, Int)]",
		"extern func returned() func(Int) IO[(String, Int)]",
		"extern func returnedContext() func(Int) IO[Int]",
		"extern func returnedSeq() func() List[String]",
		"extern func functionalOption() func(Ref[Opts]) IO[Unit]",
		"extern func invoke(p0 func() IO[(String, Int)]) IO[(String, Int)]",
		"extern func invokeContext(p0 func() IO[(String, Int)]) IO[(String, Int)]",
		"extern func invokeTriple(p0 func() (String, Int, Bool)) (String, Int, Bool)",
		"extern func opaqueLabel(recv Opaque) IO[String]",
		"extern func opaqueSetLabel(recv Opaque, value String) IO[Unit]",
		"extern func opaqueApply(recv Opaque) IO[func(Int) Int]",
		"extern func opaqueSetApply(recv Opaque, value func(Int) Int) IO[Unit]",
		"extern func opaqueReadLabel(recv Opaque) IO[String]",
		"extern func optionName(recv OptionGo) IO[String]",
		"extern func opaqueBlocked(recv Opaque) IO[Chan[Int]]",
		"extern func opaqueSetBlocked(recv Opaque, value Chan[Int]) IO[Unit]",
		"// skipped: Opaque.Conflict (name opaqueSetConflict taken by Opaque.SetConflict)",
		"// skipped: ConsumeSeq (unsupported parameter type iter.Seq[string])",
		"// skipped: ConsumeSeq2 (unsupported parameter type iter.Seq2[string, int])",
		"// skipped: Quad (unsupported multiple results: arity 4)",
	} {
		if !strings.Contains(decls, want) {
			t.Errorf("missing %q:\n%s", want, decls)
		}
	}
	prefixed, _, _, err := BindWithShim(pkg, BindOptions{Prefix: "native"}, "example.test/bindings/shim", "shim")
	if err != nil || !strings.Contains(prefixed, "extern func nativeOpaqueSetLabel(recv Opaque, value String) IO[Unit]") {
		t.Fatalf("prefixed accessors: %v\n%s", err, prefixed)
	}
	writeBindingTestFile(t, dir, "shim/shim.go", string(shim))
	writeBindingTestFile(t, dir, "shim/shim_test.go", extendedBindingRuntimeTest)
	runBindingTestGo(t, dir, "test", "./shim")
}

func TestStandardExtendedBindings(t *testing.T) {
	dir := bindingTestModule(t)
	loader := Loader{Dir: dir}
	for _, tc := range []struct {
		path string
		name string
		want []string
	}{
		{"strings", "shim_strings", []string{
			`extern func cut(p0 String, p1 String) (String, String, Bool) = "example.test/bindings/shim_strings".Cut`,
			`extern func lines(p0 String) List[String] = "example.test/bindings/shim_strings".Lines`,
			"extern func fieldsSeq(p0 String) List[String]",
			"extern func splitSeq(p0 String, p1 String) List[String]",
			`import go_io "go:io"`,
			`extern func readerAsReader(recv Reader) go_io.Reader = "example.test/bindings/shim_strings".Reader_AsReader`,
		}},
		{"context", "shim_context", []string{
			`extern func withCancel() IO[(Context, func() Unit)] = "example.test/bindings/shim_context".WithCancel`,
			"extern func afterFunc(p0 func() Unit) IO[func() Bool]",
		}},
		{"io", "shim_io", []string{
			"extern func pipe() (PipeReader, PipeWriter)",
			`extern func readCloserAsReader(recv ReadCloser) Reader = "example.test/bindings/shim_io".ReadCloser_AsReader`,
			`extern func readCloserAsCloser(recv ReadCloser) Closer = "example.test/bindings/shim_io".ReadCloser_AsCloser`,
		}},
		{"net/http", "shim_http", []string{
			`extern func requestHeader(recv Request) IO[Map[String, List[String]]] = "example.test/bindings/shim_http".Request_Header`,
			`extern func requestSetHeader(recv Request, value Map[String, List[String]]) IO[Unit] = "example.test/bindings/shim_http".Request_SetHeader`,
			`import go_io "go:io"`,
			`extern func newRequest(p0 String, p1 String, p2 go_io.Reader) IO[Request] = "example.test/bindings/shim_http".NewRequest`,
			"extern func responseBody(recv Response) IO[go_io.ReadCloser]",
			`extern func defaultClient() Client = "example.test/bindings/shim_http".DefaultClient`,
			`extern func newClient() Client = "example.test/bindings/shim_http".New_Client`,
			`extern func methodGet() String = "example.test/bindings/shim_http".MethodGet`,
		}},
	} {
		pkg, err := loader.Load(tc.path)
		if err != nil {
			t.Fatal(err)
		}
		decls, shim, _, err := BindWithShim(pkg, BindOptions{Float: true}, "example.test/bindings/"+tc.name, tc.name)
		if err != nil {
			t.Fatal(err)
		}
		for _, want := range tc.want {
			if !strings.Contains(decls, want) {
				t.Errorf("%s: missing %q:\n%s", tc.path, want, decls)
			}
		}
		if strings.Contains(decls, `"go:`+tc.path+`"`) {
			t.Errorf("%s: generated self-import", tc.path)
		}
		if tc.path == "net/http" && strings.Contains(decls, "type Reader =") {
			t.Error("net/http redeclared io.Reader")
		}
		writeBindingTestFile(t, dir, tc.name+"/shim.go", string(shim))
	}
	runBindingTestGo(t, dir, "build", "./shim_strings", "./shim_context", "./shim_io", "./shim_http")
	writeBindingTestFile(t, dir, "shim_http/composition_test.go", standardBindingCompositionTest)
	runBindingTestGo(t, dir, "test", "./shim_http")
}

const extendedBindingFixture = `package fixture
import (
 "context"
 "errors"
 "iter"
)
func Pair() (string, int) { return "pair", 7 }
func Triple() (string, int, bool) { return "triple", 3, true }
func Quad() (int, int, int, int) { return 1, 2, 3, 4 }
func PairError(fail bool) (string, int, error) {
 if fail { return "", 0, errors.New("pair failed") }; return "pair", 7, nil
}
func Names() iter.Seq[string] {
 return func(yield func(string) bool) { if !yield("first") { return }; yield("second") }
}
func Entries() iter.Seq2[string, int] {
 return func(yield func(string, int) bool) { if !yield("first", 1) { return }; yield("second", 2) }
}
func ConsumeSeq(iter.Seq[string]) {}
func ConsumeSeq2(iter.Seq2[string, int]) {}
func Returned() func(int) (string, int, error) {
 return func(n int) (string, int, error) { if n < 0 { return "", 0, errors.New("negative") }; return "result", n, nil }
}
func ReturnedContext() func(context.Context, int) (int, error) {
 return func(ctx context.Context, n int) (int, error) { if ctx.Value("test") != "ctx" { return 0, errors.New("context lost") }; return n, nil }
}
func ReturnedSeq() func() iter.Seq[string] { return Names }
type Opts struct { Count int }
func FunctionalOption() func(*Opts) { return func(o *Opts) { o.Count++ } }
func Invoke(f func() (string, int, error)) (string, int, error) { return f() }
func InvokeContext(ctx context.Context, f func(context.Context) (string, int, error)) (string, int, error) { return f(ctx) }
func InvokeTriple(f func() (string, int, bool)) (string, int, bool) { return f() }
func InvokeSeq(f func() iter.Seq[string]) iter.Seq[string] { return f() }
type Opaque struct { Label string; Apply func(int) int; Blocked chan int; Conflict int }
func (o Opaque) ReadLabel() string { return o.Label }
func (o *Opaque) SetConflict() {}
func Value() Opaque { return Opaque{Label: "value", Apply: func(n int) int { return n+1 }} }
func Pointer() *Opaque { v := Value(); return &v }
func ReadValue(v Opaque) string { return v.Label }
type Option struct { Name string; Blocked chan int }
func NamedOption() *Option { return &Option{Name: "option"} }
`

const extendedBindingRuntimeTest = `package shim_test
import (
 "context"
 "errors"
 "reflect"
 "testing"
 "github.com/reoring/go4t/rt"
 "example.test/bindings/fixture"
 s "example.test/bindings/shim"
)
type option struct { value any; present bool }
func TestBoundary(t *testing.T) {
 rt.Shapes = rt.ShapeFuncs{
  Nil: func() any { return []any{} },
  Cons: func(h,t any) any { return append([]any{h},t.([]any)...) },
  Uncons: func(v any) (any,any,bool) { xs := v.([]any); if len(xs)==0 { return nil,nil,false }; return xs[0],xs[1:],true },
  Some: func(v any) any { return option{v,true} },
  None: func() any { return option{} },
  OptionValue: func(v any) (any,bool) { x := v.(option); return x.value,x.present },
  Tuple2: func(a,b any) any { return [2]any{a,b} },
  Untuple2: func(v any) (any,any) { p := v.([2]any); return p[0],p[1] },
  Tuple3: func(a,b,c any) any { return [3]any{a,b,c} },
  Untuple3: func(v any) (any,any,any) { p := v.([3]any); return p[0],p[1],p[2] },
 }
 rt.RegisterForeignRecord("example.test/bindings/fixture.Opts", func(fields []any) any { return fields }, func(v any, i int) any { return v.([]any)[i] })
 equal := func(got,want any) { t.Helper(); if !reflect.DeepEqual(got,want) { t.Fatalf("got %#v, want %#v",got,want) } }
 run := func(action any) any { t.Helper(); v,err := action.(rt.IO)(context.Background()); if err != nil { t.Fatal(err) }; return v }
 equal(s.Pair(),[2]any{"pair",int64(7)})
 equal(s.Triple(),[3]any{"triple",int64(3),true})
 equal(run(s.PairError(false)),[2]any{"pair",int64(7)})
 if _,err := s.PairError(true).(rt.IO)(context.Background()); err == nil || err.Error()!="pair failed" { t.Fatalf("tuple failure: %v",err) }
 equal(s.Names(),[]any{"first","second"})
 equal(s.Entries(),[]any{[2]any{"first",int64(1)},[2]any{"second",int64(2)}})
 returned := s.Returned().(func(any) any)
 equal(run(returned(int64(9))),[2]any{"result",int64(9)})
 if _,err := returned(int64(-1)).(rt.IO)(context.Background()); err == nil || err.Error()!="negative" { t.Fatalf("returned failure: %v",err) }
 ctx := context.WithValue(context.Background(),"test","ctx")
 v,err := s.ReturnedContext().(func(any) any)(int64(5)).(rt.IO)(ctx)
 if err != nil { t.Fatal(err) }; equal(v,int64(5))
 equal(s.ReturnedSeq().(func() any)(),[]any{"first","second"})
 opts := run(rt.RefNew([]any{int64(2)}))
 equal(run(s.FunctionalOption().(func(any) any)(opts)),rt.Unit{})
 equal(run(rt.RefGet(opts)),[]any{int64(3)})
 callback := func() any { return rt.IO(func(context.Context) (any,error) { return [2]any{"callback",int64(4)},nil }) }
 equal(run(s.Invoke(callback)),[2]any{"callback",int64(4)})
 equal(run(s.InvokeContext(callback)),[2]any{"callback",int64(4)})
 sentinel := errors.New("callback failed")
 failing := func() any { return rt.IO(func(context.Context) (any,error) { return nil,sentinel }) }
 if _,err := s.Invoke(failing).(rt.IO)(ctx); err == nil || err.Error()!=sentinel.Error() { t.Fatalf("callback error: %v",err) }
 equal(s.InvokeTriple(func() any { return [3]any{"callback",int64(8),true} }),[3]any{"callback",int64(8),true})
 equal(s.InvokeSeq(func() any { return []any{"a","b"} }),[]any{"a","b"})
 for _,handle := range []any{s.Value(),s.Pointer()} {
  equal(run(s.Opaque_Label(handle)),"value")
  action := s.Opaque_SetLabel(handle,"changed")
  equal(run(s.Opaque_Label(handle)),"value")
  run(action)
  equal(run(s.Opaque_Label(handle)),"changed")
  equal(run(s.Opaque_ReadLabel(handle)),"changed")
  equal(run(s.ReadValue(handle)),"changed")
  equal(run(s.Opaque_Apply(handle)).(func(any) any)(int64(3)),int64(4))
  run(s.Opaque_SetApply(handle,func(v any) any { return v.(int64)*2 }))
  equal(handle.(*fixture.Opaque).Apply(3),6)
  cancelled,cancel := context.WithCancel(context.Background()); cancel()
  if _,err := s.Opaque_SetLabel(handle,"wrong").(rt.IO)(cancelled); err == nil { t.Fatal("setter ignored cancellation") }
  equal(run(s.Opaque_Label(handle)),"changed")
 }
}
`

const standardBindingCompositionTest = `package shim_http_test
import (
 "context"
 "testing"
 "github.com/reoring/go4t/rt"
 h "example.test/bindings/shim_http"
 i "example.test/bindings/shim_io"
 s "example.test/bindings/shim_strings"
)
func TestReaderComposition(t *testing.T) {
 reader := s.NewReader("cross-package body")
 asReader := s.Reader_AsReader(reader)
 if reader != asReader { t.Fatal("upcast changed handle identity") }
 request,err := h.NewRequest("POST","http://example.invalid/",asReader).(rt.IO)(context.Background())
 if err != nil { t.Fatal(err) }
 body,err := h.Request_Body(request).(rt.IO)(context.Background())
 if err != nil { t.Fatal(err) }
 asBodyReader := i.ReadCloser_AsReader(body)
 if body != asBodyReader { t.Fatal("interface upcast changed handle identity") }
 text,err := i.ReadAll(asBodyReader).(rt.IO)(context.Background())
 if err != nil || text != "cross-package body" { t.Fatalf("ReadAll = %v, %v",text,err) }
 if _,err := i.Closer_Close(i.ReadCloser_AsCloser(body)).(rt.IO)(context.Background()); err != nil { t.Fatal(err) }
}
`
