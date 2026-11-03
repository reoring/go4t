package extern

import (
	"strings"
	"testing"
)

func TestBindingNamePrecedence(t *testing.T) {
	dir := bindingTestModule(t)
	writeBindingTestFile(t, dir, "fixture/fixture.go", `package fixture
type Speaker interface { Speak() string }
type Thing struct { Name string; AsSpeaker string; SetName int; Blocked chan int }
func (*Thing) Speak() string { return "thing" }
func (*Thing) Read() int { return 1 }
func ThingRead() int { return 2 }
func ThingSetName() int { return 3 }
var ThingName = "package value"
var ThingAsSpeaker = 4
type Other struct { private int }
func (*Other) Speak() string { return "other" }
func NewOther() *Other { return &Other{} }
var OtherAsSpeaker = 5
type Construct struct { private int }
var NewConstruct = 6
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
		for _, tc := range []struct{ loser, name, winner string }{
			{"Thing.Read", "ThingRead", "ThingRead"},
			{"Thing.Name", "ThingSetName", "ThingSetName"},
			{"Thing.SetName", "ThingSetName", "ThingSetName"},
			{"ThingName", "ThingName", "Thing.Name"},
			{"ThingAsSpeaker", "ThingAsSpeaker", "Thing.AsSpeaker"},
			{"Thing.AsSpeaker", "ThingAsSpeaker", "Thing.AsSpeaker"},
			{"Other.AsSpeaker", "OtherAsSpeaker", "OtherAsSpeaker"},
			{"NewOther", "NewOther", "NewOther"},
			{"NewConstruct", "NewConstruct", "NewConstruct"},
		} {
			name, _ := bindingName(tc.name, prefix)
			want := "// skipped: " + tc.loser + " (name " + name + " taken by " + tc.winner + ")"
			if !strings.Contains(decls, want) {
				t.Errorf("missing %q:\n%s", want, decls)
			}
			if strings.Count(decls, "extern func "+name+"(") != 1 {
				t.Errorf("expected one winner for %s:\n%s", name, decls)
			}
		}
		if prefix == "" {
			writeBindingTestFile(t, dir, "shim/shim.go", string(shim))
		}
	}
	writeBindingTestFile(t, dir, "shim/shim_test.go", `package shim_test
import (
 "context"
 "testing"
 "go4t/rt"
 f "example.test/bindings/fixture"
 s "example.test/bindings/shim"
)
func TestWinningBindings(t *testing.T) {
 receiver:=&f.Thing{Name:"field",AsSpeaker:"accessor"}
 value,err:=s.Thing_Name(receiver).(rt.IO)(context.Background())
 if err!=nil || value!="field" { t.Fatalf("accessor lost to package value: %v %v",value,err) }
 value,err=s.Thing_AsSpeaker(receiver).(rt.IO)(context.Background())
 if err!=nil || value!="accessor" { t.Fatalf("accessor lost to upcast: %v %v",value,err) }
 if s.OtherAsSpeaker()!=int64(5) { t.Fatal("upcast beat value getter") }
 if s.NewConstruct()!=int64(6) { t.Fatal("constructor beat earlier value getter") }
}
`)
	runBindingTestGo(t, dir, "test", "./shim")
}

func TestUpcastOwnerDisambiguation(t *testing.T) {
	dir := bindingTestModule(t)
	for _, entry := range []struct{ path, method string }{
		{"meta/v1", "Meta"}, {"runtime", "Runtime"}, {"a/runtime", "A"}, {"b/runtime", "B"},
	} {
		writeBindingTestFile(t, dir, entry.path+"/interface.go", "package owner\ntype Object interface { "+entry.method+"() string }\n")
	}
	writeBindingTestFile(t, dir, "fixture/fixture.go", `package fixture
import (
 meta "example.test/bindings/meta/v1"
 runtime "example.test/bindings/runtime"
 a "example.test/bindings/a/runtime"
 b "example.test/bindings/b/runtime"
)
type Thing struct { private int }
func (*Thing) Meta() string { return "meta" }
func (*Thing) Runtime() string { return "runtime" }
type Collision struct { private int }
func (*Collision) A() string { return "a" }
func (*Collision) B() string { return "b" }
var _ meta.Object = (*Thing)(nil)
var _ runtime.Object = (*Thing)(nil)
var _ a.Object = (*Collision)(nil)
var _ b.Object = (*Collision)(nil)
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
		for _, goName := range []string{"ThingAsMetaV1Object", "ThingAsRuntimeObject", "CollisionAsExampleTestBindingsARuntimeObject", "CollisionAsExampleTestBindingsBRuntimeObject"} {
			name, _ := bindingName(goName, prefix)
			if !strings.Contains(decls, "extern func "+name+"(") {
				t.Errorf("missing upcast %s:\n%s", name, decls)
			}
		}
		if strings.Contains(decls, "// skipped:") || strings.Contains(decls, "extern func thingAsObject(") {
			t.Errorf("upcast ambiguity left unresolved:\n%s", decls)
		}
		if prefix == "" {
			writeBindingTestFile(t, dir, "shim/shim.go", string(shim))
		}
	}
	writeBindingTestFile(t, dir, "shim/shim_test.go", `package shim_test
import (
 "testing"
 f "example.test/bindings/fixture"
 s "example.test/bindings/shim"
)
func TestQualifiedUpcasts(t *testing.T) {
 v:=&f.Thing{}
 if s.Thing_AsMetaV1Object(v).(interface{Meta() string}).Meta()!="meta" { t.Fatal("meta upcast") }
 if s.Thing_AsRuntimeObject(v).(interface{Runtime() string}).Runtime()!="runtime" { t.Fatal("runtime upcast") }
 c:=&f.Collision{}
 if s.Collision_AsExampleTestBindingsARuntimeObject(c).(interface{A() string}).A()!="a" { t.Fatal("full alias a upcast") }
 if s.Collision_AsExampleTestBindingsBRuntimeObject(c).(interface{B() string}).B()!="b" { t.Fatal("full alias b upcast") }
}
`)
	runBindingTestGo(t, dir, "test", "./shim")
}
