package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCommandsAcceptDirectory(t *testing.T) {
	dir := toolsModule(t)
	toolsWrite(t, filepath.Join(dir, "gofx.mod"), "module example\n")
	toolsWrite(t, filepath.Join(dir, "main.gfx"), `package main
import "geometry"
func main() IO[Unit] { println(show(geometry.area(6, 7))) }
func testArea() Bool { geometry.area(6, 7) == 42 }
`)
	toolsWrite(t, filepath.Join(dir, "second.gfx"), "package main\nfunc answer() Int { 42 }\n")
	if err := os.Mkdir(filepath.Join(dir, "geometry"), 0755); err != nil {
		t.Fatal(err)
	}
	toolsWrite(t, filepath.Join(dir, "geometry", "shape.gfx"), "package geometry\nfunc area(x Int, y Int) Int { x * y }\n")
	for _, test := range []struct {
		args []string
		want string
	}{
		{[]string{"check", dir}, ""},
		{[]string{"build", "-o", filepath.Join(dir, "out"), dir}, ""},
		{[]string{"run", dir}, "42\n"},
		{[]string{"test", dir}, "PASS testArea\n1 passed, 0 failed\n"},
		{[]string{"eval", "answer()", dir}, "42\n"},
		{[]string{"eval", "geometry.area(3, 4)", dir}, "12\n"},
	} {
		out, stderr, status := toolsCommand(t, dir, "", test.args...)
		if status != 0 || out != test.want || stderr != "" {
			t.Fatalf("%v: status=%d stdout=%q stderr=%s", test.args, status, out, stderr)
		}
	}
	if _, err := os.Stat(filepath.Join(dir, "out", "geometry.go")); err != nil {
		t.Fatal(err)
	}
}
func TestDirectoryRejectsMixedPackages(t *testing.T) {
	dir := toolsModule(t)
	toolsWrite(t, filepath.Join(dir, "a.gfx"), "package first\n")
	toolsWrite(t, filepath.Join(dir, "b.gfx"), "package second\n")
	if err := run([]string{"check", dir}); err == nil || !strings.Contains(err.Error(), "package second differs from first") {
		t.Fatalf("mixed directory = %v", err)
	}
}
