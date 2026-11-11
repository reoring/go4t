package main

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestRecursionWarningCommands(t *testing.T) {
	dir := toolsModule(t)
	file := toolsWrite(t, filepath.Join(dir, "warning.gt"), `package main
func sumTo(n Int) Int { if n == 0 { 0 } else { n + sumTo(n - 1) } }
func testSum() Bool { sumTo(5) == 15 }
func main() IO[Unit] { println(show(sumTo(5))) }
`)
	message := "sumTo calls itself in a non-tail position and no argument is a structural part of its parameter; go4t cannot bound the recursion depth, and stack exhaustion cannot be recovered. Use an accumulator so the call is a tail call"
	for _, args := range [][]string{
		{"check", file},
		{"build", "-o", filepath.Join(dir, "warning"), file},
		{"run", file},
		{"test", file},
		{"eval", "sumTo(5)", file},
	} {
		t.Run(args[0], func(t *testing.T) {
			out, diagnostics, status := toolsCommand(t, dir, "", args...)
			want := file + ":2:52: warning: " + message + "\n"
			if status != 0 || strings.Count(diagnostics, message) != 1 || !strings.Contains(diagnostics, want) {
				t.Fatalf("%v: status=%d stdout=%q stderr=%q", args, status, out, diagnostics)
			}
		})
	}
}
