package types_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gofx/ast"
	"gofx/modules"
	"gofx/std"
	"gofx/types"
)

func moduleFixture(t *testing.T, sources map[string]string) string {
	t.Helper()
	root := t.TempDir()
	for name, source := range sources {
		file := filepath.Join(root, name)
		if err := os.MkdirAll(filepath.Dir(file), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(file, []byte(source), 0644); err != nil {
			t.Fatal(err)
		}
	}
	return root
}
func checkModuleFixture(t *testing.T, root string) (*types.Program, error) {
	t.Helper()
	loaded, err := modules.Load([]string{root})
	if err != nil {
		return nil, err
	}
	return checkFixture(t, loaded)
}
func TestModulePrivacyAndFileScopes(t *testing.T) {
	for _, test := range []struct{ name, body, want string }{
		{"value", "geometry._helper()", "_helper is private to module"},
		{"type", "{ let x: geometry._Secret = geometry.secret(); 0 }", "_Secret is private to module"},
		{"constructor", "match geometry.secret() { geometry._Hidden => 0 }", "_Hidden is private to module"},
		{"unknown export", "geometry.missing()", "has no exported name missing"},
	} {
		t.Run(test.name, func(t *testing.T) {
			root := moduleFixture(t, map[string]string{
				"main.gfx":             "package main\nimport \"geometry\"\nfunc main() IO[Unit] { let n = " + test.body + "; println(show(n)) }",
				"geometry/private.gfx": "package geometry\ntype _Secret = _Hidden\nfunc _helper() Int { 1 }\nfunc secret() _Secret { _Hidden }",
			})
			_, err := checkModuleFixture(t, root)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("Check = %v, want %s", err, test.want)
			}
		})
	}
	root := moduleFixture(t, map[string]string{"main.gfx": "package main\nimport \"geometry\"\nfunc main() IO[Unit] { ioPure(unit()) }", "other.gfx": "package main\nfunc other() Int { geometry.value() }", "geometry/main.gfx": "package geometry\nfunc value() Int { 1 }"})
	if _, err := checkModuleFixture(t, root); err == nil || !strings.Contains(err.Error(), "unknown identifier geometry") {
		t.Fatalf("file-scoped imports: %v", err)
	}
}
func TestModuleGlobalInstancesAndDuplicates(t *testing.T) {
	sources := map[string]string{
		"main.gfx":       "package main\nimport \"model\"\nimport \"first\"\nfunc main() IO[Unit] { println(show(model.Box(3))) }",
		"model/type.gfx": "package model\ntype Box = Box(Int)",
		"first/show.gfx": "package first\nimport \"model\"\ninstance Show[model.Box] { show(b) { match b { model.Box(n) => show(n) } } }",
	}
	root := moduleFixture(t, sources)
	if _, err := checkModuleFixture(t, root); err != nil {
		t.Fatal(err)
	}
	sources["main.gfx"] = "package main\nimport \"first\"\nimport \"second\"\nfunc main() IO[Unit] { ioPure(unit()) }"
	sources["second/show.gfx"] = "package second\nimport \"model\"\ninstance Show[model.Box] { show(b) { \"second\" } }"
	root = moduleFixture(t, sources)
	_, err := checkModuleFixture(t, root)
	if err == nil || !strings.Contains(err.Error(), "duplicate instance Show[") || !strings.Contains(err.Error(), "first") || !strings.Contains(err.Error(), "second") {
		t.Fatalf("duplicate modules: %v", err)
	}
}
func TestModulePreludeShadowAndNarrowExports(t *testing.T) {
	root := moduleFixture(t, map[string]string{
		"main.gfx": "package main\nfunc map(n Int) Int { n + 1 }\nfunc main() IO[Unit] { println(show(map(3))) }",
	})
	loaded, err := modules.Load([]string{root})
	if err != nil {
		t.Fatal(err)
	}
	for range 2 {
		p, err := checkFixture(t, loaded)
		if err != nil {
			t.Fatal(err)
		}
		if len(p.Warnings) != 1 || p.Warnings[0].Msg != "shadows prelude name map" {
			t.Fatalf("warnings: %#v", p.Warnings)
		}
		if loaded[len(loaded)-1].Files[0].Decls[0].(*ast.FuncDecl).Name != "map" {
			t.Fatal("checker mutated source names")
		}
	}
	root = moduleFixture(t, map[string]string{"main.gfx": "package main\nfunc main() IO[Unit] { println(trim(\" x \")) }"})
	if _, err := checkModuleFixture(t, root); err == nil || !strings.Contains(err.Error(), "unknown identifier trim") {
		t.Fatalf("prelude leaked trim: %v", err)
	}
}
func TestModuleResolutionRootThenPathThenEmbedded(t *testing.T) {
	stringSource, err := std.FS.ReadFile("string/string.gfx")
	if err != nil {
		t.Fatal(err)
	}
	pathRoot := moduleFixture(t, map[string]string{"geometry/value.gfx": "package geometry\nfunc value() Bool { true }", "std/string/string.gfx": string(stringSource) + "\nfunc marker() Int { 7 }\n"})
	t.Setenv("GOFX_PATH", pathRoot)
	root := moduleFixture(t, map[string]string{"gofx.mod": "module example\n", "app/main.gfx": "package main\nimport \"example/geometry\"\nfunc main() IO[Unit] { println(show(geometry.value() + 1)) }", "geometry/value.gfx": "package geometry\nfunc value() Int { 2 }"})
	if _, err := checkModuleFixture(t, filepath.Join(root, "app")); err != nil {
		t.Fatalf("nearest gofx.mod root precedence: %v", err)
	}
	root = moduleFixture(t, map[string]string{"main.gfx": "package main\nimport \"geometry\"\nimport \"std/string\"\nfunc main() IO[Unit] { println(show(geometry.value() && string.marker() == 7)) }"})
	if _, err := checkModuleFixture(t, root); err != nil {
		t.Fatalf("GOFX_PATH fallback and embedded prelude: %v", err)
	}
	t.Setenv("GOFX_PATH", "")
	root = moduleFixture(t, map[string]string{"main.gfx": "package main\nimport \"std/string\"\nfunc main() IO[Unit] { println(string.trim(\" works \")) }"})
	if _, err := checkModuleFixture(t, root); err != nil {
		t.Fatalf("embedded fallback: %v", err)
	}
}
func TestModuleQualifiedRecordsAndAlias(t *testing.T) {
	root := moduleFixture(t, map[string]string{
		"main.gfx":            "package main\nimport g \"geometry\"\nimport \"geometry\"\nfunc area(p g.Point) Int { match p { geometry.Point{x: x, y: y} => x * y } }\nfunc main() IO[Unit] { println(show(area(g.Point{x: 3, y: 4}))) }",
		"geometry/type.gfx":   "package geometry\ntype Point = Point{x: Int, y: Int}",
		"geometry/helper.gfx": "package geometry\nfunc width(p Point) Int { p.x }",
	})
	if _, err := checkModuleFixture(t, root); err != nil {
		t.Fatal(err)
	}
}

