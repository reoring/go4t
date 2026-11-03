package extern

import (
	"strings"
	"testing"
)

func TestJSONScalarShimRoundTrip(t *testing.T) {
	dir := bindingTestModule(t)
	// Implementing the interfaces does not require importing encoding/json.
	writeBindingTestFile(t, dir, "nojson/nojson.go", `package nojson
type Scalar struct { private int }
func (Scalar) MarshalJSON() ([]byte,error) { return []byte("null"),nil }
func (*Scalar) UnmarshalJSON([]byte) error { return nil }
func Echo(v Scalar) Scalar { return v }
`)
	loader := Loader{Dir: dir}
	pkg, err := loader.Load("example.test/bindings/nojson")
	if err != nil {
		t.Fatal(err)
	}
	decls, _, _, err := BindWithShim(pkg, BindOptions{}, "example.test/bindings/shim_nojson", "shim_nojson")
	if err != nil || !strings.Contains(decls, "extern func echo(p0 dyn.Value) dyn.Value") {
		t.Fatalf("structural JSON interfaces: %v\n%s", err, decls)
	}
	writeBindingTestFile(t, dir, "fixture/fixture.go", jsonScalarFixture)
	pkg, err = loader.Load("example.test/bindings/fixture")
	if err != nil {
		t.Fatal(err)
	}
	decls, shim, _, err := BindWithShim(pkg, BindOptions{}, "example.test/bindings/shim", "shim")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		`import dyn "std/dyn"`,
		"type Wallet = Wallet{balance: dyn.Value, pending: Option[dyn.Value]} deriving Eq, Debug",
		"extern func echoMoney(p0 dyn.Value) dyn.Value",
		"extern func makeMoney(p0 Int) dyn.Value",
		"extern func readMoney(p0 dyn.Value) Int",
		"extern func echoPointer(p0 Option[dyn.Value]) Option[dyn.Value]",
		"extern func echoWallet(p0 Wallet) Wallet",
		"extern func moneyAdd(recv dyn.Value, p0 Int) Int",
		"extern func echoWire(p0 dyn.Value) dyn.Value",
		"type PointerOnly = PointerOnlyHandle",
		"type MarshalOnly = MarshalOnlyHandle",
		"type UnmarshalOnly = UnmarshalOnlyHandle",
	} {
		if !strings.Contains(decls, want) {
			t.Errorf("missing %q:\n%s", want, decls)
		}
	}
	for _, forbidden := range []string{"type Money =", "type Wire =", "type Generic =", "extern func moneyAdd(recv dyn.Value, p0 Int) IO["} {
		if strings.Contains(decls, forbidden) {
			t.Errorf("unexpected %q:\n%s", forbidden, decls)
		}
	}
	writeBindingTestFile(t, dir, "shim/shim.go", string(shim))
	writeBindingTestFile(t, dir, "shim/shim_test.go", jsonScalarRuntimeTest)
	runBindingTestGo(t, dir, "test", "./shim")
}

