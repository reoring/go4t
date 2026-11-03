package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestBindShimRun(t *testing.T) {
	dir := toolsModule(t)
	shim := filepath.Join(dir, "go-shim")
	if err := os.Mkdir(shim, 0o755); err != nil {
		t.Fatal(err)
	}
	toolsWrite(t, filepath.Join(shim, "package.go"), "package bridge\n")
	toolsWrite(t, filepath.Join(dir, "main.gt"), `package main
func main() IO[Unit] {
  do {
    println(widgetEcho(Widget{name: "hello"}, "!"))
    println(show(widgetCount(Widget{name: "hello"})))
    println(show(widgetLookup(Widget{name: "hello"}, true)))
    text <- widgetRead(Widget{name: "read"})
    println(text)
  }
}
`)
	args := []string{"bind", "go4t/extern/testdata/methodfixture", ".", "--shim", shim, "-o", dir}
	out, stderr, status := toolsCommand(t, dir, "", args...)
	if out != "" || stderr != "" || status != 0 {
		t.Fatalf("bind: stdout=%q stderr=%q status=%d", out, stderr, status)
	}
	declarations, err := os.ReadFile(filepath.Join(dir, "bindings.gt"))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"type Widget = Widget{name: String} deriving Eq, Debug",
		`extern func widgetEcho(recv Widget, p0 String) String = "clitoolstest/go-shim".Widget_Echo`,
		`extern func widgetCount(recv Widget) Int = "clitoolstest/go-shim".Widget_Count`,
		"extern func widgetPointer(recv Widget, p0 Option[String]) String",
	} {
		if !strings.Contains(string(declarations), want) {
			t.Errorf("missing %q in declarations:\n%s", want, declarations)
		}
	}
	generated, err := os.ReadFile(filepath.Join(shim, "go4t_shim_methodfixture.go"))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"package bridge", "func Widget_Echo(recv any, p0 any) any", "func Widget_Count(recv any) any"} {
		if !strings.Contains(string(generated), want) {
			t.Errorf("missing %q in Go shim:\n%s", want, generated)
		}
	}
	// Legacy output is inspected above; runtime execution uses owner metadata
	// carried by go: imports rather than the generated inspection file.
	file := toolsWrite(t, filepath.Join(dir, "run.gt"), `package main
import f "go:go4t/extern/testdata/methodfixture"
func main() IO[Unit] {
  do {
    println(f.widgetEcho(f.Widget{name: "hello"}, "!"))
    println(show(f.widgetCount(f.Widget{name: "hello"})))
    println(show(f.widgetLookup(f.Widget{name: "hello"}, true)))
    text <- f.widgetRead(f.Widget{name: "read"})
    println(text)
  }
}
`)
	out, stderr, status = toolsCommand(t, dir, "", "run", file)
	if out != "hello!\n5\nSome(hello)\nread\n" || stderr != "" || status != 0 {
		t.Fatalf("run: stdout=%q stderr=%q status=%d", out, stderr, status)
	}
	// Regeneration sees the existing shim but keeps the same package and bytes.
	out, stderr, status = toolsCommand(t, dir, "", args...)
	if out != "" || stderr != "" || status != 0 {
		t.Fatalf("rebind: stdout=%q stderr=%q status=%d", out, stderr, status)
	}
	again, err := os.ReadFile(filepath.Join(shim, "go4t_shim_methodfixture.go"))
	if err != nil || string(again) != string(generated) {
		t.Fatalf("shim changed on regeneration: %v\n%s", err, again)
	}
}

func TestBindShimDirectoryAndErrors(t *testing.T) {
	for _, name := range []string{"new-shim", "123", "type"} {
		t.Run(name, func(t *testing.T) {
			dir := toolsModule(t)
			out, stderr, status := toolsCommand(t, dir, "", "bind", "net/url", "--prefix", "native", "--shim", name)
			if stderr != "" || status != 0 || !strings.Contains(out, "extern func nativeURLHostname(recv URL) IO[String]") {
				t.Fatalf("bind: stdout=%q stderr=%q status=%d", out, stderr, status)
			}
			generated, err := os.ReadFile(filepath.Join(dir, name, "go4t_shim_url.go"))
			if err != nil {
				t.Fatal(err)
			}
			wantPackage := map[string]string{"new-shim": "new_shim", "123": "_123", "type": "type_"}[name]
			if !strings.Contains(string(generated), "package "+wantPackage+"\n") {
				t.Fatalf("unexpected package:\n%s", generated)
			}
		})
	}
	for _, args := range [][]string{
		{"bind", "net/url", "--shim"},
		{"bind", "net/url", "--shim", ""},
		{"bind", "net/url", "--shim", "one", "--shim", "two"},
	} {
		if err := run(args); err == nil || err.Error() != bindUsage {
			t.Errorf("run(%q) = %v", args, err)
		}
	}
	outside := t.TempDir()
	out, stderr, status := toolsCommand(t, outside, "", "bind", "net/url", "--shim", "shim")
	if out != "" || status != 1 || !strings.Contains(stderr, "--shim requires DIR inside a Go module") {
		t.Fatalf("outside module: stdout=%q stderr=%q status=%d", out, stderr, status)
	}
}

func TestBindShimTypedExternExample(t *testing.T) {
	dir := toolsModule(t)
	shim := filepath.Join(dir, "shim")
	if err := os.Mkdir(shim, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"main.gt", "gostd.gt", "shim/url.go"} {
		source, err := os.ReadFile(filepath.Join("../../examples/typed-extern", name))
		if err != nil {
			t.Fatal(err)
		}
		text := strings.ReplaceAll(string(source), "go4t/examples/typed-extern/shim", "clitoolstest/shim")
		toolsWrite(t, filepath.Join(dir, name), text)
	}
	out, stderr, status := toolsCommand(t, dir, "", "bind", "net/url", "--shim", "shim", "-o", "methods.gt")
	if out != "" || stderr != "" || status != 0 {
		t.Fatalf("bind example: stdout=%q stderr=%q status=%d", out, stderr, status)
	}
	out, stderr, status = toolsCommand(t, dir, "", "run", ".")
	const want = "GO4T MEETS GO\nnanana\n[typed, externs, keep, lists]\ngo4t.example\nhttps://go4t.example:8443/guide\n"
	if out != want || stderr != "" || status != 0 {
		t.Fatalf("run example: stdout=%q stderr=%q status=%d", out, stderr, status)
	}
}
