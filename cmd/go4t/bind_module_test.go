package main

import (
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"testing"
)

func bindingModule(t *testing.T) (string, string) {
	t.Helper()
	offlineDependencies(t)
	root := toolsModule(t)
	library := t.TempDir()
	toolsWrite(t, filepath.Join(library, "go.mod"), "module example.com/native\n\ngo 1.22\n")
	for name, source := range map[string]string{
		"native.go": "package native\nfunc Answer() int { return 42 }\n",
		"hello-world/native.go": "package differentlynamed\nfunc Greeting() string { return \"hello\" }\n",
		"match/native.go": "package match\nfunc Word() string { return \"!\" }\n",
		"internal/private/native.go": "package private\nfunc Secret() int { return 0 }\n",
		"cmd/tool/main.go": "package main\nfunc main() {}\n",
		"external_test/native.go": "package external_test\nfunc Ignore() int { return 0 }\n",
		"broken/native.go": "package broken\nfunc Broken() int { return missing }\n",
	} {
		file := filepath.Join(library, name)
		if err := os.MkdirAll(filepath.Dir(file), 0o755); err != nil {
			t.Fatal(err)
		}
		toolsWrite(t, file, source)
	}
	mod, err := os.ReadFile(filepath.Join(root, "go.mod"))
	if err != nil {
		t.Fatal(err)
	}
	toolsWrite(t, filepath.Join(root, "go.mod"), string(mod)+"replace example.com/native => "+strconv.Quote(library)+"\n")
	toolsWrite(t, filepath.Join(root, "go4t.mod"), "module clitoolstest\n")
	return root, library
}

func bindingSnapshot(t *testing.T, root string) map[string]string {
	t.Helper()
	files := make(map[string]string)
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		relative, err := filepath.Rel(root, path)
		if err == nil {
			files[relative] = string(data)
		}
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	return files
}

func TestGetGeneratesBindingsIdempotently(t *testing.T) {
	root, library := bindingModule(t)
	beforeLibrary := bindingSnapshot(t, library)
	nested := filepath.Join(root, "app")
	if err := os.Mkdir(nested, 0o755); err != nil {
		t.Fatal(err)
	}
	for attempt := range 2 {
		before := bindingSnapshot(t, root)
		out, stderr, status := toolsCommand(t, nested, "", "get", "example.com/native@v1.0.0")
		if out != "" || status != 0 || !strings.Contains(stderr, "skipped package example.com/native/broken") {
			t.Fatalf("get: stdout=%q stderr=%q status=%d", out, stderr, status)
		}
		if attempt == 1 && !reflect.DeepEqual(before, bindingSnapshot(t, root)) {
			t.Fatal("second get changed project files")
		}
	}
	assertFile(t, filepath.Join(root, "go4t.mod"), "module clitoolstest\nrequire example.com/native v1.0.0\n")
	if files := bindingSnapshot(t, filepath.Join(root, "go4tbind")); len(files) != 6 {
		t.Fatalf("expected two generated files per package, got %v", files)
	}
	for path, packageName := range map[string]string{"example.com/native": "native", "example.com/native/hello-world": "hello_world", "example.com/native/match": "match_"} {
		dir := filepath.Join(root, "go4tbind", filepath.FromSlash(path))
		goName := goIdentifier(filepath.Base(path))
		files := bindingSnapshot(t, dir)
		if !strings.Contains(files["bindings.gt"], "package "+packageName+"\n") || !strings.Contains(files["go4t_shim_"+goName+".go"], "package "+goName+"\n") {
			t.Fatalf("unexpected binding files for %s: %v", path, files)
		}
	}
	for _, path := range []string{"internal", "cmd", "external_test", "broken"} {
		if _, err := os.Stat(filepath.Join(root, "go4tbind/example.com/native", path)); !os.IsNotExist(err) {
			t.Fatalf("excluded package %s was generated: %v", path, err)
		}
	}
	if !reflect.DeepEqual(beforeLibrary, bindingSnapshot(t, library)) {
		t.Fatal("binding changed dependency source files")
	}
	toolsWrite(t, filepath.Join(nested, "main.gt"), `package main
import "go4tbind/example.com/native"
import greeting "go4tbind/example.com/native/hello-world"
import words "go4tbind/example.com/native/match"
func main() IO[Unit] { println(greeting.greeting() + " " + show(native.answer()) + words.word()) }
`)
	out, stderr, status := toolsCommand(t, root, "", "run", nested)
	if out != "hello 42!\n" || stderr != "" || status != 0 {
		t.Fatalf("generated imports: stdout=%q stderr=%q status=%d", out, stderr, status)
	}
}

func TestGetNoBindAndBindModule(t *testing.T) {
	root, _ := bindingModule(t)
	for _, args := range [][]string{
		{"get", "--no-bind", "example.com/native@v1.0.0"},
		{"get", "example.com/native@v1.0.0", "--no-bind"},
	} {
		out, stderr, status := toolsCommand(t, root, "", args...)
		if out != "" || status != 0 {
			t.Fatalf("get --no-bind: stdout=%q stderr=%q status=%d", out, stderr, status)
		}
		if _, err := os.Stat(filepath.Join(root, "go4tbind")); !os.IsNotExist(err) {
			t.Fatalf("--no-bind generated bindings: %v", err)
		}
	}
	before := bindingSnapshot(t, root)
	out, stderr, status := toolsCommand(t, root, "", "bind", "--module", "example.com/native")
	if out != "" || status != 0 {
		t.Fatalf("bind --module: stdout=%q stderr=%q status=%d", out, stderr, status)
	}
	for _, name := range []string{"go.mod", "go4t.mod"} {
		assertFile(t, filepath.Join(root, name), before[name])
	}
	if _, err := os.Stat(filepath.Join(root, "go4tbind/example.com/native/bindings.gt")); err != nil {
		t.Fatal(err)
	}
}

func TestBindModuleStandardPackage(t *testing.T) {
	offlineDependencies(t)
	root := toolsModule(t)
	toolsWrite(t, filepath.Join(root, "go4t.mod"), "module clitoolstest\n")
	out, stderr, status := toolsCommand(t, root, "", "bind", "--module", "database/sql")
	if out != "" || stderr != "" || status != 0 {
		t.Fatalf("bind std: stdout=%q stderr=%q status=%d", out, stderr, status)
	}
	files := bindingSnapshot(t, filepath.Join(root, "go4tbind"))
	if len(files) != 2 || !strings.Contains(files["database/sql/bindings.gt"], "package sql\n") {
		t.Fatalf("expected only database/sql, got %v", files)
	}
}

func TestBindModuleErrors(t *testing.T) {
	for _, args := range [][]string{{"--module"}, {"--module", ""}, {"--module", "../escape"}, {"--module", "database/sql", "-o", "other"}} {
		if err := runBind(args); err == nil || err.Error() != bindUsage {
			t.Errorf("bind %q: %v", args, err)
		}
	}
	offlineDependencies(t)
	root := toolsModule(t)
	_, stderr, status := toolsCommand(t, root, "", "bind", "--module", "database/sql")
	if status != 1 || !strings.Contains(stderr, "no go4t.mod found") {
		t.Fatalf("bind outside project: stderr=%q status=%d", stderr, status)
	}
	toolsWrite(t, filepath.Join(root, "go4t.mod"), "module clitoolstest\n")
	_, stderr, status = toolsCommand(t, root, "", "bind", "--module", "example.invalid/missing")
	if status != 1 || !strings.Contains(stderr, "not a required Go module or standard package") {
		t.Fatalf("bind missing module: stderr=%q status=%d", stderr, status)
	}
}
