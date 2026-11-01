package codegen

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"gofx/modules"
	"gofx/std"
	"gofx/types"
)

func TestGenerateFilesAndRuntimeShapes(t *testing.T) {
	loaded, err := modules.Load([]string{"../examples/modules"})
	if err != nil {
		t.Fatal(err)
	}
	program, err := types.Check(loaded)
	if err != nil {
		t.Fatal(err)
	}
	generated, err := Generate(program, "main")
	if err != nil {
		t.Fatal(err)
	}
	if len(generated) != len(loaded) {
		t.Fatalf("got %d files for %d modules", len(generated), len(loaded))
	}
	geometry := string(generated["geometry.go"])
	if !strings.Contains(geometry, "func geometry_squaredDistance_") || !strings.Contains(geometry, "func geometry_translate_") || strings.Contains(geometry, "func main()") {
		t.Fatalf("geometry ownership:\n%s", geometry)
	}
	main := string(generated["main.go"])
	for _, text := range []string{"rt.Shapes = rt.ShapeFuncs", "list_List_Cons", "prelude_Option_Some", "result_Result_Ok", "Tuple2", "geometry_squaredDistance_"} {
		if !strings.Contains(main, text) {
			t.Errorf("entry missing %q", text)
		}
	}
	want := "point: (3, 4)\nsquared distance: 25\ntranslated: Point{x: 4, y: 6}\nlocal map: 42\n"
	if output := runGeneratedModules(t, generated); output != want {
		t.Fatalf("output = %q, want %q", output, want)
	}
}

func runGeneratedModules(t *testing.T, generated map[string][]byte) string {
	t.Helper()
	dir := t.TempDir()
	root, err := filepath.Abs("..")
	if err != nil {
		t.Fatal(err)
	}
	for name, source := range generated {
		if err := os.WriteFile(filepath.Join(dir, name), source, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module moduletest\n\ngo 1.22\n\nrequire gofx v0.0.0\nreplace gofx => "+root+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	command := exec.Command("go", "run", ".")
	command.Dir = dir
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("generated modules: %v\n%s", err, output)
	}
	return string(output)
}

func TestGenerateFilesystemStandardOverrides(t *testing.T) {
	for _, location := range []string{"source-root", "GOFX_PATH"} {
		t.Run(location, func(t *testing.T) {
			entry := t.TempDir()
			standard := entry
			t.Setenv("GOFX_PATH", "")
			if location == "GOFX_PATH" {
				standard = t.TempDir()
				t.Setenv("GOFX_PATH", standard)
			}
			for _, name := range []string{"prelude", "list", "result", "io", "float"} {
				source, err := std.FS.ReadFile(name + "/" + name + ".gfx")
				if err != nil {
					t.Fatal(err)
				}
				dir := filepath.Join(standard, "std", name)
				if err := os.MkdirAll(dir, 0o755); err != nil {
					t.Fatal(err)
				}
				source = append(source, []byte("\nfunc overrideMarker() String { \""+name+"\" }\n")...)
				if err := os.WriteFile(filepath.Join(dir, name+".gfx"), source, 0o644); err != nil {
					t.Fatal(err)
				}
			}
			const source = `package main
import p "std/prelude"
import l "std/list"
import r "std/result"
import io "std/io"
import f "std/float"
import s "std/string"
func main() IO[Unit] {
  do {
    println(p.overrideMarker() + " " + l.overrideMarker() + " " + r.overrideMarker() + " " + io.overrideMarker() + " " + f.overrideMarker())
    println(show(map(s.split(",", "1,2"), func(text) => s.parseInt(text))))
    outcome <- io.tryIO(ioPure(f.toFloat(3)))
    println(show(outcome))
  }
}
`
			if err := os.WriteFile(filepath.Join(entry, "main.gfx"), []byte(source), 0o644); err != nil {
				t.Fatal(err)
			}
			loaded, err := modules.Load([]string{entry})
			if err != nil {
				t.Fatal(err)
			}
			program, err := types.Check(loaded)
			if err != nil {
				t.Fatal(err)
			}
			if len(program.Warnings) != 0 {
				t.Fatalf("unexpected warnings: %v", program.Warnings)
			}
			generated, err := Generate(program, "main")
			if err != nil {
				t.Fatal(err)
			}
			want := "prelude list result io float\n[Some(1), Some(2)]\nOk(3)\n"
			if output := runGeneratedModules(t, generated); output != want {
				t.Fatalf("output = %q, want %q", output, want)
			}
		})
	}
}
