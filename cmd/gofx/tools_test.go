package main

import (
	"bytes"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"gofx/ast"
	"gofx/parser"
)

func TestCLIToolsProcess(t *testing.T) {
	if os.Getenv("GOFX_CLI_TOOLS_PROCESS") != "1" {
		return
	}
	for i, arg := range os.Args {
		if arg == "--" {
			os.Args = append([]string{"gofx"}, os.Args[i+1:]...)
			main()
			os.Exit(0)
		}
	}
	t.Fatal("missing command separator")
}

func toolsModule(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	toolsWrite(t, filepath.Join(dir, "go.mod"), "module clitoolstest\n\ngo 1.22\n\nrequire gofx v0.0.0\nreplace gofx => "+strconv.Quote(root)+"\n")
	return dir
}

func toolsWrite(t *testing.T, path, source string) string {
	t.Helper()
	if err := os.WriteFile(path, []byte(source), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func toolsCommand(t *testing.T, dir, input string, args ...string) (stdout, stderr string, status int) {
	t.Helper()
	command := exec.Command(os.Args[0], append([]string{"-test.run=^TestCLIToolsProcess$", "--"}, args...)...)
	command.Dir = dir
	command.Env = append(os.Environ(), "GOFX_CLI_TOOLS_PROCESS=1")
	command.Stdin = strings.NewReader(input)
	var out, diagnostics bytes.Buffer
	command.Stdout, command.Stderr = &out, &diagnostics
	err := command.Run()
	if err != nil {
		var exit *exec.ExitError
		if !errors.As(err, &exit) {
			t.Fatal(err)
		}
		status = exit.ExitCode()
	}
	return out.String(), diagnostics.String(), status
}

func TestTestCommand(t *testing.T) {
	for _, tt := range []struct {
		name, first, second, want string
		status                    int
	}{
		{
			name: "pure IO source order and ignored main",
			first: `package main
func main() Int { missing }
func testZulu() Bool { 1 + 2 == 3 }
func testAlpha() IO[Bool] { ioPure(true) }
func test() Bool { false }
func testArgument(x Bool) Bool { x }
func testWrongResult() Int { 0 }
func testGeneric[A]() Bool { false }
func gofxTestRun() Bool { true }
`,
			second: "package main\nfunc testLast() Bool { gofxTestRun() }\n",
			want:   "PASS testZulu\nPASS testAlpha\nPASS testLast\n3 passed, 0 failed\n",
		},
		{
			name: "failures do not short circuit",
			first: `package main
func testFalse() Bool { false }
func testIOFalse() IO[Bool] { ioPure(false) }
func testError() IO[Bool] { ioFail("broken") }
func testAfterError() IO[Bool] { ioPure(true) }
`,
			want:   "FAIL testFalse\nFAIL testIOFalse\nFAIL testError: broken\nPASS testAfterError\n1 passed, 3 failed\n",
			status: 1,
		},
		{
			name:  "no tests or main",
			first: "package main\nfunc helper() Int { 42 }\n",
			want:  "0 passed, 0 failed\n",
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			dir := toolsModule(t)
			first := toolsWrite(t, filepath.Join(dir, "first.gfx"), tt.first)
			args := []string{"test", first}
			if tt.second != "" {
				args = append(args, toolsWrite(t, filepath.Join(dir, "second.gfx"), tt.second))
			}
			// Test must not overwrite an existing ordinary build.
			outputDir := filepath.Join(dir, "first_gen")
			if err := os.Mkdir(outputDir, 0o755); err != nil {
				t.Fatal(err)
			}
			existing := toolsWrite(t, filepath.Join(outputDir, "main.go"), "existing build\n")
			out, stderr, status := toolsCommand(t, dir, "", args...)
			if out != tt.want || stderr != "" || status != tt.status {
				t.Fatalf("stdout=%q stderr=%q status=%d; want stdout=%q status=%d", out, stderr, status, tt.want, tt.status)
			}
			contents, err := os.ReadFile(existing)
			if err != nil || string(contents) != "existing build\n" {
				t.Fatalf("test replaced existing build: %q, %v", contents, err)
			}
		})
	}
}

func TestTestHarnessExcludesExterns(t *testing.T) {
	file, err := parser.Parse(`package main
extern func testExternal() Bool = "example".Test
func testOrdinary() Bool { true }
`)
	if err != nil {
		t.Fatal(err)
	}
	harness, err := testHarness([]*ast.File{{}, file})
	if err != nil {
		t.Fatal(err)
	}
	body := harness.Decls[1].(*ast.FuncDecl).Body.Result.(*ast.Do)
	var names []string
	for _, stmt := range body.Stmts {
		if bind, ok := stmt.(*ast.DoBind); ok {
			names = append(names, bind.X.(*ast.Call).Args[0].(*ast.StringLit).Value)
		}
	}
	if len(names) != 1 || names[0] != "testOrdinary" {
		t.Fatalf("collected tests = %v", names)
	}
}

func TestEvalCommand(t *testing.T) {
	for _, tt := range []struct {
		name, expression, input, want string
	}{
		{"pure", "1 + 2", "", "3\n"},
		{"negative expression", "-2", "", "-2\n"},
		{"string uses Show", `"hello"`, "", "hello\n"},
		{"pure Option", "Some(1)", "", "Some(1)\n"},
		{"IO", "ioPure(7)", "", "7\n"},
		{"stdin", "io.readLine()", "hello\n", "Some(hello)\n"},
		{"EOF", "io.readLine()", "", "None\n"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			dir := toolsModule(t)
			args := []string{"eval", tt.expression}
			if strings.HasPrefix(tt.expression, "io.") {
				file := toolsWrite(t, filepath.Join(dir, "imports.gfx"), "package main\nimport \"std/io\"\n")
				args = append(args, file)
			}
			out, stderr, status := toolsCommand(t, dir, tt.input, args...)
			if out != tt.want || stderr != "" || status != 0 {
				t.Fatalf("stdout=%q stderr=%q status=%d; want stdout=%q status=0", out, stderr, status, tt.want)
			}
		})
	}
}

