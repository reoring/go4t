package extern

import (
	"strings"
	"testing"
)

func TestValueAndConstructorBindings(t *testing.T) {
	dir := bindingTestModule(t)
	writeBindingTestFile(t, dir, "values/values.go", `package values
type Client struct { Tag string; Blocked chan int }
type Existing struct { private int }
func NewExisting() *Existing { return &Existing{} }
type State struct { private int }
func (*State) Count() int { return 3 }
var StateCount = 99
var DefaultClient = &Client{Tag: "original"}
var Count = 1
var Bad chan int
var Type = 1
var Callback = func(n int) int { return n+1 }
const MethodGet = "GET"
const Enabled = true
const Number = 7
const Rune = 'a'
const Decimal = 1.25
const MaxUint uint64 = ^uint64(0)
const Huge = 1 << 100
const HugeFloat = 1e1000
const Complex = 1+2i
`)
	loader := Loader{Dir: dir}
	pkg, err := loader.Load("example.test/bindings/values")
	if err != nil {
		t.Fatal(err)
	}
	decls, shim, _, err := BindWithShim(pkg, BindOptions{Float: true}, "example.test/bindings/shim_values", "shim_values")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		`extern func newClient() Client = "example.test/bindings/shim_values".New_Client`,
		`extern func defaultClient() Client = "example.test/bindings/shim_values".DefaultClient`,
		`extern func methodGet() String = "example.test/bindings/shim_values".MethodGet`,
		"extern func enabled() Bool",
		"extern func number() Int",
		"extern func rune() Int",
		"extern func decimal() Float",
		"extern func count() Int",
		"extern func callback() func(Int) Int",
		"// skipped: NewExisting (name newExisting taken by NewExisting)",
		"// skipped: StateCount (name stateCount taken by State.Count)",
		"// skipped: Bad (unsupported value type chan int)",
		"// skipped: Type (reserved go4t name type; use --prefix)",
		"// skipped: Huge (constant overflows int)",
		"// skipped: HugeFloat (constant overflows float64)",
		"// skipped: Complex (unsupported value type untyped complex)",
	} {
		if !strings.Contains(decls, want) {
			t.Errorf("missing %q:\n%s", want, decls)
		}
	}
	if strings.Count(decls, "extern func newExisting(") != 1 || strings.Count(decls, "extern func stateCount(") != 1 {
		t.Errorf("duplicate bindings:\n%s", decls)
	}
	prefixed, _, _, err := BindWithShim(pkg, BindOptions{Prefix: "native", Float: true}, "example.test/bindings/shim_values", "shim_values")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"extern func nativeNewClient() Client", "extern func nativeDefaultClient() Client", "extern func nativeType() Int"} {
		if !strings.Contains(prefixed, want) {
			t.Errorf("missing prefixed %q:\n%s", want, prefixed)
		}
	}
	writeBindingTestFile(t, dir, "shim_values/shim.go", string(shim))
	writeBindingTestFile(t, dir, "shim_values/values_test.go", `package shim_values_test
import (
 "context"
 "testing"
 "go4t/rt"
 v "example.test/bindings/values"
 s "example.test/bindings/shim_values"
)
func TestValues(t *testing.T) {
 first,second := s.New_Client().(*v.Client),s.New_Client().(*v.Client)
 if first == second || first.Tag != "" || first.Blocked != nil { t.Fatal("constructor did not allocate a fresh zero value") }
 if _,err := s.Client_SetTag(first,"changed").(rt.IO)(context.Background()); err != nil { t.Fatal(err) }
 if first.Tag != "changed" || second.Tag != "" { t.Fatal("constructor boxes share state") }
 if s.DefaultClient() != v.DefaultClient { t.Fatal("global pointer identity lost") }
 v.DefaultClient = &v.Client{Tag: "replacement"}
 if s.DefaultClient() != v.DefaultClient { t.Fatal("getter cached the old package variable") }
 if s.Count() != int64(1) { t.Fatal("initial variable") }; v.Count = 2
 if s.Count() != int64(2) { t.Fatal("getter did not observe variable mutation") }
 if s.MethodGet() != "GET" || s.Enabled() != true || s.Number() != int64(7) || s.Rune() != int64('a') || s.Decimal() != float64(1.25) || s.MaxUint() != int64(-1) { t.Fatal("constant conversion") }
 if s.Callback().(func(any) any)(int64(2)) != int64(3) { t.Fatal("function variable conversion") }
}
`)
	runBindingTestGo(t, dir, "test", "./shim_values")
}
