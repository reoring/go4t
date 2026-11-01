package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"gofx/ast"
	"gofx/types"
)

func TestBuildUsesPerProgramDirectories(t *testing.T) {
	dir := t.TempDir()
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	mod := "module buildtest\n\ngo 1.22\n\nrequire gofx v0.0.0\nreplace gofx => " + root + "\n"
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte(mod), 0o644); err != nil {
		t.Fatal(err)
	}
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
		rel, err := filepath.Rel(dir, filepath.Join(dir, name+"_gen"))
		if err != nil {
			t.Fatal(err)
		}
		vet := exec.Command("go", "vet", "./"+filepath.ToSlash(rel))
		vet.Dir = dir
		if output, err := vet.CombinedOutput(); err != nil {
			t.Fatalf("go vet generated %s: %v\n%s", name, err, output)
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

func TestWarningsContinueForEveryCommand(t *testing.T) {
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	for _, command := range []string{"check", "build", "run"} {
		t.Run(command, func(t *testing.T) {
			dir := t.TempDir()
			mod := "module warningtest\n\ngo 1.22\n\nrequire gofx v0.0.0\nreplace gofx => " + root + "\n"
			if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte(mod), 0o644); err != nil {
				t.Fatal(err)
			}
			file := filepath.Join(dir, "warning.gfx")
			source := "package main\nfunc main() IO[Unit] {\n let x = None\n ioPure(unit())\n}\n"
			if err := os.WriteFile(file, []byte(source), 0o644); err != nil {
				t.Fatal(err)
			}
			stderr, err := os.CreateTemp(dir, "stderr-")
			if err != nil {
				t.Fatal(err)
			}
			previous := os.Stderr
			os.Stderr = stderr
			t.Cleanup(func() {
				os.Stderr = previous
				stderr.Close()
			})
			runErr := run([]string{command, file})
			os.Stderr = previous
			if err := stderr.Close(); err != nil {
				t.Fatal(err)
			}
			output, err := os.ReadFile(stderr.Name())
			if err != nil {
				t.Fatal(err)
			}
			if runErr != nil {
				t.Fatalf("%s with warnings failed: %v\n%s", command, runErr, output)
			}
			pattern := "^" + regexp.QuoteMeta(file) + `:3:[1-9][0-9]*: warning: type of (let x|None) defaulted to Unit; add an annotation or use the value` + "\n$"
			if !regexp.MustCompile(pattern).Match(output) {
				t.Errorf("stderr = %q, want a positioned defaulting warning matching %q", output, pattern)
			}
			if command != "check" {
				if _, err := os.Stat(filepath.Join(dir, "warning_gen", "main.go")); err != nil {
					t.Fatalf("%s did not generate output: %v", command, err)
				}
			}
		})
	}
}

func TestDiagnosticUsesCheckerFilename(t *testing.T) {
	for _, file := range []string{"", "prelude.gfx"} {
		err := &types.Error{Pos: ast.Pos{Line: 2, Col: 3}, Msg: "invalid type", File: file}
		wantFile := file
		if wantFile == "" {
			wantFile = "user.gfx"
		}
		if got := diagnostic("user.gfx", err).Error(); got != wantFile+":2:3: invalid type" {
			t.Errorf("diagnostic = %q, want checker file %q", got, wantFile)
		}
	}
}