func TestEvalUsesFilesAndIgnoresMain(t *testing.T) {
	dir := toolsModule(t)
	first := toolsWrite(t, filepath.Join(dir, "first.gfx"), `package app
func main() IO[Unit] { ioFail("must not run") }
func answer() Int { twice(21) }
`)
	second := toolsWrite(t, filepath.Join(dir, "second.gfx"), "package app\nfunc twice(n Int) Int { n * 2 }\n")
	out, stderr, status := toolsCommand(t, dir, "", "eval", "answer()", first, second)
	if out != "42\n" || stderr != "" || status != 0 {
		t.Fatalf("stdout=%q stderr=%q status=%d", out, stderr, status)
	}
}

func TestEvalIOFailure(t *testing.T) {
	dir := toolsModule(t)
	file := toolsWrite(t, filepath.Join(dir, "failure.gfx"), "package main\nfunc failed() IO[Int] { ioFail(\"broken\") }\n")
	out, stderr, status := toolsCommand(t, dir, "", "eval", "failed()", file)
	if out != "" || stderr != "error: broken\n" || status != 1 {
		t.Fatalf("stdout=%q stderr=%q status=%d", out, stderr, status)
	}
}

func TestEvalPreservesOriginalError(t *testing.T) {
	for _, expression := range []string{"1 + true", "missing", "func(x Int) => x"} {
		expr, err := parser.ParseExpr(expression, ast.Pos{Line: 1, Col: 1})
		if err != nil {
			t.Fatal(err)
		}
		files, err := parseFiles(nil)
		if err != nil {
			t.Fatal(err)
		}
		wrapper, err := evalWrapper(expr, false)
		if err != nil {
			t.Fatal(err)
		}
		entry := files[len(files)-1]
		entry.Files = append(entry.Files, wrapper)
		_, original := checkFiles(files, false)
		got := run([]string{"eval", expression})
		if original == nil || got == nil || original.Error() != got.Error() {
			t.Errorf("eval %q error=%v, original=%v", expression, got, original)
		}
		if got != nil && !strings.HasPrefix(got.Error(), "<eval>:") {
			t.Errorf("missing eval source position: %v", got)
		}
	}
}

func TestToolDiagnostics(t *testing.T) {
	dir := toolsModule(t)
	bad := toolsWrite(t, filepath.Join(dir, "bad.gfx"), "package main\nfunc testBad() Bool { 1 }\n")
	for _, args := range [][]string{{"test", bad}, {"eval", "1", bad}} {
		err := run(args)
		if err == nil || !strings.Contains(err.Error(), bad+":2:") || !strings.Contains(err.Error(), "type mismatch") ||
			!strings.Contains(err.Error(), "\nfunc testBad() Bool { 1 }\n                      ^") {
			t.Errorf("%q error=%v, want original file, type mismatch and caret", args, err)
		}
	}
	if err := run([]string{"eval", "1; func injected() Int { 2 }"}); err == nil || !strings.Contains(err.Error(), "expected end of expression") {
		t.Errorf("trailing declarations error=%v", err)
	}
	if err := run([]string{"eval", ""}); err == nil || !strings.HasPrefix(err.Error(), "<eval>:1:1:") {
		t.Errorf("empty expression error=%v", err)
	}
	other := toolsWrite(t, filepath.Join(dir, "other.gfx"), "package other\n")
	if err := run([]string{"test", bad, other}); err == nil || !strings.Contains(err.Error(), "package other differs from main") {
		t.Errorf("package mismatch error=%v", err)
	}
}

