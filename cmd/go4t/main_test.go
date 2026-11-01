package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"go4t/ast"
	"go4t/types"
)

func TestBuildUsesPerProgramDirectories(t *testing.T) {
	dir := t.TempDir()
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	mod := "module buildtest\n\ngo 1.22\n\nrequire go4t v0.0.0\nreplace go4t => " + root + "\n"
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte(mod), 0o644); err != nil {
		t.Fatal(err)
	}
	legacyPath := filepath.Join(dir, "first_gen.go")
	legacy := []byte("user-owned legacy output\n")
	if err := os.WriteFile(legacyPath, legacy, 0o644); err != nil {
		t.Fatal(err)
	}

	for _, name := range []string{"first", "second"} {
		file := filepath.Join(dir, name+".gt")
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
		for _, fragment := range []string{"package main", "return io_println_(\"" + name + "\")"} {
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
			mod := "module warningtest\n\ngo 1.22\n\nrequire go4t v0.0.0\nreplace go4t => " + root + "\n"
			if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte(mod), 0o644); err != nil {
				t.Fatal(err)
			}
			file := filepath.Join(dir, "warning.gt")
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
			want := file + ":3:6: warning: x is unused; prefix with _ to silence\n let x = None\n     ^\n" +
				file + ":3:10: warning: type of None defaulted to Unit; add an annotation or use the value\n let x = None\n         ^~~~\n"
			if string(output) != want {
				t.Errorf("stderr = %q, want %q", output, want)
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
	for _, file := range []string{"", "std/prelude/prelude.gt"} {
		err := &types.Error{Pos: ast.Pos{Line: 3, Col: 3}, Msg: "invalid type", File: file}
		wantFile := file
		if wantFile == "" {
			wantFile = "user.gt"
		}
		want := wantFile + ":3:3: invalid type"
		if file == "std/prelude/prelude.gt" {
			want += "\nimport \"std/list\"\n  ^"
		}
		if got := diagnostic("user.gt", err).Error(); got != want {
			t.Errorf("diagnostic = %q, want %q", got, want)
		}
	}
}

func TestRunUsage(t *testing.T) {
	for _, args := range [][]string{
		nil,
		{"run"},
		{"unknown", "main.gt"},
		{"check", "main.gt", "--"},
		{"build", "main.gt", "--", "argument"},
	} {
		err := run(args)
		if err == nil || err.Error() != usage {
			t.Errorf("run(%q) = %v, want %q", args, err, usage)
		}
	}
}

func TestRunPassesProgramArguments(t *testing.T) {
	if os.Getenv("GO4T_CLI_RUN_ARGS_TEST") == "1" {
		for i, arg := range os.Args {
			if arg == "--" {
				if err := run(os.Args[i+1:]); err != nil {
					t.Fatal(err)
				}
				os.Exit(0)
			}
		}
		t.Fatal("missing helper argument separator")
	}
	dir := t.TempDir()
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	mod := "module argstest\n\ngo 1.22\n\nrequire go4t v0.0.0\nreplace go4t => " + root + "\n"
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte(mod), 0o644); err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(dir, "args.gt")
	source := `package main
import "std/io"
func main() IO[Unit] {
    do {
        values <- io.args()
        println(show(values))
    }
}
`
	if err := os.WriteFile(file, []byte(source), 0o644); err != nil {
		t.Fatal(err)
	}
	for _, tt := range []struct {
		name string
		args []string
		want string
	}{
		{"no arguments", nil, "[]\n"},
		{"empty separator", []string{"--"}, "[]\n"},
		{"arguments", []string{"--", "first", "--flag", "two words", "", "日本", "--", "-n"}, "[first, --flag, two words, , 日本, --, -n]\n"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			args := append([]string{"-test.run=^TestRunPassesProgramArguments$", "--", "run", file}, tt.args...)
			command := exec.Command(os.Args[0], args...)
			command.Env = append(os.Environ(), "GO4T_CLI_RUN_ARGS_TEST=1")
			output, err := command.CombinedOutput()
			if err != nil {
				t.Fatalf("go4t run args program: %v\n%s", err, output)
			}
			if string(output) != tt.want {
				t.Errorf("output = %q, want %q", output, tt.want)
			}
		})
	}
}
