package extern

import (
	"reflect"
	"testing"
)

func TestForeignRecordOwnerKeys(t *testing.T) {
	dir := bindingTestModule(t)
	writeBindingTestFile(t, dir, "left/left.go", `package left
 type Status struct { Count int }
 type Option struct { Label string }
`)
	writeBindingTestFile(t, dir, "right/right.go", `package right
 type Status struct { Message string; Ready bool }
`)
	writeBindingTestFile(t, dir, "consumer/consumer.go", `package consumer
 import (
  "example.test/bindings/left"
  "example.test/bindings/right"
 )
 func Echo(a left.Status, b right.Status) (left.Status, right.Status) { return a, b }
 func EchoOption(v left.Option) left.Option { return v }
`)
	loader := Loader{Dir: dir}
	for _, name := range []string{"left", "right", "consumer"} {
		pkg, err := loader.Load("example.test/bindings/" + name)
		if err != nil {
			t.Fatal(err)
		}
		_, shim, metadata, err := BindWithShim(pkg, BindOptions{}, "example.test/bindings/shim_"+name, "shim_"+name)
		if err != nil {
			t.Fatal(err)
		}
		want := map[string]string{}
		switch name {
		case "left":
			want["Status"] = "example.test/bindings/left.Status"
			want["OptionGo"] = "example.test/bindings/left.Option"
		case "right":
			want["Status"] = "example.test/bindings/right.Status"
		}
		if !reflect.DeepEqual(metadata.RecordKeys, want) {
			t.Fatalf("%s record metadata = %v, want %v", name, metadata.RecordKeys, want)
		}
		writeBindingTestFile(t, dir, "shim_"+name+"/shim.go", string(shim))
	}
	writeBindingTestFile(t, dir, "shim_consumer/record_keys_test.go", `package shim_consumer_test
 import (
  "reflect"
  "testing"
  "github.com/reoring/go4t/rt"
  s "example.test/bindings/shim_consumer"
 )
 type leftStatus struct { count int64 }
 type rightStatus struct { message string; ready bool }
 type option struct { label string }
 func TestRoundTrip(t *testing.T) {
  rt.Shapes.Tuple2 = func(a,b any) any { return [2]any{a,b} }
  rt.Shapes.Untuple2 = func(v any) (any,any) { p := v.([2]any); return p[0],p[1] }
  rt.RegisterForeignRecord("example.test/bindings/left.Status",
   func(fields []any) any { return leftStatus{fields[0].(int64)} },
   func(v any, i int) any { return v.(leftStatus).count })
  rt.RegisterForeignRecord("example.test/bindings/right.Status",
   func(fields []any) any { return rightStatus{fields[0].(string),fields[1].(bool)} },
   func(v any, i int) any { r := v.(rightStatus); if i == 0 { return r.message }; return r.ready })
  rt.RegisterForeignRecord("example.test/bindings/left.Option",
   func(fields []any) any { return option{fields[0].(string)} },
   func(v any, i int) any { return v.(option).label })
  a, b := leftStatus{7}, rightStatus{"ready",true}
  if got := s.Echo(a,b); !reflect.DeepEqual(got,[2]any{a,b}) {
   t.Fatalf("same-named records did not round-trip: %#v",got)
  }
  if got := s.EchoOption(option{"prelude collision"}); got != (option{"prelude collision"}) {
   t.Fatalf("Go type name did not round-trip: %#v",got)
  }
 }
`)
	runBindingTestGo(t, dir, "test", "./shim_consumer")
}
