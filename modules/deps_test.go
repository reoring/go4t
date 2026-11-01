package modules_test

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"go4t/modules"
	"go4t/types"
)

func writeSources(t *testing.T, root string, sources map[string]string) {
	t.Helper()
	for name, source := range sources {
		file := filepath.Join(root, name)
		if err := os.MkdirAll(filepath.Dir(file), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(file, []byte(source), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

func dependencyProject(t *testing.T) (string, string) {
	t.Helper()
	t.Setenv("GOPROXY", "off")
	t.Setenv("GOWORK", "off")
	t.Setenv("GOFLAGS", "-mod=mod")
	t.Setenv("GO4T_PATH", "")
	library, err := filepath.Abs("testdata/deplib")
	if err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	writeSources(t, root, map[string]string{
		"go.mod": "module example.com/app\n\ngo 1.22\n\nrequire example.com/lib v1.0.0\nreplace example.com/lib => " + strconv.Quote(library) + "\n",
		"go4t.mod": "module example.com/app\n\nrequire example.com/lib v1.0.0\n",
		"app/main.gt": `package main
import "example.com/lib"
import "example.com/lib/shapes"
func main() IO[Unit] { println(lib.name() + ": " + show(shapes.area(6, 7))) }
`,
	})
	return root, library
}

func TestDependenciesResolveOfflineAndFromNestedEntry(t *testing.T) {
	root, library := dependencyProject(t)
	loaded, err := modules.Load([]string{filepath.Join(root, "app")})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := types.Check(loaded); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{library, filepath.Join(library, "shapes"), filepath.Join(library, "dimensions"), "std/prelude"} {
		count := 0
		for _, module := range loaded {
			if module.Path == want {
				count++
			}
		}
		if count != 1 {
			t.Errorf("module %s loaded %d times; want once", want, count)
		}
	}
}

func TestDependencyResolutionPrecedence(t *testing.T) {
	for _, local := range []bool{false, true} {
		t.Run(strconv.FormatBool(local), func(t *testing.T) {
			root, library := dependencyProject(t)
			writeSources(t, root, map[string]string{"app/main.gt": "package main\nimport \"example.com/lib/shapes\"\nfunc main() IO[Unit] { println(show(shapes.area(6, 7))) }\n"})
			search := t.TempDir()
			writeSources(t, search, map[string]string{"example.com/lib/shapes/broken.gt": "invalid GO4T_PATH source"})
			t.Setenv("GO4T_PATH", search)
			want := filepath.Join(library, "shapes")
			if local {
				want = filepath.Join(root, "example.com/lib/shapes")
				writeSources(t, root, map[string]string{"example.com/lib/shapes/local.gt": "package shapes\nfunc area(x Int, y Int) Int { x + y }\n"})
			}
			loaded, err := modules.Load([]string{filepath.Join(root, "app")})
			if err != nil {
				t.Fatal(err)
			}
			entry := loaded[len(loaded)-1]
			if got := entry.Files[0].Imports[0].Resolved; got != want {
				t.Fatalf("resolved %s, want %s", got, want)
			}
		})
	}
}

func TestDependencyMissingSubdirectoryFallsThrough(t *testing.T) {
	root, _ := dependencyProject(t)
	search := t.TempDir()
	writeSources(t, search, map[string]string{"example.com/lib/extra/main.gt": "package extra\nfunc value() Int { 42 }\n"})
	t.Setenv("GO4T_PATH", search)
	writeSources(t, root, map[string]string{"app/main.gt": "package main\nimport \"example.com/lib/extra\"\nfunc main() IO[Unit] { println(show(extra.value())) }\n"})
	if _, err := modules.Load([]string{filepath.Join(root, "app")}); err != nil {
		t.Fatal(err)
	}
}

func TestDependencyFailuresDoNotFallThrough(t *testing.T) {
	for _, failure := range []string{"empty", "invalid", "unresolved", "prefix boundary"} {
		t.Run(failure, func(t *testing.T) {
			root, _ := dependencyProject(t)
			writeSources(t, root, map[string]string{"app/main.gt": "package main\nimport \"example.com/lib/shapes\"\nfunc main() IO[Unit] { println(show(shapes.area(6, 7))) }\n"})
			search := t.TempDir()
			writeSources(t, search, map[string]string{"example.com/lib/shapes/main.gt": "package shapes\nfunc area(x Int, y Int) Int { x * y }\n"})
			t.Setenv("GO4T_PATH", search)
			want := ""
			switch failure {
			case "empty":
				if err := os.MkdirAll(filepath.Join(root, "example.com/lib/shapes"), 0o755); err != nil {
					t.Fatal(err)
				}
				want = "no .gt files in " + filepath.Join(root, "example.com/lib/shapes")
			case "invalid":
				library := t.TempDir()
				writeSources(t, library, map[string]string{"go.mod": "module example.com/lib\ngo 1.22\n", "lib.gt": "package lib\n", "shapes/broken.gt": "invalid dependency source"})
				writeSources(t, root, map[string]string{"go.mod": "module example.com/app\ngo 1.22\nrequire example.com/lib v1.0.0\nreplace example.com/lib => " + strconv.Quote(library) + "\n"})
				want = "broken.gt"
			case "unresolved":
				writeSources(t, root, map[string]string{"go.mod": "module example.com/app\ngo 1.22\n"})
				want = "resolve dependency example.com/lib: go list -m -json example.com/lib:"
			case "prefix boundary":
				writeSources(t, root, map[string]string{"app/main.gt": "package main\nimport \"example.com/library/shapes\"\nfunc main() IO[Unit] { ioPure(unit()) }\n"})
				want = "module \"example.com/library/shapes\" not found"
			}
			_, err := modules.Load([]string{filepath.Join(root, "app")})
			if err == nil || !strings.Contains(err.Error(), want) {
				t.Fatalf("error=%v; want %q", err, want)
			}
		})
	}
}

func TestTransitiveDependenciesUseProjectReplacements(t *testing.T) {
	root, library := dependencyProject(t)
	first := t.TempDir()
	writeSources(t, first, map[string]string{
		"go.mod": "module example.com/first\ngo 1.22\nrequire example.com/lib v1.0.0\n",
		"go4t.mod": "module example.com/first\nrequire example.com/lib v1.0.0\n",
		"first.gt": "package first\nimport \"example.com/lib/shapes\"\nfunc answer() Int { shapes.area(6, 7) }\n",
	})
	writeSources(t, root, map[string]string{
		"go.mod": "module example.com/app\ngo 1.22\nrequire example.com/first v1.0.0\nrequire example.com/lib v1.1.0\nreplace example.com/first => " + strconv.Quote(first) + "\nreplace example.com/lib => " + strconv.Quote(library) + "\n",
		"go4t.mod": "module example.com/app\nrequire example.com/first v1.0.0\n",
		"app/main.gt": "package main\nimport \"example.com/first\"\nfunc main() IO[Unit] { println(show(first.answer())) }\n",
	})
	loaded, err := modules.Load([]string{filepath.Join(root, "app")})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := types.Check(loaded); err != nil {
		t.Fatal(err)
	}
}

func TestNestedRequirementUsesLongestPrefix(t *testing.T) {
	root, _ := dependencyProject(t)
	nested := t.TempDir()
	writeSources(t, nested, map[string]string{"go.mod": "module example.com/lib/shapes\ngo 1.22\n", "go4t.mod": "module example.com/lib/shapes\n", "shapes.gt": "package shapes\nfunc area(x Int, y Int) Int { x * y }\n"})
	file := filepath.Join(root, "go.mod")
	data, err := os.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}
	writeSources(t, root, map[string]string{
		"go.mod": string(data) + "require example.com/lib/shapes v1.0.0\nreplace example.com/lib/shapes => " + strconv.Quote(nested) + "\n",
		"go4t.mod": "module example.com/app\nrequire example.com/lib v1.0.0\nrequire example.com/lib/shapes v1.0.0\n",
	})
	loaded, err := modules.Load([]string{filepath.Join(root, "app")})
	if err != nil {
		t.Fatal(err)
	}
	if got := loaded[len(loaded)-1].Files[0].Imports[1].Resolved; got != nested {
		t.Fatalf("nested module resolved %q, want %q", got, nested)
	}
}

func TestManifestParsingAndUpdates(t *testing.T) {
	root := t.TempDir()
	writeSources(t, root, map[string]string{"go4t.mod": "// library dependencies\nmodule example.com/app\n\nrequire example.com/lib v1.0.0 // local\nrequire example.com/other v2.0.0\n"})
	manifest, err := modules.ReadManifest(root)
	if err != nil {
		t.Fatal(err)
	}
	manifest.SetRequire("example.com/lib", "v1.1.0")
	manifest.SetRequire("example.com/new", "v1.0.0")
	want := "// library dependencies\nmodule example.com/app\n\nrequire example.com/lib v1.1.0 // local\nrequire example.com/other v2.0.0\nrequire example.com/new v1.0.0\n"
	if got := string(manifest.Bytes()); got != want {
		t.Fatalf("manifest=%q, want %q", got, want)
	}
}

func TestInvalidManifestErrors(t *testing.T) {
	for _, tt := range []struct{ source, want string }{
		{"", "expected module <name>"},
		{"require example.com/lib v1.0.0\n", "expected module <name>"},
		{"module ../escape\n", "expected module <name>"},
		{"module example.com/app\nmodule other\n", "line 2: expected require <path> <version>"},
		{"module example.com/app\nrequire example.com/lib\n", "line 2: expected require <path> <version>"},
		{"module example.com/app\nreplace example.com/lib => ../lib\n", "line 2: expected require <path> <version>"},
		{"module example.com/app\nrequire example.com/lib v1.0.0\nrequire example.com/lib v1.1.0\n", "duplicate require example.com/lib"},
	} {
		root := t.TempDir()
		writeSources(t, root, map[string]string{"go4t.mod": tt.source})
		_, err := modules.ReadManifest(root)
		if want := "invalid go4t.mod in " + root + ": " + tt.want; err == nil || err.Error() != want {
			t.Fatalf("ReadManifest(%q)=%v; want %q", tt.source, err, want)
		}
	}
}
