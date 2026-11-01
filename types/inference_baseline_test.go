package types_test

import (
	"encoding/json"
	"flag"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"regexp"
	"strings"
	"testing"

	"go4t/ast"
	"go4t/modules"
	"go4t/parser"
	"go4t/types"
)

var updateInferenceBaseline = flag.Bool("update", false, "update the inference baseline before changing inference")

const inferenceCaptureEnv = "GO4T_INFERENCE_CAPTURE"

type inferenceSnapshot map[string]map[string]string

type inferenceCapture struct {
	Types     inferenceSnapshot   `json:"types"`
	Libraries map[string][]string `json:"libraries"`
}

var inferenceCaptured = inferenceSnapshot{}
var inferenceCalls = map[string]int{}
var inferenceLibraries = map[string][]string{}

var inferredLetName = regexp.MustCompile(`\bLet[0-9]+\b`)

func TestMain(m *testing.M) {
	code := m.Run()
	if path := os.Getenv(inferenceCaptureEnv); path != "" {
		data, err := json.Marshal(inferenceCapture{Types: inferenceCaptured, Libraries: inferenceLibraries})
		if err == nil {
			err = os.WriteFile(path, append(data, '\n'), 0644)
		}
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			code = 1
		}
	}
	os.Exit(code)
}

// checkFixture keeps the existing fixture assertions and captures every successful
// check, including repeated checks and fixtures built dynamically by their tests.
func checkFixture(t *testing.T, input []*ast.Module) (*types.Program, error) {
	t.Helper()
	p, err := types.Check(input)
	assertDiagnosticNames(t, p, err)
	if os.Getenv(inferenceCaptureEnv) != "" {
		name := t.Name()
		inferenceCalls[name]++
		if err == nil {
			key := fmt.Sprintf("%s/check%d", name, inferenceCalls[name])
			inferenceCaptured[key] = snapshotInference(p)
			inferenceLibraries[key] = libraryInferenceKeys(p, inferenceCaptured[key])
		}
	}
	return p, err
}

func snapshotInference(p *types.Program) map[string]string {
	out := map[string]string{}
	for name, f := range p.Funcs {
		out["func/"+name] = types.TypeString(f.Scheme.Type)
	}
	index := p.PositionIndex()
	var walk func(reflect.Value, string, string)
	walk = func(v reflect.Value, path, file string) {
		if !v.IsValid() {
			return
		}
		if v.Kind() == reflect.Interface {
			if !v.IsNil() {
				walk(v.Elem(), path, file)
			}
			return
		}
		if v.Kind() == reflect.Pointer {
			if v.IsNil() {
				return
			}
			if e, ok := v.Interface().(ast.Expr); ok && *e.AnnSlot() != nil {
				out[path] = types.TypeString(types.AnnOf(e).Type)
			}
			if pat, ok := v.Interface().(*ast.PVar); ok {
				if binding := index.At(file, pat.Pos); binding != nil {
					out[path+"/binding/"+pat.Name] = types.TypeString(binding.Type)
				}
			}
			walk(v.Elem(), path, file)
			return
		}
		switch v.Kind() {
		case reflect.Struct:
			for i := range v.NumField() {
				name := v.Type().Field(i).Name
				// Semantic annotations have back-links; Pipe is the untyped
				// source spelling of an already visited desugared expression.
				if name != "Ann" && name != "Pipe" {
					walk(v.Field(i), path+"/"+name, file)
				}
			}
		case reflect.Slice:
			for i := range v.Len() {
				walk(v.Index(i), fmt.Sprintf("%s/%d", path, i), file)
			}
		}
	}
	for i, f := range p.Files {
		for j, d := range f.Decls {
			name := fmt.Sprintf("%T/%d", d, j)
			if fn, ok := d.(*ast.FuncDecl); ok {
				name = "func/" + fn.Name
			}
			walk(reflect.ValueOf(d), fmt.Sprintf("file%d/%s", i, name), f.Name)
		}
	}
	return out
}

// Keep embedded standard-library files and the fixture prelude separate from
// user annotations. Classification is metadata only: compaction never changes
// any frozen TypeString value.
func libraryInferenceKeys(p *types.Program, bindings map[string]string) []string {
	files := map[*ast.File]bool{}
	for _, module := range p.Modules {
		if strings.HasPrefix(module.ImportPath, "std/") {
			for _, file := range module.Files {
				files[file] = true
			}
		}
	}
	prefixes := map[string]bool{}
	functions := map[*ast.FuncDecl]bool{}
	for i, file := range p.Files {
		if !files[file] && file.Source != testPrelude {
			continue
		}
		prefixes[fmt.Sprintf("file%d", i)] = true
		for _, declaration := range file.Decls {
			if function, ok := declaration.(*ast.FuncDecl); ok {
				functions[function] = true
			}
		}
	}
	var keys []string
	for key := range bindings {
		prefix, name, _ := strings.Cut(key, "/")
		if prefixes[prefix] || prefix == "func" && functions[p.Funcs[name].Decl] {
			keys = append(keys, key)
		}
	}
	return keys
}