func TestToolWarningsRenderSource(t *testing.T) {
	dir := toolsModule(t)
	file := toolsWrite(t, filepath.Join(dir, "warning.gfx"), "package main\nfunc testUnused() Bool { let forgotten = 1; true }\n")
	for _, args := range [][]string{{"test", file}, {"eval", "testUnused()", file}} {
		_, stderr, status := toolsCommand(t, dir, "", args...)
		want := file + ":2:30: warning: forgotten is unused; prefix with _ to silence\n" +
			"func testUnused() Bool { let forgotten = 1; true }\n                             ^~~~~~~~~\n"
		if status != 0 || stderr != want {
			t.Errorf("%q stderr=%q status=%d, want %q and success", args, stderr, status, want)
		}
	}
}

func TestBuildOutputOverride(t *testing.T) {
	dir := toolsModule(t)
	file := toolsWrite(t, filepath.Join(dir, "source.gfx"), "package main\nfunc main() IO[Unit] { println(\"override\") }\n")
	for _, after := range []bool{false, true} {
		output := filepath.Join(dir, "nested", strconv.FormatBool(after))
		args := []string{"build", "-o", output, file}
		if after {
			args = []string{"build", file, "-o", output}
		}
		if err := run(args); err != nil {
			t.Fatal(err)
		}
		command := exec.Command("go", "run", "./nested/"+strconv.FormatBool(after))
		command.Dir = dir
		if out, err := command.CombinedOutput(); err != nil || string(out) != "override\n" {
			t.Fatalf("generated executable output=%q error=%v", out, err)
		}
	}
	if _, err := os.Stat(filepath.Join(dir, "source_gen")); !os.IsNotExist(err) {
		t.Fatalf("default directory was created: %v", err)
	}
}

func TestEmitGoPackageCanBeImported(t *testing.T) {
	dir := toolsModule(t)
	file := toolsWrite(t, filepath.Join(dir, "source.gfx"), `package main
import "std/string"
func main() IO[Unit] {
    do {
        println(show(string.split(",", "a,b")))
        ioPure(unit())
    }
}
`)
	if err := run([]string{"build", "--emit-go", filepath.Join(dir, "embedded"), "--package", "embedded", file}); err != nil {
		t.Fatal(err)
	}
	toolsWrite(t, filepath.Join(dir, "main.go"), `package main
import (
    "clitoolstest/embedded"
    "gofx/rt"
)
func main() {
    var action rt.IO = embedded.Main()
    value, err := action()
    if err != nil { panic(err) }
    if _, ok := value.(rt.Unit); !ok { panic("not Unit") }
}
`)
	command := exec.Command("go", "run", ".")
	command.Dir = dir
	if out, err := command.CombinedOutput(); err != nil || string(out) != "[a, b]\n" {
		t.Fatalf("imported package output=%q error=%v", out, err)
	}
}

func TestToolUsageErrors(t *testing.T) {
	for _, args := range [][]string{
		{"test"}, {"eval"}, {"test", "--"}, {"eval", "1", "--"},
		{"build", "-o"}, {"build", "-o", "out"},
		{"build", "--emit-go", "out", "main.gfx"},
		{"build", "--package", "app", "main.gfx"},
		{"build", "--emit-go", "out", "--package", "main", "main.gfx"},
		{"build", "-o", "out", "--emit-go", "other", "--package", "app", "main.gfx"},
		{"build", "-o", "out", "-o", "other", "main.gfx"},
		{"build", "--unknown", "main.gfx"},
	} {
		if err := run(args); err == nil || !strings.Contains(err.Error(), "usage") {
			t.Errorf("run(%q)=%v, want usage error", args, err)
		}
	}
	dir := toolsModule(t)
	file := toolsWrite(t, filepath.Join(dir, "source.gfx"), "package main\nfunc main() IO[Unit] { ioPure(unit()) }\n")
	for _, pkg := range []string{"func", "_", "bad-name"} {
		err := run([]string{"build", "--emit-go", filepath.Join(dir, "out"), "--package", pkg, file})
		if err == nil || !strings.Contains(err.Error(), "invalid Go package name") {
			t.Errorf("package %q error=%v", pkg, err)
		}
	}
}
