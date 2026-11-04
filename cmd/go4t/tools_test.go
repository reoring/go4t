package main

import (
	"bytes"
	"errors"
	"fmt"
	"go/format"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/reoring/go4t/ast"
	"github.com/reoring/go4t/parser"
)

func TestCLIToolsProcess(t *testing.T) {
	if os.Getenv("GO4T_CLI_TOOLS_PROCESS") != "1" {
		return
	}
	for i, arg := range os.Args {
		if arg == "--" {
			os.Args = append([]string{"go4t"}, os.Args[i+1:]...)
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
	toolsWrite(t, filepath.Join(dir, "go.mod"), "module clitoolstest\n\ngo 1.22\n\nrequire github.com/reoring/go4t v0.0.0\nreplace github.com/reoring/go4t => "+strconv.Quote(root)+"\n")
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
	command.Env = append(os.Environ(), "GO4T_CLI_TOOLS_PROCESS=1")
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
func go4tTestRun() Bool { true }
`,
			second: "package main\nfunc testLast() Bool { go4tTestRun() }\n",
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
			first := toolsWrite(t, filepath.Join(dir, "first.gt"), tt.first)
			args := []string{"test", first}
			if tt.second != "" {
				args = append(args, toolsWrite(t, filepath.Join(dir, "second.gt"), tt.second))
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

func TestTestCommandExcludesExterns(t *testing.T) {
	dir := toolsModule(t)
	toolsWrite(t, filepath.Join(dir, "foreign.go"), "package clitoolstest\nfunc Excluded() bool { return false }\n")
	file := toolsWrite(t, filepath.Join(dir, "tests.gt"), `package main
extern func testExternal() Bool = "clitoolstest".Excluded
func testOrdinary() Bool { true }
`)
	out, stderr, status := toolsCommand(t, dir, "", "test", file)
	if out != "PASS testOrdinary\n1 passed, 0 failed\n" || stderr != "" || status != 0 {
		t.Fatalf("stdout=%q stderr=%q status=%d", out, stderr, status)
	}
}

func TestTestCommandLargeSuite(t *testing.T) {
	dir := toolsModule(t)
	toolsWrite(t, filepath.Join(dir, "foreign.go"), "package clitoolstest\nfunc PanicBool() bool { panic(\"boom\") }\n")
	var source, want strings.Builder
	source.WriteString(`package main
import "std/io"
extern func panicBool() Bool = "clitoolstest".PanicBool
func main() Int { missing }
func go4tTestRun() Bool { true }
func go4tTestBatch0() Bool { true }
func go4tTestGroup0_0() Bool { true }
func go4tTestGroup1_0() Bool { true }
func go4tTestResult0() Bool { true }
func go4tTestPassed() Bool { true }
`)
	// More than 64 leaves exercises aggregation beyond one level, not merely
	// the original 952-test scope-depth failure.
	const count = 4097
	passed := 0
	for i := range count {
		name := fmt.Sprintf("testCase%d", i)
		body, result := "", "PASS"
		switch i % 5 {
		case 0:
			body = "Bool { go4tTestRun() && go4tTestBatch0() && go4tTestGroup0_0() && go4tTestGroup1_0() && go4tTestResult0() && go4tTestPassed() }"
		case 1:
			body, result = "Bool { false }", "FAIL"
		case 2:
			body = fmt.Sprintf("IO[Bool] { do { println(\"STEP %d\"); ioPure(true) } }", i)
			fmt.Fprintf(&want, "STEP %d\n", i)
		case 3:
			body, result = "IO[Bool] { ioFail(\"broken\") }", "FAIL"
		case 4:
			body = "IO[Bool] { do { caught <- io.tryIO(do { println(\"PANIC\"); ioPure(panicBool()) }); ioPure(caught == Err(io.Panic(\"boom\"))) } }"
			want.WriteString("PANIC\n")
		}
		fmt.Fprintf(&source, "func %s() %s\n", name, body)
		fmt.Fprintf(&want, "%s %s", result, name)
		if i%5 == 3 {
			want.WriteString(": broken")
		}
		want.WriteByte('\n')
		if result == "PASS" {
			passed++
		}
	}
	fmt.Fprintf(&want, "%d passed, %d failed\n", passed, count-passed)
	file := toolsWrite(t, filepath.Join(dir, "tests.gt"), source.String())
	out, stderr, status := toolsCommand(t, dir, "", "test", file)
	if out != want.String() || stderr != "" || status != 1 {
		t.Fatalf("stdout=%q stderr=%q status=%d; want stdout=%q status=1", out, stderr, status, want.String())
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
				file := toolsWrite(t, filepath.Join(dir, "imports.gt"), "package main\nimport \"std/io\"\n")
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
	first := toolsWrite(t, filepath.Join(dir, "first.gt"), `package app
func main() IO[Unit] { ioFail("must not run") }
func answer() Int { twice(21) }
`)
	second := toolsWrite(t, filepath.Join(dir, "second.gt"), "package app\nfunc twice(n Int) Int { n * 2 }\n")
	out, stderr, status := toolsCommand(t, dir, "", "eval", "answer()", first, second)
	if out != "42\n" || stderr != "" || status != 0 {
		t.Fatalf("stdout=%q stderr=%q status=%d", out, stderr, status)
	}
}

func TestEvalIOFailure(t *testing.T) {
	dir := toolsModule(t)
	file := toolsWrite(t, filepath.Join(dir, "failure.gt"), "package main\nfunc failed() IO[Int] { ioFail(\"broken\") }\n")
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
	bad := toolsWrite(t, filepath.Join(dir, "bad.gt"), "package main\nfunc testBad() Bool { 1 }\n")
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
	other := toolsWrite(t, filepath.Join(dir, "other.gt"), "package other\n")
	if err := run([]string{"test", bad, other}); err == nil || !strings.Contains(err.Error(), "package other differs from main") {
		t.Errorf("package mismatch error=%v", err)
	}
}

func TestToolWarningsRenderSource(t *testing.T) {
	dir := toolsModule(t)
	file := toolsWrite(t, filepath.Join(dir, "warning.gt"), "package main\nfunc testUnused() Bool { let forgotten = 1; true }\n")
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
	file := toolsWrite(t, filepath.Join(dir, "source.gt"), "package main\nfunc main() IO[Unit] { println(message()) }\n")
	second := toolsWrite(t, filepath.Join(dir, "helper.gt"), "package main\nfunc message() String { \"override\" }\n")
	for _, after := range []bool{false, true} {
		output := filepath.Join(dir, "nested", strconv.FormatBool(after))
		args := []string{"build", "-o", output, file, second}
		if after {
			args = []string{"build", file, second, "--full-types", "-o", output}
		}
		if err := run(args); err != nil {
			t.Fatal(err)
		}
		command := exec.Command(output)
		if out, err := command.CombinedOutput(); err != nil || string(out) != "override\n" {
			t.Fatalf("generated executable output=%q error=%v", out, err)
		}
	}
	if _, err := os.Stat(filepath.Join(dir, "source_gen", "main.go")); err != nil {
		t.Fatalf("generated sources missing: %v", err)
	}
}

func TestBuildRejectsDirectoryOutput(t *testing.T) {
	dir := toolsModule(t)
	file := toolsWrite(t, filepath.Join(dir, "source.gt"), "package main\nfunc main() IO[Unit] { ioPure(unit()) }\n")
	out, stderr, status := toolsCommand(t, dir, "", "build", "-o", dir, file)
	want := file + ":1:1: build output " + strconv.Quote(dir) + " is a directory; use -o FILE or --emit-go DIR\npackage main\n^\n"
	if out != "" || stderr != want || status != 1 {
		t.Fatalf("directory output: stdout=%q stderr=%q status=%d; want %q", out, stderr, status, want)
	}
	if _, err := os.Stat(filepath.Join(dir, "source_gen")); !os.IsNotExist(err) {
		t.Fatalf("rejected output generated sources: %v", err)
	}
}

func TestBuildPropagatesGoFailure(t *testing.T) {
	dir := toolsModule(t)
	toolsWrite(t, filepath.Join(dir, "source.gt"), "package main\nfunc main() IO[Unit] { ioPure(unit()) }\n")
	generated := filepath.Join(dir, "source_gen")
	if err := os.Mkdir(generated, 0o755); err != nil {
		t.Fatal(err)
	}
	toolsWrite(t, filepath.Join(generated, "user.go"), "package main\nvar _ = missingGoSymbol\n")
	out, stderr, status := toolsCommand(t, dir, "", "build", "-o", "prog", "source.gt")
	if out != "" || status != 1 || !strings.Contains(stderr, "undefined: missingGoSymbol") ||
		!strings.Contains(stderr, "source.gt:1:1: go build: exit status 1") {
		t.Fatalf("Go build failure: stdout=%q stderr=%q status=%d", out, stderr, status)
	}
}

func TestEmitGoDefaultsToMain(t *testing.T) {
	dir := toolsModule(t)
	file := toolsWrite(t, filepath.Join(dir, "source.gt"), "package main\nfunc main() IO[Unit] { println(\"emitted\") }\n")
	for _, explicit := range []bool{false, true} {
		output := filepath.Join(dir, "emitted")
		args := []string{"build", "--emit-go", output, file}
		if explicit {
			args = append(args, "--package", "main")
		}
		out, stderr, status := toolsCommand(t, dir, "", args...)
		if out != "" || stderr != "" || status != 0 {
			t.Fatalf("emit Go: stdout=%q stderr=%q status=%d", out, stderr, status)
		}
		command := exec.Command("go", "run", "./emitted")
		command.Dir = dir
		if output, err := command.CombinedOutput(); err != nil || string(output) != "emitted\n" {
			t.Fatalf("emitted executable: output=%q error=%v", output, err)
		}
	}
	for _, name := range []string{"source", "source_gen"} {
		if _, err := os.Stat(filepath.Join(dir, name)); !os.IsNotExist(err) {
			t.Fatalf("emit Go created %s: %v", name, err)
		}
	}
}

func TestEmitGoPackageCanBeImported(t *testing.T) {
	dir := toolsModule(t)
	file := toolsWrite(t, filepath.Join(dir, "source.gt"), `package main
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
    "context"
    "clitoolstest/embedded"
    "github.com/reoring/go4t/rt"
)
func main() {
    var action rt.IO = embedded.Main()
    value, err := action(context.Background())
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

func TestGoShellExample(t *testing.T) {
	dir := toolsModule(t)
	example := filepath.Join("..", "..", "examples", "go-shell")
	source, err := os.ReadFile(filepath.Join(example, "core.gt"))
	if err != nil {
		t.Fatal(err)
	}
	input := toolsWrite(t, filepath.Join(dir, "core.gt"), string(source))
	output := filepath.Join(dir, "core")
	// Match the directive's relative paths, including the generated //line names.
	if out, stderr, status := toolsCommand(t, dir, "", "build", "--emit-go", "core", "--package", "core", "core.gt"); status != 0 {
		t.Fatalf("build Go shell: status=%d\n%s%s", status, out, stderr)
	}
	exports, err := os.ReadFile(filepath.Join(output, "go4t_export.go"))
	if err != nil {
		t.Fatal(err)
	}
	for _, fragment := range []string{
		"func Decide(p0 Observed) Step",
		"// not exported: identity (type parameters)",
		"type StepDependenciesPending struct",
		"func (v Observed) String() string",
		"func (v StepCleanup) String() string",
	} {
		if !strings.Contains(string(exports), fragment) {
			t.Errorf("exports missing %q:\n%s", fragment, exports)
		}
	}
	if strings.Contains(string(exports), "_helper") || strings.Contains(string(exports), "not exported: main") {
		t.Errorf("private/main declaration leaked:\n%s", exports)
	}
	formatted, err := format.Source(exports)
	if err != nil || !bytes.Equal(exports, formatted) {
		t.Fatalf("exports not go/format-clean: %v", err)
	}
	consumer, err := os.ReadFile(filepath.Join(example, "main.go"))
	if err != nil {
		t.Fatal(err)
	}
	code := strings.TrimPrefix(string(consumer), "//go:build ignore\n\n")
	code = strings.ReplaceAll(code, `"goshell/core"`, `"clitoolstest/core"`)
	toolsWrite(t, filepath.Join(dir, "main.go"), code)
	command := exec.Command("go", "run", ".")
	command.Dir = dir
	want := "pending: [cache secrets]\ncleanup: Cleanup(7)\nround trips: Ready record list option\n"
	if out, err := command.CombinedOutput(); err != nil || string(out) != want {
		t.Fatalf("Go shell output=%q error=%v; want %q", out, err, want)
	}
	// Exercise the checked-in directive through a real CLI on PATH, then compare
	// every emitted file so go generate cannot silently become a no-op or drift.
	generated := make(map[string][]byte)
	entries, err := os.ReadDir(output)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		content, err := os.ReadFile(filepath.Join(output, entry.Name()))
		if err != nil {
			t.Fatal(err)
		}
		generated[entry.Name()] = content
	}
	bin := filepath.Join(t.TempDir(), "go4t")
	build := exec.Command("go", "build", "-o", bin, ".")
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build generation CLI: %v\n%s", err, out)
	}
	generate := exec.Command("go", "generate", "-x", "./...")
	generate.Dir = dir
	generate.Env = append(os.Environ(), "PATH="+filepath.Dir(bin)+string(os.PathListSeparator)+os.Getenv("PATH"))
	if out, err := generate.CombinedOutput(); err != nil || !strings.Contains(string(out), "go4t build --emit-go core --package core core.gt") {
		t.Fatalf("Go shell generate: %v\n%s", err, out)
	}
	entries, err = os.ReadDir(output)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != len(generated) {
		t.Fatalf("go generate changed file count: got %d, want %d", len(entries), len(generated))
	}
	for _, entry := range entries {
		content, err := os.ReadFile(filepath.Join(output, entry.Name()))
		before, exists := generated[entry.Name()]
		if err != nil || !exists || !bytes.Equal(content, before) {
			t.Errorf("go generate changed %s: %v", entry.Name(), err)
		}
	}
	vet := exec.Command("go", "vet", "./core")
	vet.Dir = dir
	if out, err := vet.CombinedOutput(); err != nil {
		t.Fatalf("Go shell vet: %v\n%s", err, out)
	}
	// The same marker-based sweep as ordinary modules removes obsolete exports
	// when the output directory is subsequently used for package main sources.
	owned := toolsWrite(t, filepath.Join(output, "user.go"), "package main\n")
	if err := run([]string{"build", "--emit-go", output, input}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(output, "go4t_export.go")); !os.IsNotExist(err) {
		t.Fatalf("obsolete Go exports remain: %v", err)
	}
	if _, err := os.Stat(owned); err != nil {
		t.Fatalf("user file removed: %v", err)
	}
}

func TestToolsUsageErrors(t *testing.T) {
	for _, args := range [][]string{
		{"test"}, {"eval"}, {"test", "--"}, {"eval", "1", "--"},
		{"build", "-o"}, {"build", "-o", "out"},
		{"build", "--emit-go", "out"},
		{"build", "--package", "app", "main.gt"},
		{"build", "-o", "out", "--emit-go", "other", "--package", "app", "main.gt"},
		{"build", "-o", "out", "-o", "other", "main.gt"},
		{"build", "--emit-go", "out", "--emit-go", "other", "main.gt"},
		{"build", "--emit-go", "out", "--package", "app", "--package", "other", "main.gt"},
		{"build", "--unknown", "main.gt"},
	} {
		if err := run(args); err == nil || !strings.Contains(err.Error(), "usage") {
			t.Errorf("run(%q)=%v, want usage error", args, err)
		}
	}
	dir := toolsModule(t)
	file := toolsWrite(t, filepath.Join(dir, "source.gt"), "package main\nfunc main() IO[Unit] { ioPure(unit()) }\n")
	for _, pkg := range []string{"func", "_", "bad-name"} {
		err := run([]string{"build", "--emit-go", filepath.Join(dir, "out"), "--package", pkg, file})
		if err == nil || !strings.Contains(err.Error(), "invalid Go package name") {
			t.Errorf("package %q error=%v", pkg, err)
		}
	}
}
