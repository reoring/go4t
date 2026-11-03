package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestGoImportTransitiveClassifier(t *testing.T) {
	dir := toolsModule(t)
	cache := t.TempDir()
	t.Setenv("GO4T_CACHE", cache)
	t.Setenv("GO4T_TRACE", "1")
	for name, source := range map[string]string{
		"dep/dep.go": `package dep
type missing struct{}
func (missing) Error() string { return "gone" }
func NewError() error { return missing{} }
func IsMissing(err error) bool { _, ok := err.(missing); return ok }
`,
		"lib/lib.go": `package lib
import "clitoolstest/dep"
func Fail() error { return dep.NewError() }
`,
	} {
		file := filepath.Join(dir, name)
		if err := os.MkdirAll(filepath.Dir(file), 0o755); err != nil {
			t.Fatal(err)
		}
		toolsWrite(t, file, source)
	}
	file := toolsWrite(t, filepath.Join(dir, "main.gt"), `package main
import "std/io"
import "go:clitoolstest/lib"
func main() IO[Unit] {
  do {
    result <- io.tryIO(lib.fail())
    println(match result { Err(e) => show(e); Ok(_) => "unexpected success" })
  }
}
func testMissing() IO[Bool] {
  do {
    result <- io.tryIO(lib.fail())
    ioPure(match result { Err(e) => show(e) == "Missing: gone"; Ok(_) => false })
  }
}
`)
	for _, phase := range []string{"cold", "warm"} {
		out, stderr, status := toolsCommand(t, dir, "", "run", file)
		if status != 0 || out != "Missing: gone\n" {
			t.Fatalf("%s run: stdout=%q stderr=%q status=%d", phase, out, stderr, status)
		}
		if strings.Count(stderr, "go4t bind classifier clitoolstest/dep\n") != 1 {
			t.Fatalf("%s did not register the dependency exactly once:\n%s", phase, stderr)
		}
		if phase == "warm" {
			for _, want := range []string{
				"go4t bind cache hit clitoolstest/lib\n",
				"go4t bind cache hit clitoolstest/dep\n",
				"go4t bind cache classifier hit clitoolstest/lib\n",
				"go4t bind cache classifier hit clitoolstest/dep\n",
			} {
				if !strings.Contains(stderr, want) {
					t.Errorf("warm run missing %q:\n%s", want, stderr)
				}
			}
			if strings.Contains(stderr, "classifier miss") {
				t.Fatalf("warm run reloaded a closure package:\n%s", stderr)
			}
		}
	}
	for _, tc := range []struct {
		args []string
		want string
	}{
		{[]string{"test", file}, "PASS testMissing\n1 passed, 0 failed\n"},
		{[]string{"eval", "testMissing()", file}, "true\n"},
	} {
		out, stderr, status := toolsCommand(t, dir, "", tc.args...)
		if status != 0 || out != tc.want {
			t.Fatalf("%v: stdout=%q stderr=%q status=%d", tc.args, out, stderr, status)
		}
	}
	binder := newGoImportBinder(dir)
	if _, err := binder.Bind("clitoolstest/lib"); err != nil {
		t.Fatal(err)
	}
	if len(binder.visited) != 0 || len(binder.sideEffectImports) != 0 || len(binder.bound) != 1 {
		t.Fatal("check/LSP mode walked the dependency closure")
	}
}

func TestClassifierCache(t *testing.T) {
	entry := t.TempDir()
	for _, present := range []bool{false, true} {
		if err := writeClassifierCache(entry, present); err != nil {
			t.Fatal(err)
		}
		got, err := readClassifierCache(entry)
		if err != nil || got != present {
			t.Fatalf("read classifier %v: %v, %v", present, got, err)
		}
	}
	toolsWrite(t, filepath.Join(entry, "classifier"), "corrupt")
	if _, err := readClassifierCache(entry); err == nil {
		t.Fatal("corrupt classifier metadata was a hit")
	}
}

func TestGoImportRecordOwnerKeys(t *testing.T) {
	dir := toolsModule(t)
	t.Setenv("GO4T_CACHE", t.TempDir())
	for name, source := range map[string]string{
		"left/left.go": `package left
type Status struct { Count int }
type Option struct { Label string }
func Echo(v Status) Status { return v }
`,
		"right/right.go": `package right
type Status struct { Message string; Ready bool }
func Echo(v Status) Status { return v }
`,
		"consumer/consumer.go": `package consumer
import (
 "clitoolstest/left"
 "clitoolstest/right"
)
func Echo(a left.Status, b right.Status) (left.Status, right.Status) { return a,b }
func EchoOption(v left.Option) left.Option { return v }
`,
	} {
		file := filepath.Join(dir, name)
		if err := os.MkdirAll(filepath.Dir(file), 0o755); err != nil {
			t.Fatal(err)
		}
		toolsWrite(t, file, source)
	}
	file := toolsWrite(t, filepath.Join(dir, "main.gt"), `package main
import "go:clitoolstest/left"
import "go:clitoolstest/right"
import "go:clitoolstest/consumer"
func main() IO[Unit] {
  do {
    let (a, b) = consumer.echo(left.Status{count: 7}, right.Status{message: "ready", ready: true})
    println(debug(left.echo(a)))
    println(debug(right.echo(b)))
    println(debug(consumer.echoOption(left.OptionGo{label: "owned"})))
  }
}
`)
	for _, phase := range []string{"cold", "warm"} {
		out, stderr, status := toolsCommand(t, dir, "", "run", file)
		const want = "Status{count: 7}\nStatus{message: \"ready\", ready: true}\nOptionGo{label: \"owned\"}\n"
		if status != 0 || out != want || stderr != "" {
			t.Fatalf("%s run: stdout=%q stderr=%q status=%d", phase, out, stderr, status)
		}
	}
}
