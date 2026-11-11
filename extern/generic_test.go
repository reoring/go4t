package extern

import (
	"github.com/reoring/go4t/internal/testgo"
	"go/types"
	"strings"
	"testing"

	"github.com/reoring/go4t/ast"
)

func genericType(name string, args ...ast.Type) *ast.TypeName {
	return &ast.TypeName{Name: name, Args: args}
}
func genericFunc(params []ast.Type, ret ast.Type) *ast.FuncType {
	return &ast.FuncType{Params: params, Ret: genericType("IO", ret)}
}

func TestGenericMirrors(t *testing.T) {
	testgo.Require(t, 23, "binds maps.Keys and slices.Sorted")
	var loader Loader
	integer, text, boolean := genericType("Int"), genericType("String"), genericType("Bool")
	for _, tc := range []struct {
		pkg, symbol  string
		signature    *ast.FuncType
		want, reason string
	}{
		{"slices", "Contains", genericFunc([]ast.Type{genericType("List", integer), integer}, boolean), "slices.Contains[[]int64, int64]", ""},
		{"slices", "Min", genericFunc([]ast.Type{genericType("List", genericType("Float"))}, genericType("Float")), "slices.Min[[]float64, float64]", ""},
		{"maps", "Keys", genericFunc([]ast.Type{genericType("Map", text, integer)}, genericType("List", text)), "maps.Keys[map[string]int64, string, int64]", ""},
		{"sync", "OnceValue", genericFunc([]ast.Type{genericFunc(nil, integer)}, genericFunc(nil, integer)), "sync.OnceValue[int64]", ""},
		{"slices", "Contains", genericFunc([]ast.Type{genericType("List", genericType("List", integer)), genericType("List", integer)}, boolean), "", "does not satisfy comparable"},
		{"slices", "Min", genericFunc([]ast.Type{genericType("List", boolean)}, boolean), "", "does not satisfy cmp.Ordered"},
		{"slices", "Sorted", genericFunc([]ast.Type{genericType("List", integer)}, genericType("List", integer)), "", "iterator parameters are unsupported"},
	} {
		t.Run(tc.pkg+"."+tc.symbol+tc.reason, func(t *testing.T) {
			pkg, err := loader.Load(tc.pkg)
			if err != nil {
				t.Fatal(err)
			}
			instance, err := InstantiateGeneric(&loader, pkg, tc.symbol, tc.signature)
			if tc.reason != "" {
				if err == nil || !strings.Contains(err.Error(), tc.reason) {
					t.Fatalf("got %v, want %s", err, tc.reason)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			parts := make([]string, len(instance.Arguments))
			for n, typ := range instance.Arguments {
				parts[n] = types.TypeString(typ, nil)
			}
			actual := tc.pkg + "." + tc.symbol + "[" + strings.Join(parts, ", ") + "]"
			if actual != tc.want {
				t.Fatalf("got %s, want %s", actual, tc.want)
			}
			shim, err := instance.Shim("fixture/instance")
			if err != nil || !strings.Contains(string(shim), "ctx.Err()") {
				t.Fatalf("effect boundary: %v\n%s", err, shim)
			}
		})
	}
}

func TestGenericInterfaceAndCallbackShim(t *testing.T) {
	dir := bindingTestModule(t)
	writeBindingTestFile(t, dir, "fixture/fixture.go", `package fixture
import "context"
type Label struct { Text string }
func (l Label) String() string { return l.Text }
func Stringify[T interface{ String() string }](value T) string { return value.String() }
func Invoke[T any](f func(context.Context) (T,error)) (T,error) { return f(context.Background()) }
func Retain[T any](f func() T) func() T { return f }
func Echo[T any](value T) T { return value }
func Zero[T any]() T { var zero T; return zero }
`)
	loader := Loader{Dir: dir}
	pkg, err := loader.Load("example.test/bindings/fixture")
	if err != nil {
		t.Fatal(err)
	}
	label := genericType("go:example.test/bindings/fixture.Label")
	text, integer := genericType("String"), genericType("Int")
	contextType := genericType("go:context.Context")
	for _, tc := range []struct {
		symbol string
		sig    *ast.FuncType
	}{
		{"Stringify", genericFunc([]ast.Type{label}, text)},
		{"Invoke", genericFunc([]ast.Type{genericFunc(nil, integer)}, integer)},
		{"Retain", genericFunc([]ast.Type{genericFunc(nil, integer)}, genericFunc(nil, integer))},
		{"Echo", genericFunc([]ast.Type{contextType}, contextType)},
		{"Zero", genericFunc(nil, integer)},
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
	if _, err := InstantiateGeneric(&loader, pkg, "Stringify", genericFunc([]ast.Type{integer}, text)); err == nil || !strings.Contains(err.Error(), "missing method String") {
		t.Fatalf("method constraint: %v", err)
	}
	writeBindingTestFile(t, dir, "Retain/shim_test.go", `package instance_test
import (
 "context"
 "errors"
 "testing"
 "github.com/reoring/go4t/rt"
 retain "example.test/bindings/Retain"
 invoke "example.test/bindings/Invoke"
 stringify "example.test/bindings/Stringify"
 echo "example.test/bindings/Echo"
 zero "example.test/bindings/Zero"
)
func TestGenericCallbacks(t *testing.T) {
 rt.RegisterForeignRecord("example.test/bindings/fixture.Label",func(xs []any) any { return xs },func(v any,i int) any { return v.([]any)[i] })
 value,err := stringify.Stringify([]any{"named"}).(rt.IO)(context.Background())
 if err != nil || value != "named" { t.Fatalf("record: %v %v",value,err) }
 ctx := context.Background()
 echoed,err := echo.Echo(ctx).(rt.IO)(ctx)
 if err != nil || echoed != ctx { t.Fatalf("generic context argument: %v %v",echoed,err) }
 value,err = zero.Zero().(rt.IO)(ctx)
 if err != nil || value != int64(0) { t.Fatalf("result-guided inference: %v %v",value,err) }
 calls := 0
 action := retain.Retain(func() any { return rt.IO(func(ctx context.Context)(any,error) { calls++; return int64(calls),nil }) }).(rt.IO)
 if calls != 0 { t.Fatal("callback executed during construction") }
 cancelled,cancel := context.WithCancel(context.Background()); cancel()
 if _,err := action(cancelled); err == nil || calls != 0 { t.Fatal("cancelled construction ran") }
 raw,err := action(context.Background()); if err != nil { t.Fatal(err) }
 callback := raw.(func() any)
 if calls != 0 { t.Fatal("retained callback ran early") }
 later := callback().(rt.IO)
 if _,err := later(cancelled); err == nil || calls != 0 { t.Fatal("cancelled callback ran") }
 got,err := later(context.Background()); if err != nil || got != int64(1) { t.Fatalf("callback: %v %v",got,err) }
 _,err = invoke.Invoke(func() any { return rt.IO(func(context.Context)(any,error) { return nil,errors.New("callback failed") }) }).(rt.IO)(context.Background())
 if err == nil || err.Error() != "callback failed" { t.Fatalf("callback failure: %v",err) }
}
`)
	runBindingTestGo(t, dir, "test", "./Retain")
}