// The child executes the real fixture tests, rather than attempting to extract
// source strings from Go code. No production checker hook or second algorithm is
// involved. The parent compares the immutable pre-change observations; new
// fixtures and new expression paths may be added without rewriting that baseline.
func TestInferenceBaseline(t *testing.T) {
	if os.Getenv(inferenceCaptureEnv) != "" {
		checkInferenceExamples(t)
		return
	}
	capture := filepath.Join(t.TempDir(), "capture.json")
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(executable, "-test.run=.", "-test.timeout=2m")
	cmd.Env = append(os.Environ(), inferenceCaptureEnv+"="+capture)
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("fixture capture: %v\n%s", err, output)
	}
	data, err := os.ReadFile(capture)
	if err != nil {
		t.Fatal(err)
	}
	var captured inferenceCapture
	if err := json.Unmarshal(data, &captured); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join("testdata", "inference_baseline.json.gz")
	if *updateInferenceBaseline {
		if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			t.Fatal(err)
		}
		if err := writeInferenceArchive(path, captured); err != nil {
			t.Fatal(err)
		}
	}
	want, err := readInferenceArchive(path)
	if err != nil {
		t.Fatal(err)
	}
	got := captured.Types
	for fixture, bindings := range want {
		renamed, original := map[string]string{}, map[string]string{}
		for binding, typ := range bindings {
			if actual, ok := got[fixture][binding]; !ok || !sameInferredType(typ, actual, renamed, original) {
				t.Errorf("%s %s: type = %q (present %t), baseline %q", fixture, binding, actual, ok, typ)
			}
		}
	}
	t.Logf("preserved %d successful fixture checks", len(want))
}

// Only generated LetN variables may be alpha-renamed. The maps are shared by
// every binding/annotation in one fixture, so distinct variables cannot merge
// and one variable cannot change identity between expression annotations.
func sameInferredType(want, got string, renamed, original map[string]string) bool {
	if !reflect.DeepEqual(inferredLetName.Split(want, -1), inferredLetName.Split(got, -1)) {
		return false
	}
	before, after := inferredLetName.FindAllString(want, -1), inferredLetName.FindAllString(got, -1)
	for i, name := range before {
		replacement := after[i]
		if old, ok := renamed[name]; ok && old != replacement {
			return false
		}
		if old, ok := original[replacement]; ok && old != name {
			return false
		}
		renamed[name], original[replacement] = replacement, name
	}
	return true
}

func TestInferenceBaselineAlphaRenaming(t *testing.T) {
	for _, tc := range []struct {
		want, got string
		equal     bool
	}{
		{"func(Let20) Let20", "func(Let35) Let35", true},
		{"Let20", "Let35", true},
		{"Let20", "Let36", false},
		{"Let21", "Let35", false},
		{"func(A) A", "func(B) B", false},
		{"List[Int]", "List[Float]", false},
		{"Let20Extra", "Let35Extra", false},
	} {
		// Keep the successful first mapping across all subsequent probes.
		renamed, original := map[string]string{"Let20": "Let35"}, map[string]string{"Let35": "Let20"}
		if got := sameInferredType(tc.want, tc.got, renamed, original); got != tc.equal {
			t.Errorf("%q versus %q: equal = %t", tc.want, tc.got, got)
		}
	}
}

func checkInferenceExamples(t *testing.T) {
	err := filepath.WalkDir("../examples", func(path string, entry fs.DirEntry, err error) error {
		if err != nil || !entry.IsDir() {
			return err
		}
		files, err := filepath.Glob(filepath.Join(path, "*.gt"))
		if err != nil || len(files) == 0 {
			return err
		}
		var helpers, mains []string
		for _, file := range files {
			source, err := os.ReadFile(file)
			if err != nil {
				return err
			}
			parsed, err := parser.Parse(string(source))
			if err != nil {
				return err
			}
			main := false
			for _, decl := range parsed.Decls {
				if fn, ok := decl.(*ast.FuncDecl); ok && fn.Name == "main" {
					main = true
				}
			}
			if main {
				mains = append(mains, file)
			} else {
				helpers = append(helpers, file)
			}
		}
		for _, main := range mains {
			t.Run(filepath.ToSlash(main), func(t *testing.T) {
				loaded, err := modules.Load(append([]string{main}, helpers...))
				if err != nil {
					t.Fatal(err)
				}
				_, err = checkFixture(t, loaded)
				// These are intentionally invalid diagnostic programs and a
				// pre-existing formatting-only fixture with an unbound pure.
				negative := strings.HasPrefix(filepath.Base(main), "bad_") || filepath.ToSlash(main) == "../examples/fmt/before.gt"
				if err != nil && !negative {
					t.Fatal(err)
				}
			})
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
