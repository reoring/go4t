package codegen

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"gofx/ast"
	"gofx/modules"
	"gofx/parser"
	"gofx/types"
)

func TestNaturalExternAdapters(t *testing.T) {
	p := fixture()
	list(p)
	option(p)
	app := func(name string, inner types.Type) types.Type { return types.App(&types.TCon{Name: name}, inner) }
	float := &types.TCon{Name: "Float"}
	for _, tc := range []struct {
		name string
		typ  types.Type
	}{
		{"Int", types.Int}, {"NativeInt", types.Int}, {"String", types.String},
		{"Bool", types.Bool}, {"Unit", types.Unit}, {"Float", float},
		{"IO", app("IO", types.Int)}, {"Strings", app("List", types.String)}, {"Ints", app("List", types.Int)},
		{"OptionInt", app("Option", types.Int)}, {"OptionNativeInt", app("Option", types.Int)},
		{"OptionFloat", app("Option", float)}, {"OptionString", app("Option", types.String)},
		{"OptionBool", app("Option", types.Bool)}, {"OptionUnit", app("Option", types.Unit)},
		{"Echo", app("List", types.Bool)},
	} {
		extern(p, "typed"+tc.name, []string{"value"}, []types.Type{tc.typ}, "gofx/extern/testdata/natural", tc.name)
		p.Funcs["typed"+tc.name].Scheme.Type.Ret = tc.typ
	}
	for _, tc := range []struct {
		name string
		typ  types.Type
	}{
		{"WithError", types.String}, {"StringsWithError", app("List", types.String)},
		{"IntsWithError", app("List", types.Int)}, {"ErasedWithError", app("List", types.Bool)},
	} {
		extern(p, "typed"+tc.name, []string{"value"}, []types.Type{tc.typ}, "gofx/extern/testdata/natural", tc.name)
		p.Funcs["typed"+tc.name].Scheme.Type.Ret = app("IO", tc.typ)
	}
	extern(p, "httpStatus", []string{"url"}, []types.Type{types.String}, "gofx/rt/stdhttp", "GetStatus")
	p.Funcs["httpStatus"].Scheme.Type.Ret = app("IO", types.Int)
	source, err := generate(p, "main")
	if err != nil {
		t.Fatal(err)
	}
	for _, fragment := range []string{
		"value_.(int64)", "int(value_.(int64))", "value_.(float64)", "value_.(string)", "value_.(bool)",
		"value_.(rt.Unit)", "value_.(rt.IO)", "rt.ListFromStrings(", "rt.ListToStrings(value_)",
		"rt.ListFromInts(", "rt.ListToInts(value_)", "rt.SomeOf(", "rt.NoneOf()", "value_.(*Option_Some)",
		"return rt.IO(func() (any, error)",
	} {
		if !strings.Contains(string(source), fragment) {
			t.Errorf("missing adapter fragment %q", fragment)
		}
	}
	dir := t.TempDir()
	root, err := filepath.Abs("..")
	if err != nil {
		t.Fatal(err)
	}
	files := map[string]string{
		"go.mod":       "module adaptertest\n\ngo 1.22\nrequire gofx v0.0.0\nreplace gofx => " + filepath.ToSlash(root) + "\n",
		"main.go":      string(source),
		"main_test.go": adapterRuntimeTests,
	}
	for name, content := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	command := exec.Command("go", "test", "-vet=off", ".")
	command.Dir = dir
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("generated adapters: %v\n%s", err, output)
	}
}

func TestExternMismatchDiagnostics(t *testing.T) {
	for _, tc := range []struct{ symbol, want string }{
		{"Int", "extern Int: Go signature func(int64) int64 does not match declared type func(String) Bool"},
		{"Missing", "extern Missing: exported Go function not found in gofx/extern/testdata/natural"},
	} {
		p := fixture()
		extern(p, "foreign", []string{"value"}, []types.Type{types.String}, "gofx/extern/testdata/natural", tc.symbol)
		p.Funcs["foreign"].Decl.Pos = ast.Pos{Line: 3, Col: 7}
		p.Funcs["foreign"].Scheme.Type.Ret = types.Bool
		_, err := generate(p, "main")
		if err == nil || err.Error() != "3:7: "+tc.want {
			t.Errorf("error = %v, want 3:7: %s", err, tc.want)
		}
	}
}