const jsonScalarFixture = `package fixture
import (
 "encoding/json"
 "errors"
 "fmt"
 "strconv"
 "strings"
)
type Money struct { cents int }
func (m Money) MarshalJSON() ([]byte,error) {
 if m.cents < 0 { return nil,errors.New("negative money") }
 return json.Marshal(fmt.Sprintf("%d.%02d",m.cents/100,m.cents%100))
}
func (m *Money) UnmarshalJSON(data []byte) error {
 var text string
 if err := json.Unmarshal(data,&text); err != nil { return err }
 parts := strings.Split(text,".")
 if len(parts)!=2 || len(parts[1])!=2 { return errors.New("invalid money") }
 dollars,err := strconv.Atoi(parts[0]); if err != nil { return err }
 cents,err := strconv.Atoi(parts[1]); if err != nil { return err }
 m.cents = dollars*100+cents
 return nil
}
func (m *Money) Add(cents int) int { m.cents += cents; return m.cents }
func MakeMoney(cents int) Money { return Money{cents} }
func ReadMoney(m Money) int { return m.cents }
func EchoMoney(m Money) Money { return m }
func EchoPointer(m *Money) *Money { return m }
type Wallet struct { Balance Money; Pending *Money }
func EchoWallet(v Wallet) Wallet { return v }
type Wire struct { Cents int }
func (v Wire) MarshalJSON() ([]byte,error) { return Money{v.Cents}.MarshalJSON() }
func (v *Wire) UnmarshalJSON(data []byte) error { var m Money; err := m.UnmarshalJSON(data); v.Cents=m.cents; return err }
func EchoWire(v Wire) Wire { return v }
type PointerOnly struct { private int }
func (*PointerOnly) MarshalJSON() ([]byte,error) { return []byte("null"),nil }
func (*PointerOnly) UnmarshalJSON([]byte) error { return nil }
type MarshalOnly struct { private int }
func (MarshalOnly) MarshalJSON() ([]byte,error) { return []byte("null"),nil }
type UnmarshalOnly struct { private int }
func (*UnmarshalOnly) UnmarshalJSON([]byte) error { return nil }
type Generic[T any] struct { private T }
func (Generic[T]) MarshalJSON() ([]byte,error) { return []byte("null"),nil }
func (*Generic[T]) UnmarshalJSON([]byte) error { return nil }
type Invalid struct { private int }
func (Invalid) MarshalJSON() ([]byte,error) { return []byte("{"),nil }
func (*Invalid) UnmarshalJSON([]byte) error { return nil }
func MakeInvalid() Invalid { return Invalid{} }
`

const jsonScalarRuntimeTest = `package shim_test
import (
 "fmt"
 "math"
 "reflect"
 "strings"
 "testing"
 "go4t/rt"
 s "example.test/bindings/shim"
)
type dynamic struct { kind string; value any }
type option struct { value any; present bool }
func TestJSONScalarRoundTrip(t *testing.T) {
 rt.Shapes = rt.ShapeFuncs{
  DynStr: func(v any) any { return dynamic{"str",v} },
  DynView: func(v any) (string,any) { d:=v.(dynamic); return d.kind,d.value },
  Some: func(v any) any { return option{v,true} },
  None: func() any { return option{} },
  OptionValue: func(v any) (any,bool) { o:=v.(option); return o.value,o.present },
 }
 rt.RegisterForeignRecord("example.test/bindings/fixture.Wallet",func(fields []any) any { return fields },func(v any,i int) any { return v.([]any)[i] })
 equal := func(got,want any) { t.Helper(); if !reflect.DeepEqual(got,want) { t.Fatalf("got %#v, want %#v",got,want) } }
 money:=dynamic{"str","12.34"}
 equal(s.MakeMoney(int64(1234)),money)
 equal(s.ReadMoney(money),int64(1234))
 equal(s.EchoMoney(money),money)
 equal(s.Money_Add(money,int64(1)),int64(1235))
 equal(s.ReadMoney(money),int64(1234))
 equal(s.EchoPointer(option{money,true}),option{money,true})
 equal(s.EchoPointer(option{}),option{})
 wallet:=[]any{money,option{dynamic{"str","56.78"},true}}
 equal(s.EchoWallet(wallet),wallet)
 equal(s.EchoWire(money),money)
 panicContains := func(name string,call func(),want string) {
  t.Helper()
  t.Run(name,func(t *testing.T) {
   defer func() { got:=recover(); if got==nil || !strings.Contains(fmt.Sprint(got),want) { t.Errorf("panic = %v, want %q",got,want) } }()
   call()
  })
 }
 panicContains("unmarshal",func(){ s.ReadMoney(dynamic{"str","invalid"}) },"go4t json scalar: example.test/bindings/fixture.Money: invalid money")
 panicContains("marshal_to",func(){ s.ReadMoney(dynamic{"float",math.Inf(1)}) },"go4t json scalar: example.test/bindings/fixture.Money: json: unsupported value: +Inf")
 panicContains("marshal_from",func(){ s.MakeMoney(int64(-1)) },"go4t json scalar: example.test/bindings/fixture.Money: json: error calling MarshalJSON")
 panicContains("invalid_json",func(){ s.MakeInvalid() },"go4t json scalar: example.test/bindings/fixture.Invalid: json: error calling MarshalJSON")
}
`
