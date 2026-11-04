package extern

import (
	"strings"
	"testing"
)

func TestDirectImportSourceUpcasts(t *testing.T) {
	const fixture = "go4t/extern/testdata/upcastfixture/"
	dir := bindingTestModule(t)
	loader := Loader{Dir: dir}
	pkg, err := loader.Load(fixture + "p")
	if err != nil {
		t.Fatal(err)
	}
	for _, prefix := range []string{"", "native"} {
		decls, shim, _, err := BindWithShim(pkg, BindOptions{Prefix: prefix}, "example.test/bindings/shim", "shim")
		if err != nil {
			t.Fatal(err)
		}
		for _, tc := range []struct{ name, receiver, shim string }{
			{"ImplAsI", "Impl", "Impl_AsI"},
			{"QImplAsI", "go_go4t_extern_testdata_upcastfixture_q.Impl", "QImpl_AsI"},
			{"RecordAsI", "go_go4t_extern_testdata_upcastfixture_q.Record", "Record_AsI"},
			{"ScalarAsI", "String", "Scalar_AsI"},
		} {
			name, _ := bindingName(tc.name, prefix)
			want := "extern func " + name + "(recv " + tc.receiver + ") I = \"example.test/bindings/shim\"." + tc.shim
			if !strings.Contains(decls, want) {
				t.Errorf("missing %q:\n%s", want, decls)
			}
		}
		for _, forbidden := range []string{"// skipped:", "transitive", "unused", "genericAsI", "constraintAsI"} {
			if strings.Contains(decls, forbidden) {
				t.Errorf("unexpected %q in bindings:\n%s", forbidden, decls)
			}
		}
		if prefix == "" {
			writeBindingTestFile(t, dir, "shim/shim.go", string(shim))
		}
	}
	writeBindingTestFile(t, dir, "shim/upcasts_test.go", `package shim_test
import (
 "testing"
 "go4t/rt"
 p "go4t/extern/testdata/upcastfixture/p"
 q "go4t/extern/testdata/upcastfixture/q"
 s "example.test/bindings/shim"
)
func TestOwners(t *testing.T) {
 own,foreign := &p.Impl{},&q.Impl{}
 if got:=s.Impl_AsI(own); got!=own || got.(p.I).Speak()!="p" { t.Fatal("own source lost identity") }
 if got:=s.QImpl_AsI(foreign); got!=foreign || got.(p.I).Speak()!="q" { t.Fatal("foreign source lost identity") }
 rt.RegisterForeignRecord("go4t/extern/testdata/upcastfixture/q.Record",func(fields []any) any { return fields },func(v any,i int) any { return v.([]any)[i] })
 if s.Record_AsI([]any{int64(7)}).(p.I).Speak()!="record" { t.Fatal("foreign record upcast") }
 if got,ok:=s.Scalar_AsI("foreign").(q.Scalar); !ok || got!="foreign" { t.Fatalf("foreign scalar upcast = %T(%v)",got,got) }
}
`)
	runBindingTestGo(t, dir, "test", "./shim")
}
