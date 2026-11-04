package extern

import (
	"strings"
	"testing"

	gfx "github.com/reoring/go4t/types"
)

func TestLoadPackages(t *testing.T) {
	var loader Loader
	for _, tc := range []struct{ path, name, signature string }{
		{"strings", "ToUpper", "func(string) string"},
		{"strings", "Repeat", "func(string, int) string"},
		{"strings", "Fields", "func(string) []string"},
		{"github.com/reoring/go4t/rt", "ListFromStrings", "func([]string) any"},
	} {
		pkg, err := loader.Load(tc.path)
		if err != nil {
			t.Fatal(err)
		}
		f, ok := pkg.Lookup(tc.name)
		if !ok {
			t.Fatalf("missing %s.%s", tc.path, tc.name)
		}
		if got := SignatureString(f.Signature); got != tc.signature {
			t.Errorf("%s.%s: %s, want %s", tc.path, tc.name, got, tc.signature)
		}
		again, err := loader.Load(tc.path)
		if err != nil || again != pkg {
			t.Fatalf("loader did not reuse package: %v", err)
		}
		for i := 1; i < len(pkg.Functions); i++ {
			if pkg.Functions[i-1].Name >= pkg.Functions[i].Name {
				t.Fatal("functions not in lexical order")
			}
		}
	}
	pkg, err := loader.Load("github.com/reoring/go4t/extern/testdata/bindfixture")
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := pkg.Lookup("hidden"); ok {
		t.Fatal("unexported function exposed")
	}
}

func TestLoaderDependencies(t *testing.T) {
	dir := bindingTestModule(t)
	writeBindingTestFile(t, dir, "leaf/leaf.go", "package leaf\nfunc Value() string { return \"leaf\" }\n")
	writeBindingTestFile(t, dir, "dep/dep.go", "package dep\nimport \"example.test/bindings/leaf\"\nfunc Value() string { return leaf.Value() }\n")
	writeBindingTestFile(t, dir, "lib/lib.go", "package lib\nimport \"example.test/bindings/dep\"\nfunc Value() string { return dep.Value() }\n")
	loader := Loader{Dir: dir}
	if len(loader.Dependencies("unknown")) != 0 {
		t.Fatal("unknown package has dependencies")
	}
	if _, err := loader.Load("example.test/bindings/lib"); err != nil {
		t.Fatal(err)
	}
	// Loading the root records both edges; loading its transitive dependency
	// subsequently must not need a second go list.
	for parent, child := range map[string]string{"lib": "dep", "dep": "leaf"} {
		got := loader.Dependencies("example.test/bindings/" + parent)
		if len(got) != 1 || got[0] != "example.test/bindings/"+child {
			t.Fatalf("%s dependencies = %q", parent, got)
		}
	}
	t.Setenv("PATH", t.TempDir())
	for _, name := range []string{"dep", "leaf"} {
		if _, err := loader.Load("example.test/bindings/" + name); err != nil {
			t.Fatalf("loading recorded dependency %s: %v", name, err)
		}
	}
}

func TestLoadErrors(t *testing.T) {
	var loader Loader
	for _, path := range []string{"", "-json", "./...", "github.com/reoring/go4t/no-such-extern-package"} {
		if _, err := loader.Load(path); err == nil {
			t.Errorf("Load(%q) succeeded", path)
		}
	}
}

func TestMatchSignatures(t *testing.T) {
	var loader Loader
	pkg, err := loader.Load("github.com/reoring/go4t/extern/testdata/natural")
	if err != nil {
		t.Fatal(err)
	}
	app := func(name string, inner gfx.Type) gfx.Type { return gfx.App(&gfx.TCon{Name: name}, inner) }
	float := &gfx.TCon{Name: "Float"}
	for _, tc := range []struct {
		name   string
		typ    gfx.Type
		kind   Kind
		goType string
	}{
		{"Int", gfx.Int, Scalar, "int64"}, {"NativeInt", gfx.Int, Scalar, "int"},
		{"Float", float, Scalar, "float64"}, {"String", gfx.String, Scalar, "string"},
		{"Bool", gfx.Bool, Scalar, "bool"}, {"Unit", gfx.Unit, Scalar, "rt.Unit"},
		{"IO", app("IO", gfx.Int), Scalar, "rt.IO"},
		{"Strings", app("List", gfx.String), Strings, ""}, {"Ints", app("List", gfx.Int), Ints, ""},
		{"OptionInt", app("Option", gfx.Int), Optional, "int64"},
		{"OptionNativeInt", app("Option", gfx.Int), Optional, "int"},
		{"OptionFloat", app("Option", float), Optional, "float64"},
		{"OptionString", app("Option", gfx.String), Optional, "string"},
		{"OptionBool", app("Option", gfx.Bool), Optional, "bool"},
		{"OptionUnit", app("Option", gfx.Unit), Optional, "rt.Unit"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f, _ := pkg.Lookup(tc.name)
			adapter, err := Match(f, &gfx.TFunc{Params: []gfx.Type{tc.typ}, Ret: tc.typ})
			if err != nil {
				t.Fatal(err)
			}
			want := Value{Kind: tc.kind, Go: tc.goType}
			if adapter.Uniform || adapter.Error || len(adapter.Params) != 1 || adapter.Params[0] != want || adapter.Result != want {
				t.Fatalf("adapter = %#v, want %v", adapter, want)
			}
		})
	}
	f, _ := pkg.Lookup("Echo")
	adapter, err := Match(f, &gfx.TFunc{Params: []gfx.Type{gfx.String}, Ret: gfx.String})
	if err != nil || !adapter.Uniform {
		t.Fatalf("legacy ABI: %#v, %v", adapter, err)
	}
	f, _ = pkg.Lookup("WithError")
	adapter, err = Match(f, &gfx.TFunc{Params: []gfx.Type{gfx.String}, Ret: app("IO", gfx.String)})
	if err != nil || !adapter.Error || adapter.Result.Go != "string" {
		t.Fatalf("IO ABI: %#v, %v", adapter, err)
	}
	f, _ = pkg.Lookup("ErasedWithError")
	unknown := app("List", gfx.Bool)
	adapter, err = Match(f, &gfx.TFunc{Params: []gfx.Type{unknown}, Ret: app("IO", unknown)})
	if err != nil || !adapter.Error || adapter.Params[0].Kind != Erased || adapter.Result.Kind != Erased {
		t.Fatalf("erased fallback: %#v, %v", adapter, err)
	}
	for _, name := range []string{"Wrong", "Mixed", "Variadic", "Generic", "WithError", "OptionString"} {
		f, _ := pkg.Lookup(name)
		_, err := Match(f, &gfx.TFunc{Params: []gfx.Type{gfx.Int}, Ret: gfx.Int})
		if err == nil || !strings.HasPrefix(err.Error(), "extern "+name+": Go signature ") || !strings.HasSuffix(err.Error(), " does not match declared type func(Int) Int") {
			t.Errorf("%s mismatch = %v", name, err)
		}
	}
}
