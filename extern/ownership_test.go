package extern

import (
	"strings"
	"testing"
)

func TestOwningPackageBindings(t *testing.T) {
	dir := bindingTestModule(t)
	writeBindingTestFile(t, dir, "owner/owner.go", `package owner
// Record has no constructor or method: its owning module must still declare it.
type Record struct { Count int }
type Handle struct { Next *Handle; Name string }
type Speaker interface { Speak() string }
type Constraint interface { ~string }
type Generic[T any] interface { Speak() T }
type Value struct { Name string }
func (v *Value) Speak() string { return v.Name }
type Collision struct { Name string }
func (v *Collision) Speak() string { return v.Name }
func CollisionAsSpeaker() {}
`)
	writeBindingTestFile(t, dir, "consumer/consumer.go", `package consumer
import "example.test/bindings/owner"
func Echo(r owner.Record) owner.Record { return r }
func Handle(h *owner.Handle) *owner.Handle { return h }
`)
	loader := Loader{Dir: dir}
	for _, name := range []string{"owner", "consumer"} {
		pkg, err := loader.Load("example.test/bindings/" + name)
		if err != nil {
			t.Fatal(err)
		}
		decls, shim, err := BindWithShim(pkg, BindOptions{}, "example.test/bindings/shim_"+name, "shim_"+name)
		if err != nil {
			t.Fatal(err)
		}
		var want, forbidden []string
		if name == "owner" {
			want = []string{
				"type Record = Record{count: Int} deriving Eq, Debug",
				"type Handle = HandleHandle",
				`extern func valueAsSpeaker(recv Value) Speaker = "example.test/bindings/shim_owner".Value_AsSpeaker`,
				"// skipped: Collision.AsSpeaker (upcast name collides with existing binding)",
			}
			forbidden = []string{`"go:example.test/bindings/owner"`, "AsConstraint(", "AsGeneric(", "speakerAsSpeaker("}
		} else {
			want = []string{
				`import go_example_test_bindings_owner "go:example.test/bindings/owner"`,
				"extern func echo(p0 go_example_test_bindings_owner.Record) go_example_test_bindings_owner.Record",
				"extern func handle(p0 go_example_test_bindings_owner.Handle) IO[go_example_test_bindings_owner.Handle]",
			}
			forbidden = []string{"type Record =", "type Handle =", `"go:example.test/bindings/consumer"`}
			if !strings.Contains(string(shim), `rt.ForeignRecord("example.test/bindings/shim_consumer.Record"`) {
				t.Errorf("foreign record registration key lost source name:\n%s", shim)
			}
		}
		for _, text := range want {
			if !strings.Contains(decls, text) {
				t.Errorf("%s: missing %q:\n%s", name, text, decls)
			}
		}
		for _, text := range forbidden {
			if strings.Contains(decls, text) {
				t.Errorf("%s: unexpected %q:\n%s", name, text, decls)
			}
		}
		writeBindingTestFile(t, dir, "shim_"+name+"/shim.go", string(shim))
	}
	writeBindingTestFile(t, dir, "shim_consumer/ownership_test.go", `package shim_consumer_test
import (
 "testing"
 "go4t/rt"
 o "example.test/bindings/shim_owner"
 c "example.test/bindings/shim_consumer"
 "example.test/bindings/owner"
)
func TestOwnedRecord(t *testing.T) {
 for _,key := range []string{"example.test/bindings/shim_consumer.Record","example.test/bindings/shim_owner.Value"} {
  rt.RegisterForeignRecord(key,func(fields []any) any { return fields },func(v any, i int) any { return v.([]any)[i] })
 }
 if got := c.Echo([]any{int64(9)}).([]any)[0]; got != int64(9) { t.Fatalf("record round-trip: %v",got) }
 speaker := o.Value_AsSpeaker([]any{"owned"}).(owner.Speaker)
 if speaker.Speak() != "owned" { t.Fatal("record upcast did not preserve fields") }
}
`)
	runBindingTestGo(t, dir, "test", "./shim_consumer")
}
