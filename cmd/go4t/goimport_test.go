package main

import (
	"os"
	"os/exec"
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

func TestGoImportHandleExports(t *testing.T) {
	dir := toolsModule(t)
	t.Setenv("GO4T_CACHE", t.TempDir())
	file := toolsWrite(t, filepath.Join(dir, "h.gt"), `package main
import url "go:net/url"
type Envelope = Envelope { url: url.URL, urls: List[Option[url.URL]], named: Map[String, url.URL], pair: (url.URL, url.URL) }
type Box[A] = Box { value: A }
func host(u url.URL) IO[String] { url.urlHostname(u) }
func parse(s String) IO[url.URL] { url.parse(s) }
func keep(u url.URL) url.URL { u }
func echoEnvelope(v Envelope) Envelope { v }
func boxed(v Box[url.URL]) Box[url.URL] { v }
func main() IO[Unit] { ioPure(unit()) }
`)
	toolsWrite(t, filepath.Join(dir, "main.go"), `package main
import (
	"context"
	"fmt"
	"net/url"
	"clitoolstest/out"
	"go4t/rt"
)
func main() {
	ctx := context.Background()
	var u *url.URL
	var err error
	u, err = hx.Parse("https://example.com/x")(ctx)
	if err != nil { panic(err) }
	host, err := hx.Host(u)(ctx)
	if err != nil || host != "example.com" { panic(fmt.Sprintf("parsed host: %q, %v", host, err)) }
	direct := &url.URL{Scheme: "https", Host: "go.example:443"}
	host, err = hx.Host(direct)(ctx)
	if err != nil || host != "go.example" { panic(fmt.Sprintf("direct host: %q, %v", host, err)) }
	if hx.Keep(u) != u || hx.Keep(direct) != direct || hx.Keep(nil) != nil { panic("handle identity lost") }
	input := hx.Envelope{
		Url: direct,
		Urls: []rt.Option[*url.URL]{rt.Some(u), rt.None[*url.URL]()},
		Named: map[string]*url.URL{"parsed": u},
		Pair: rt.Tuple2[*url.URL, *url.URL]{F0: u, F1: direct},
	}
	got := hx.EchoEnvelope(input)
	if got.Url != direct || len(got.Urls) != 2 || !got.Urls[0].Present || got.Urls[0].Value != u || got.Urls[1].Present ||
		got.Named["parsed"] != u || got.Pair.F0 != u || got.Pair.F1 != direct { panic("nested handle identity lost") }
	if hx.Boxed(hx.BoxUrlURL{Value: u}).Value != u { panic("generic handle field identity lost") }
	fmt.Println("handle exports ok")
}
`)
	var cold string
	for _, phase := range []string{"cold", "warm"} {
		out, stderr, status := toolsCommand(t, dir, "", "build", "--emit-go", "out", "--package", "hx", file)
		if status != 0 || out != "" || stderr != "" {
			t.Fatalf("%s build: stdout=%q stderr=%q status=%d", phase, out, stderr, status)
		}
		data, err := os.ReadFile(filepath.Join(dir, "out", "go4t_export.go"))
		if err != nil {
			t.Fatal(err)
		}
		source := string(data)
		for _, absent := range []string{"type UrlURL ", "// not exported: host ", "// not exported: parse "} {
			if strings.Contains(source, absent) {
				t.Fatalf("%s export contains %q:\n%s", phase, absent, source)
			}
		}
		if phase == "cold" {
			cold = source
			t.Logf("generated handle exports:\n%s", source)
		} else if source != cold {
			t.Fatal("cached handle exports differ from cold build")
		}
		command := exec.Command("go", "run", ".")
		command.Dir = dir
		if output, err := command.CombinedOutput(); err != nil || string(output) != "handle exports ok\n" {
			t.Fatalf("%s consumer: output=%q error=%v", phase, output, err)
		}
	}
}

func TestGoImportInterfaceHandleExports(t *testing.T) {
	dir := toolsModule(t)
	t.Setenv("GO4T_CACHE", t.TempDir())
	file := toolsWrite(t, filepath.Join(dir, "reader.gt"), `package main
import goio "go:io"
func reader(r goio.Reader) IO[goio.Reader] { ioPure(r) }
func main() IO[Unit] { ioPure(unit()) }
`)
	out, stderr, status := toolsCommand(t, dir, "", "build", "--emit-go", "out", "--package", "hx", file)
	if status != 0 || out != "" || stderr != "" {
		t.Fatalf("build: stdout=%q stderr=%q status=%d", out, stderr, status)
	}
	data, err := os.ReadFile(filepath.Join(dir, "out", "go4t_export.go"))
	if err != nil {
		t.Fatal(err)
	}
	source := string(data)
	for _, want := range []string{
		`import go0 "io"`,
		"func Reader(p0 go0.Reader) func(context.Context) (go0.Reader, error)",
		"return v.(go0.Reader)",
	} {
		if !strings.Contains(source, want) {
			t.Errorf("missing %q:\n%s", want, source)
		}
	}
	for _, absent := range []string{"type IoReader ", "*go0.Reader", "// not exported: reader "} {
		if strings.Contains(source, absent) {
			t.Errorf("unexpected %q:\n%s", absent, source)
		}
	}
	toolsWrite(t, filepath.Join(dir, "main.go"), `package main
import (
	"context"
	"fmt"
	"io"
	"strings"
	"clitoolstest/out"
)
func main() {
	input := strings.NewReader("native interface")
	var reader io.Reader
	reader, err := hx.Reader(input)(context.Background())
	if err != nil { panic(err) }
	if reader != input { panic("interface identity lost") }
	data, err := io.ReadAll(reader)
	if err != nil || string(data) != "native interface" { panic("interface cannot be used") }
	fmt.Println("interface exports ok")
}
`)
	command := exec.Command("go", "run", ".")
	command.Dir = dir
	if output, err := command.CombinedOutput(); err != nil || string(output) != "interface exports ok\n" {
		t.Fatalf("consumer: output=%q error=%v", output, err)
	}
}