func TestHTTPExampleUsesNaturalExtern(t *testing.T) {
	source, err := os.ReadFile("../examples/http/main.gfx")
	if err != nil {
		t.Fatal(err)
	}
	file, err := parser.Parse(string(source))
	if err != nil {
		t.Fatal(err)
	}
	modules, err := modules.LoadFiles([]*ast.File{file}, ".")
	if err != nil {
		t.Fatal(err)
	}
	program, err := types.Check(modules)
	if err != nil {
		t.Fatal(err)
	}
	generated, err := generate(program, "main")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(generated), ".GetStatus(url_.(string))") {
		t.Fatalf("HTTP example did not adapt its natural extern")
	}
}

const adapterRuntimeTests = `package main
import (
    "net/http"
    "net/http/httptest"
    "reflect"
    "testing"
    "gofx/rt"
    foreign "gofx/extern/testdata/natural"
)
func TestAdapters(t *testing.T) {
    for _, tc := range []struct { call func(any) any; value any }{
        {typedInt_, int64(-9223372036854775808)}, {typedNativeInt_, int64(42)},
        {typedFloat_, 1.25}, {typedString_, "日本語"}, {typedBool_, false}, {typedUnit_, rt.Unit{}},
    } {
        if got := tc.call(tc.value); got != tc.value { t.Errorf("roundtrip = %#v, want %#v", got, tc.value) }
    }
    action := rt.IO(func() (any, error) { return int64(7), nil })
    if got, err := typedIO_(action).(rt.IO)(); got != int64(7) || err != nil { t.Fatalf("IO = %v, %v", got, err) }
    for _, values := range [][]string{nil, {"one", "", "日本"}} {
        got := rt.ListToStrings(typedStrings_(rt.ListFromStrings(values)))
        if !reflect.DeepEqual(got, values) { t.Fatalf("strings = %q, want %q", got, values) }
    }
    for _, values := range [][]int64{nil, {-1, 0, 9223372036854775807}} {
        got := rt.ListToInts(typedInts_(rt.ListFromInts(values)))
        if !reflect.DeepEqual(got, values) { t.Fatalf("ints = %v, want %v", got, values) }
    }
    for _, tc := range []struct { call func(any) any; value any }{
        {typedOptionInt_, int64(0)}, {typedOptionNativeInt_, int64(-4)},
        {typedOptionFloat_, 1.5}, {typedOptionString_, ""}, {typedOptionBool_, false}, {typedOptionUnit_, rt.Unit{}},
    } {
        if _, ok := tc.call(&Option_None{}).(*Option_None); !ok { t.Fatal("None not preserved") }
        if got := tc.call(&Option_Some{F0: tc.value}).(*Option_Some).F0; got != tc.value { t.Errorf("Some = %#v", got) }
    }
    erased := &List_Cons{F0: true, F1: &List_Nil{}}
    if typedEcho_(erased) != erased { t.Fatal("any forwarding changed identity") }
    foreign.Calls = 0
    good := typedWithError_("ok").(rt.IO)
    bad := typedWithError_("bad").(rt.IO)
    if foreign.Calls != 0 { t.Fatal("Go effect executed before IO") }
    if got, err := good(); got != "ok" || err != nil { t.Fatalf("good = %v, %v", got, err) }
    if got, err := bad(); got != nil || !reflect.DeepEqual(err, &rt.Failure{Kind: rt.FailureKind, Message: "fixture failure"}) { t.Fatalf("bad = %v, %v", got, err) }
    if _, err := good(); err != nil || foreign.Calls != 3 { t.Fatalf("IO replay: calls %d, err %v", foreign.Calls, err) }
    for _, tc := range []struct { action any; want any }{
        {typedStringsWithError_(rt.ListFromStrings([]string{"ok"})), rt.ListFromStrings([]string{"ok"})},
        {typedIntsWithError_(rt.ListFromInts([]int64{1})), rt.ListFromInts([]int64{1})},
        {typedErasedWithError_(erased), erased},
    } {
        got, err := tc.action.(rt.IO)()
        if err != nil || !reflect.DeepEqual(got, tc.want) { t.Fatalf("boxed IO result: %#v, %v", got, err) }
    }
}
func TestHTTPAdapter(t *testing.T) {
    requests := 0
    server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { requests++; w.WriteHeader(418) }))
    defer server.Close()
    action := httpStatus_(server.URL).(rt.IO)
    if requests != 0 { t.Fatal("HTTP effect before IO") }
    if status, err := action(); status != int64(418) || err != nil { t.Fatalf("HTTP result = %v, %v", status, err) }
    server.Close()
    status, err := action()
    failure, ok := err.(*rt.Failure)
    if status != nil || !ok || failure.Kind != rt.Network { t.Fatalf("HTTP error result = %v, %v", status, err) }
}
`
