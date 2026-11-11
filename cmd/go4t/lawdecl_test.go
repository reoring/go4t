package main

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestLawInlineSampleCLI(t *testing.T) {
	dir := t.TempDir()
	file := toolsWrite(t, filepath.Join(dir, "inline.gt"), `package main
import "std/regex"
law implicit(a Int) { a == a }
law replacement(text String) { regex.regexReplace("z", text, "z") == text } proof { sample(10) }
law predicate() { regex.regexMatch("a", "a") } proof { sample(1) }
func main() IO[Unit] { ioPure(unit()) }
`)
	out, stderr, status := toolsCommand(t, dir, "", "check", file)
	if status != 0 || out != "" || strings.Count(stderr, "tested by go4t test (sample ") != 2 {
		t.Fatalf("check: status=%d stdout=%q stderr=%q", status, out, stderr)
	}
	out, stderr, status = toolsCommand(t, dir, "", "test", file)
	if status != 0 || stderr != "" || !strings.Contains(out, "2 passed, 0 failed") {
		t.Fatalf("test: status=%d stdout=%q stderr=%q", status, out, stderr)
	}
	bad := toolsWrite(t, filepath.Join(dir, "false.gt"), `package main
import "std/regex"
law falsePredicate(text String) { regex.regexMatch("^a$", text) } proof { sample(10) }
`)
	out, stderr, status = toolsCommand(t, dir, "", "test", bad)
	if status != 1 || stderr != "" || !strings.Contains(out, "counterexample (seed ") || !strings.Contains(out, "0 passed, 1 failed") {
		t.Fatalf("false sample: status=%d stdout=%q stderr=%q", status, out, stderr)
	}
}
