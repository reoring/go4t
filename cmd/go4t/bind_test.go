package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"go4t/ast"
	"go4t/modules"
	"go4t/parser"
	"go4t/types"
)

func TestBindSnapshot(t *testing.T) {
	want, err := os.ReadFile("../../extern/testdata/bindfixture/expected.gt")
	if err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(t.TempDir(), "bindings.gt")
	if err := run([]string{"bind", "go4t/extern/testdata/bindfixture", "-o", file}); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(want) {
		t.Fatalf("bind output:\n%s\nwant:\n%s", got, want)
	}
	stdout, err := os.CreateTemp(t.TempDir(), "stdout")
	if err != nil {
		t.Fatal(err)
	}
	previous := os.Stdout
	os.Stdout = stdout
	t.Cleanup(func() { os.Stdout = previous; stdout.Close() })
	err = run([]string{"bind", "go4t/extern/testdata/bindfixture"})
	os.Stdout = previous
	if err != nil {
		t.Fatal(err)
	}
	if err := stdout.Close(); err != nil {
		t.Fatal(err)
	}
	got, err = os.ReadFile(stdout.Name())
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(want) {
		t.Fatalf("stdout bind output:\n%s\nwant:\n%s", got, want)
	}
}

func TestBindPrefixProducesUsableDeclarations(t *testing.T) {
	file := filepath.Join(t.TempDir(), "bindings.gt")
	if err := run([]string{"bind", "go4t/extern/testdata/bindfixture", "--prefix", "fixture", "-o", file}); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(got), "collides with prelude") {
		t.Fatalf("prefixed bindings collide:\n%s", got)
	}
	for _, name := range []string{"Greeting", "Length", "LookupCode", "Read", "Repeat"} {
		if !strings.Contains(string(got), "extern func fixture"+name+"(") {
			t.Errorf("missing prefixed %s", name)
		}
	}
	var files []*ast.File
	for _, source := range []string{string(got), `package main
func main() IO[Unit] { println(fixtureGreeting("world")) }
`} {
		file, err := parser.Parse(source)
		if err != nil {
			t.Fatal(err)
		}
		files = append(files, file)
	}
	modules, err := modules.LoadFiles(files, ".")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := types.Check(modules); err != nil {
		t.Fatalf("prefixed bindings are unusable: %v", err)
	}
}

func TestBindReceivingModuleAndDirectoryOutput(t *testing.T) {
	for _, context := range []string{"directory", "file"} {
		t.Run(context, func(t *testing.T) {
			dir := toolsModule(t)
			source := toolsWrite(t, filepath.Join(dir, "main.gt"), "package utilities\nfunc main() IO[Unit] { println(greeting(\"world\")) }\n")
			helper := filepath.Join(dir, "helper")
			if err := os.Mkdir(helper, 0o755); err != nil {
				t.Fatal(err)
			}
			toolsWrite(t, filepath.Join(helper, "helper.go"), "package helper\nfunc Greeting(name string) string { return \"hello \" + name }\n")
			input := dir
			if context == "file" {
				input = source
			}
			if err := run([]string{"bind", "clitoolstest/helper", input, "-o", dir}); err != nil {
				t.Fatal(err)
			}
			bindings, err := os.ReadFile(filepath.Join(dir, "bindings.gt"))
			if err != nil {
				t.Fatal(err)
			}
			for _, want := range []string{"package utilities\n", `extern func greeting(p0 String) String = "clitoolstest/helper".Greeting`} {
				if !strings.Contains(string(bindings), want) {
					t.Errorf("bindings missing %q:\n%s", want, bindings)
				}
			}
			loaded, err := modules.Load([]string{dir})
			if err != nil {
				t.Fatal(err)
			}
			if _, err := types.Check(loaded); err != nil {
				t.Fatalf("receiving module cannot use its bindings: %v", err)
			}
		})
	}
}

func TestBindErrors(t *testing.T) {
	for _, args := range [][]string{
		{"bind"}, {"bind", "-o", "a.gt"}, {"bind", "strings", "-o"},
		{"bind", "strings", "--unknown"}, {"bind", "strings", "--prefix"},
		{"bind", "strings", "--prefix", "a", "--prefix", "b"},
		{"bind", "strings", "-o", ""},
	} {
		if err := run(args); err == nil || err.Error() != bindUsage {
			t.Errorf("run(%q) = %v", args, err)
		}
	}
	for _, prefix := range []string{"UPPER", "1bad", "not valid", "dict_bad", "ext_bad"} {
		if err := run([]string{"bind", "strings", "--prefix", prefix}); err == nil || !strings.HasPrefix(err.Error(), "go4t:1:1: invalid binding prefix ") {
			t.Errorf("prefix %q: %v", prefix, err)
		}
	}
	if err := run([]string{"bind", "go4t/no-such-extern-package"}); err == nil || !strings.HasPrefix(err.Error(), "go4t:1:1: load Go package ") {
		t.Errorf("missing package: %v", err)
	}
	output := filepath.Join(t.TempDir(), "missing", "bindings.gt")
	if err := run([]string{"bind", "strings", "-o", output}); err == nil || !strings.HasPrefix(err.Error(), output+":1:1:") {
		t.Errorf("write failure: %v", err)
	}
}
