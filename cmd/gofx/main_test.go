package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestBuildUsesPerProgramDirectories(t *testing.T) {
	dir := t.TempDir()
	legacyPath := filepath.Join(dir, "first_gen.go")
	legacy := []byte("user-owned legacy output\n")
	if err := os.WriteFile(legacyPath, legacy, 0o644); err != nil {
		t.Fatal(err)
	}

	for _, name := range []string{"first", "second"} {
		file := filepath.Join(dir, name+".gfx")
		source := "package main\nfunc main() IO[Unit] { println(\"" + name + "\") }\n"
		if err := os.WriteFile(file, []byte(source), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := run([]string{"build", file}); err != nil {
			t.Fatal(err)
		}
	}

	for _, name := range []string{"first", "second"} {
		output, err := os.ReadFile(filepath.Join(dir, name+"_gen", "main.go"))
		if err != nil {
			t.Fatal(err)
		}
		for _, fragment := range []string{"package main", "return println_(\"" + name + "\")"} {
			if !strings.Contains(string(output), fragment) {
				t.Errorf("%s output does not contain %q", name, fragment)
			}
		}
	}

	gotLegacy, err := os.ReadFile(legacyPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(gotLegacy) != string(legacy) {
		t.Errorf("legacy output changed: got %q, want %q", gotLegacy, legacy)
	}
	if _, err := os.Stat(filepath.Join(dir, "second_gen.go")); !os.IsNotExist(err) {
		t.Errorf("legacy output was created or could not be checked: %v", err)
	}
}