func TestStandardOverridesKeepPreludeIdentity(t *testing.T) {
	for _, location := range []string{"source-root", "GOFX_PATH"} {
		for _, name := range []string{"prelude", "list", "result", "io", "float"} {
			t.Run(location+"/"+name, func(t *testing.T) {
				source, err := std.FS.ReadFile(name + "/" + name + ".gfx")
				if err != nil {
					t.Fatal(err)
				}
				importPath := "std/" + name
				sources := map[string]string{
					"gofx.mod": "module example\n",
					"app/main.gfx": "package main\nimport selected \"" + importPath + "\"\n" + `
func main() IO[Unit] {
  let xs: List[Int] = [1, 2]
  let result: Result[String, Int] = Ok(length(map(xs, func(n) => n + 1)))
  do {
    println(show(result))
    println(show(toFloat(selected.overrideMarker())))
  }
}
`,
				}
				override := map[string]string{importPath + "/" + name + ".gfx": string(source) + "\nfunc overrideMarker() Int { 42 }\n"}
				var overrideRoot string
				if location == "source-root" {
					for path, contents := range override {
						sources[path] = contents
					}
					sources["app/main.gfx"] = strings.Replace(sources["app/main.gfx"], "func main()", "import same \"example/"+importPath+"\"\nfunc duplicatePath() Int { same.overrideMarker() }\nfunc main()", 1)
					t.Setenv("GOFX_PATH", moduleFixture(t, map[string]string{importPath + "/broken.gfx": "invalid source must lose to source root"}))
				} else {
					overrideRoot = moduleFixture(t, override)
					losingRoot := moduleFixture(t, map[string]string{importPath + "/broken.gfx": "invalid source must lose to earlier GOFX_PATH entry"})
					t.Setenv("GOFX_PATH", overrideRoot+string(os.PathListSeparator)+losingRoot)
				}
				root := moduleFixture(t, sources)
				if overrideRoot == "" {
					overrideRoot = root
				}
				loaded, err := modules.Load([]string{filepath.Join(root, "app")})
				if err != nil {
					t.Fatal(err)
				}
				wantOwner := filepath.Join(overrideRoot, filepath.FromSlash(importPath))
				count := 0
				for _, module := range loaded {
					if module.Path == wantOwner {
						count++
					}
				}
				if count != 1 {
					t.Fatalf("filesystem override identity %s loaded %d times", wantOwner, count)
				}
				program, err := checkFixture(t, loaded)
				if err != nil {
					t.Fatal(err)
				}
				if len(program.Warnings) != 0 {
					t.Fatalf("standard definitions incorrectly shadow prelude: %v", program.Warnings)
				}
				intrinsic := map[string]string{"prelude": "Option", "list": "List", "result": "Result", "io": "println", "float": "toFloat"}[name]
				if program.Owners[intrinsic] != wantOwner {
					t.Fatalf("%s owner = %q, want %q", intrinsic, program.Owners[intrinsic], wantOwner)
				}
			})
		}
	}
}
