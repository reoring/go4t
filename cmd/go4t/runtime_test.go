package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestCommandsOutsideModule(t *testing.T) {
	for _, withModule := range []bool{false, true} {
		name := "no module"
		if withModule {
			name = "unrelated module"
		}
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			temporary := t.TempDir()
			t.Setenv("TMPDIR", temporary)
			const mod = "module unrelated\n\ngo 1.22\n"
			if withModule {
				toolsWrite(t, filepath.Join(dir, "go.mod"), mod)
			}
			file := toolsWrite(t, filepath.Join(dir, "main.gt"), `package main
import "greeting"
func main() IO[Unit] { println(greeting.message()) }
func testGreeting() Bool { greeting.message() == "hi" }
func answer() Int { 42 }
`)
			if err := os.Mkdir(filepath.Join(dir, "greeting"), 0o755); err != nil {
				t.Fatal(err)
			}
			toolsWrite(t, filepath.Join(dir, "greeting", "greeting.gt"), "package greeting\nfunc message() String { \"hi\" }\n")
			for _, tt := range []struct {
				args []string
				want string
			}{
				{[]string{"run", file}, "hi\n"},
				{[]string{"run", dir}, "hi\n"},
				{[]string{"test", file}, "PASS testGreeting\n1 passed, 0 failed\n"},
				{[]string{"eval", "answer()", file}, "42\n"},
				{[]string{"eval", "1 + 2"}, "3\n"},
			} {
				out, stderr, status := toolsCommand(t, dir, "", tt.args...)
				if out != tt.want || stderr != "" || status != 0 {
					t.Fatalf("%v: stdout=%q stderr=%q status=%d; want stdout=%q status=0", tt.args, out, stderr, status, tt.want)
				}
			}
			for _, generated := range []string{"main_gen", "_gen"} {
				if _, err := os.Stat(filepath.Join(dir, generated)); !os.IsNotExist(err) {
					t.Errorf("generated sources left beside input: %s: %v", generated, err)
				}
			}
			if withModule {
				got, err := os.ReadFile(filepath.Join(dir, "go.mod"))
				if err != nil || string(got) != mod {
					t.Errorf("input go.mod changed: %q, %v", got, err)
				}
			} else if _, err := os.Stat(filepath.Join(dir, "go.mod")); !os.IsNotExist(err) {
				t.Errorf("created a module beside input: %v", err)
			}
			entries, err := os.ReadDir(temporary)
			if err != nil || len(entries) != 0 {
				t.Errorf("temporary artifacts remain: %v, %v", entries, err)
			}
		})
	}
}

func TestInstalledCLIOutsideModule(t *testing.T) {
	dir := t.TempDir()
	bin := filepath.Join(dir, "go4t")
	build := exec.Command("go", "build", "-trimpath", "-o", bin, ".")
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build installed CLI: %v\n%s", err, out)
	}
	file := toolsWrite(t, filepath.Join(dir, "main.gt"), `package main
import "std/http"
func main() IO[Unit] { println("hi") }
func testInstalled() Bool { true }
`)
	for _, tt := range []struct {
		args []string
		want string
	}{
		{[]string{"run", file}, "hi\n"},
		{[]string{"test", file}, "PASS testInstalled\n1 passed, 0 failed\n"},
		{[]string{"eval", "1 + 2"}, "3\n"},
	} {
		command := exec.Command(bin, tt.args...)
		command.Dir = dir
		command.Env = append(os.Environ(), "GOPROXY=off", "GOWORK=off")
		out, err := command.CombinedOutput()
		if err != nil || string(out) != tt.want {
			t.Fatalf("installed %v: output=%q error=%v; want %q", tt.args, out, err, tt.want)
		}
	}
}

func TestOutsideModuleRequiresExternModule(t *testing.T) {
	for _, withModule := range []bool{false, true} {
		dir := t.TempDir()
		if withModule {
			toolsWrite(t, filepath.Join(dir, "go.mod"), "module unrelated\n\ngo 1.22\n")
		}
		file := toolsWrite(t, filepath.Join(dir, "main.gt"), `package main
extern func upper(s String) String = "strings".ToUpper
func main() IO[Unit] { println(upper("hi")) }
func testUpper() Bool { upper("hi") == "HI" }
`)
		const want = "extern \"strings\" requires a Go module: run inside a directory with go.mod that provides it\n"
		for _, args := range [][]string{
			{"run", file}, {"test", file}, {"eval", "upper(\"hi\")", file}, {"build", file},
		} {
			out, stderr, status := toolsCommand(t, dir, "", args...)
			if out != "" || stderr != want || status != 1 {
				t.Fatalf("%v: stdout=%q stderr=%q status=%d; want %q and status=1", args, out, stderr, status, want)
			}
		}
	}
}

func TestOutsideModuleCleansFailedExecution(t *testing.T) {
	dir, temporary := t.TempDir(), t.TempDir()
	t.Setenv("TMPDIR", temporary)
	file := toolsWrite(t, filepath.Join(dir, "failure.gt"), "package main\nfunc main() IO[Unit] { ioFail(\"broken\") }\n")
	output := filepath.Join(dir, "failure_gen")
	if err := os.Mkdir(output, 0o755); err != nil {
		t.Fatal(err)
	}
	existing := toolsWrite(t, filepath.Join(output, "main.go"), "existing build\n")
	out, stderr, status := toolsCommand(t, dir, "", "run", file)
	if out != "" || stderr != "error: broken\n" || status != 1 {
		t.Fatalf("stdout=%q stderr=%q status=%d", out, stderr, status)
	}
	contents, err := os.ReadFile(existing)
	if err != nil || string(contents) != "existing build\n" {
		t.Fatalf("run replaced existing build: %q, %v", contents, err)
	}
	entries, err := os.ReadDir(temporary)
	if err != nil || len(entries) != 0 {
		t.Fatalf("temporary artifacts remain after failure: %v, %v", entries, err)
	}
}

func TestBuildOutsideModule(t *testing.T) {
	for _, override := range []bool{false, true} {
		dir := t.TempDir()
		file := toolsWrite(t, filepath.Join(dir, "main.gt"), "package main\nfunc main() IO[Unit] { println(\"hi\") }\n")
		output := filepath.Join(dir, "main_gen")
		args := []string{"build", file}
		if override {
			// External output still uses the input module to inspect externs.
			inputDir := toolsModule(t)
			file = toolsWrite(t, filepath.Join(inputDir, "main.gt"), "package main\nextern func upper(s String) String = \"strings\".ToUpper\nfunc main() IO[Unit] { println(upper(\"hi\")) }\n")
			output = filepath.Join(dir, "nested", "generated")
			args = []string{"build", "-o", output, file}
		}
		out, stderr, status := toolsCommand(t, dir, "", args...)
		if out != "" || status != 0 || !strings.Contains(stderr, "go build") || !strings.Contains(stderr, "replace go4t =>") {
			t.Fatalf("%v: stdout=%q stderr=%q status=%d", args, out, stderr, status)
		}
		if _, err := os.Stat(filepath.Join(output, "main.go")); err != nil {
			t.Fatal(err)
		}
		if _, err := os.Stat(filepath.Join(output, "go.mod")); !os.IsNotExist(err) {
			t.Fatalf("build must emit sources only: %v", err)
		}
	}
}
