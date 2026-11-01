package extern

import (
	"strings"
	"testing"

	gfx "gofx/types"
)

func TestLoadPackages(t *testing.T) {
	var loader Loader
	for _, tc := range []struct{ path, name, signature string }{
		{"strings", "ToUpper", "func(string) string"},
		{"strings", "Repeat", "func(string, int) string"},
		{"strings", "Fields", "func(string) []string"},
		{"gofx/rt/stdhttp", "GetStatus", "func(string) gofx/rt.IO"},
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
	pkg, err := loader.Load("gofx/extern/testdata/bindfixture")
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := pkg.Lookup("hidden"); ok {
		t.Fatal("unexported function exposed")
	}
}

func TestLoadErrors(t *testing.T) {
	var loader Loader
	for _, path := range []string{"", "-json", "./...", "gofx/no-such-extern-package"} {
		if _, err := loader.Load(path); err == nil {
			t.Errorf("Load(%q) succeeded", path)
		}
	}
}

func TestMatchSignatures(t *testing.T) {
	var loader Loader
	pkg, err := loader.Load("gofx/extern/testdata/natural")
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
