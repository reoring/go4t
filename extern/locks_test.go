package extern

import (
	"strings"
	"testing"
)

func TestLockValueBoundaries(t *testing.T) {
	dir := bindingTestModule(t)
	loader := Loader{Dir: dir}
	pkg, err := loader.Load("github.com/reoring/go4t/extern/testdata/lockfixture")
	if err != nil {
		t.Fatal(err)
	}
	decls, shim, _, err := BindWithShim(pkg, BindOptions{}, "example.test/bindings/shim", "shim")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"extern func global() Guarded",
		"extern func guardedInc(recv Guarded) IO[Unit]",
		"extern func guardedReadLabel(recv Guarded) IO[String]",
		"extern func holderG(recv Holder) IO[Guarded]",
		"extern func takePointer(p0 Guarded) IO[Unit]",
		"extern func takeLocker(p0 LockerHolder) IO[Unit]",
		"extern func takeValueLocker(p0 ValueLocker) Unit",
		"type PlainHolder = PlainHolder{p: Plain}\n",
		"type PrivateHolder = PrivateHolderHandle\n",
		"extern func privateHolderP(recv PrivateHolder) IO[Plain]",
		"// skipped: Holder.SetG (copies lock value Guarded)",
		"// skipped: TakeArray (copies lock value ArrayGuarded)",
		"// skipped: TakeEmbedded (copies lock value EmbeddedGuarded)",
		"// skipped: GlobalSlice (copies lock value Guarded)",
		"// skipped: LockedChannel (copies lock value Guarded)",
		"// skipped: StreamHolder.Stream (copies lock value Guarded)",
	} {
		if !strings.Contains(decls, want) {
			t.Errorf("missing %q:\n%s", want, decls)
		}
	}
	for _, name := range []string{"Take", "Make", "TakeSlice", "MakeMap", "TakeChannel", "TakeCallback", "MakeCallback"} {
		if want := "// skipped: " + name + " (copies lock value Guarded)"; !strings.Contains(decls, want) {
			t.Errorf("missing %q:\n%s", want, decls)
		}
	}
	for _, forbidden := range []string{"copies lock value LockerHolder", "copies lock value ValueLocker", "extern func holderSetG(", "extern func newGuardedChan("} {
		if strings.Contains(decls, forbidden) {
			t.Errorf("unexpected %q:\n%s", forbidden, decls)
		}
	}
	for _, want := range []string{"From(&go2.Global)", "From(&receiver.G)", "From(&go2.PlainGlobal)", "From(&receiver.P)"} {
		if !strings.Contains(string(shim), want) {
			t.Errorf("missing address-taking %q:\n%s", want, shim)
		}
	}
	writeBindingTestFile(t, dir, "shim/shim.go", string(shim))
	writeBindingTestFile(t, dir, "shim/locks_test.go", `package shim_test
import (
 "context"
 "testing"
 "github.com/reoring/go4t/rt"
 f "github.com/reoring/go4t/extern/testdata/lockfixture"
 s "example.test/bindings/shim"
)
func TestStorageIdentity(t *testing.T) {
 run := func(action any) any { t.Helper(); v,err:=action.(rt.IO)(context.Background()); if err!=nil { t.Fatal(err) }; return v }
 if s.Global()!=&f.Global { t.Fatal("global copied") }
 run(s.Guarded_Inc(s.Global()))
 if f.Global.N!=1 { t.Fatal("method did not mutate global") }
 holder:=&f.Holder{}
 field:=run(s.Holder_G(holder))
 if field!=&holder.G { t.Fatal("field copied") }
 run(s.Guarded_Inc(field))
 if holder.G.N!=1 { t.Fatal("method did not mutate field") }
 holder.G.Label="promoted"
 if run(s.Guarded_ReadLabel(field))!="promoted" { t.Fatal("promoted value method receiver") }
 if s.PlainGlobal()!=&f.PlainGlobal { t.Fatal("non-lock global copied") }
 plain:=&f.PrivateHolder{}
 if run(s.PrivateHolder_P(plain))!=&plain.P { t.Fatal("non-lock field copied") }
}
`)
	runBindingTestGo(t, dir, "test", "./shim")
	// Vet the generated package, not the fixture's deliberately unsafe Go APIs.
	runBindingTestGo(t, dir, "vet", "./shim")
}
