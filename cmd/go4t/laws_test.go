package main

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestLawCheckAndRun(t *testing.T) {
	if os.Getenv("GO4T_LAW_CLI_TEST") == "1" {
		for i, arg := range os.Args {
			if arg == "--" {
				os.Args = append([]string{"go4t"}, os.Args[i+1:]...)
				main()
				os.Exit(0)
			}
		}
		t.Fatal("missing helper argument separator")
	}
	source, err := os.ReadFile("../../laws/testdata/cancel_bad1.gt")
	if err != nil {
		t.Fatal(err)
	}
	// A second failure ensures check prints every positioned diagnostic.
	source = append(source, []byte("\nlaw alsoBad(o Order) { cancel(cancel(o)) == cancel(o) }\nproof alsoBad(o) { match o { Pending(n) => refl; Cancelled(n) => refl; Shipped(n) => refl } }\n")...)
	dir := t.TempDir()
	file := filepath.Join(dir, "cancel_bad1.gt")
	if err := os.WriteFile(file, source, 0o644); err != nil {
		t.Fatal(err)
	}
	for _, command := range []string{"check", "run"} {
		t.Run(command, func(t *testing.T) {
			cmd := exec.Command(os.Args[0], "-test.run=^TestLawCheckAndRun$", "--", command, file)
			cmd.Env = append(os.Environ(), "GO4T_LAW_CLI_TEST=1")
			output, err := cmd.CombinedOutput()
			if command == "run" {
				if err != nil || len(output) != 0 {
					t.Fatalf("run = %v, output:\n%s", err, output)
				}
				return
			}
			var exit *exec.ExitError
			if !errors.As(err, &exit) || exit.ExitCode() != 1 {
				t.Fatalf("check = %v, want exit 1; output:\n%s", err, output)
			}
			for _, name := range []string{"cancelIdempotent", "alsoBad"} {
				if !strings.Contains(string(output), ": law "+name+": ") {
					t.Errorf("missing %s diagnostic:\n%s", name, output)
				}
			}
			if !strings.Contains(string(output), file+":17:53: law cancelIdempotent: ") || strings.Count(string(output), "^~~~") != 2 || !strings.Contains(string(output), "\nproof cancelIdempotent(o)") {
				t.Errorf("missing positioned source excerpts:\n%s", output)
			}
		})
	}
}

func TestCounterexampleLawCLI(t *testing.T) {
	source, err := os.ReadFile("../../laws/testdata/counterexample_cli.gt")
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	file := toolsWrite(t, filepath.Join(dir, "diagnostic.gt"), string(source))
	out, stderr, status := toolsCommand(t, dir, "", "check", file)
	if status != 1 || out != "" {
		t.Fatalf("check: status=%d stdout=%q stderr=%q", status, out, stderr)
	}
	for _, want := range []string{
		"counterexample: a = 9223372036854775807\n  LHS: false\n  RHS: true",
		"no counterexample in 128 candidates: the law may hold; this proof needs a case split, rewrite or induction",
	} {
		if !strings.Contains(stderr, want) {
			t.Fatalf("missing %q:\n%s", want, stderr)
		}
	}
}

func TestRequiresLawWarningCLI(t *testing.T) {
	dir := t.TempDir()
	file := toolsWrite(t, filepath.Join(dir, "warning.gt"), `package main
law vacuous(a Int) requires 0 <= a && a <= 10 && a * 2 == 21 { a == a }
proof vacuous(a) { refl }
func main() IO[Unit] { ioPure(unit()) }
`)
	out, stderr, status := toolsCommand(t, dir, "", "check", file)
	want := "warning: law vacuous: no arguments satisfying requires were found in 128 candidates; the law may say nothing"
	if status != 0 || out != "" || !strings.Contains(stderr, file+":2:") || strings.Count(stderr, want) != 1 || !strings.Contains(stderr, "\nlaw vacuous(a Int)") || !strings.Contains(stderr, "^~~~") {
		t.Fatalf("check: status=%d stdout=%q stderr=%q", status, out, stderr)
	}
	toolsWrite(t, file, `package main
law literal(a Int) requires a == 123456 { (a > 0) == true }
proof literal(a) { refl }
func main() IO[Unit] { ioPure(unit()) }
`)
	out, stderr, status = toolsCommand(t, dir, "", "check", file)
	if status != 0 || out != "" || stderr != "" {
		t.Fatalf("seeded witness: status=%d stdout=%q stderr=%q", status, out, stderr)
	}
}

// A precondition is resolved in the module that declares the law, like the
// law's sides: its unqualified names never bind to another module's
// declarations, and module-qualified names are available.
func TestRequiresResolvesInDeclaringModuleCLI(t *testing.T) {
	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, "lib"), 0o755); err != nil {
		t.Fatal(err)
	}
	lib := toolsWrite(t, filepath.Join(dir, "lib", "lib.gt"), `package lib
func ok(a Int) Bool { true }
func small(a Int) Bool { a < 100 }
law bounded(a Int) requires ok(a) { (a < 100) == true }
proof bounded(a) { refl }
`)
	// The entry module's ok is stronger than lib's. Binding lib's precondition
	// to it would prove the false law above.
	file := toolsWrite(t, filepath.Join(dir, "main.gt"), `package main
import "lib"
func ok(a Int) Bool { a < 100 }
func main() IO[Unit] { ioPure(unit()) }
`)
	out, stderr, status := toolsCommand(t, dir, "", "check", file)
	if status != 1 || out != "" || !strings.Contains(stderr, lib+":5:") || !strings.Contains(stderr, "law bounded: stuck normal forms differ") || !strings.Contains(stderr, "counterexample: a = 9223372036854775807") {
		t.Fatalf("captured precondition: status=%d stdout=%q stderr=%q", status, out, stderr)
	}

	toolsWrite(t, lib, `package lib
func small(a Int) Bool { a < 100 }
law bounded(a Int) requires small(a) { (a < 100) == true }
proof bounded(a) { refl }
`)
	toolsWrite(t, file, `package main
import "lib"
law viaQualified(a Int) requires lib.small(a) { (a < 100) == true }
proof viaQualified(a) { refl }
func main() IO[Unit] { ioPure(unit()) }
`)
	out, stderr, status = toolsCommand(t, dir, "", "check", file)
	if status != 0 || out != "" || stderr != "" {
		t.Fatalf("own and qualified helpers: status=%d stdout=%q stderr=%q", status, out, stderr)
	}
}

func TestSampleLawCLI(t *testing.T) {
	source, err := os.ReadFile("../../laws/testdata/sample/valid.gt")
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	file := toolsWrite(t, filepath.Join(dir, "sample.gt"), string(source))
	out, stderr, status := toolsCommand(t, dir, "", "check", file)
	if status != 0 || out != "" || strings.Count(stderr, "tested by go4t test (sample ") != 6 || strings.Count(stderr, "\n") != 6 {
		t.Fatalf("check: status=%d stdout=%q stderr=%q", status, out, stderr)
	}
	out, stderr, status = toolsCommand(t, dir, "", "test", file)
	if status != 0 || stderr != "" || !strings.Contains(out, "6 passed, 0 failed") {
		t.Fatalf("test: status=%d stdout=%q stderr=%q", status, out, stderr)
	}
	bad := toolsWrite(t, filepath.Join(dir, "false.gt"), `package main
import "std/regex"
law falseMatch(text String) { regex.regexMatch("^a$", text) == true }
proof falseMatch(text) { sample(30) }
`)
	out, stderr, status = toolsCommand(t, dir, "", "test", bad)
	if status != 1 || stderr != "" || !strings.Contains(out, "counterexample (seed ") || !strings.Contains(out, `text = ""`) || !strings.Contains(out, "0 passed, 1 failed") {
		t.Fatalf("false sample: status=%d stdout=%q stderr=%q", status, out, stderr)
	}
}

func TestSampleImportedLawCLI(t *testing.T) {
	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, "lib"), 0o755); err != nil {
		t.Fatal(err)
	}
	toolsWrite(t, filepath.Join(dir, "lib", "lib.gt"), `package lib
import "std/regex"
law replacement(text String) { regex.regexReplace("z", text, "z") == text }
proof replacement(text) { sample(8) }
`)
	file := toolsWrite(t, filepath.Join(dir, "main.gt"), "package main\nimport \"lib\"\n")
	out, stderr, status := toolsCommand(t, dir, "", "test", file)
	if status != 0 || stderr != "" || !strings.Contains(out, "1 passed, 0 failed") {
		t.Fatalf("imported sample: status=%d stdout=%q stderr=%q", status, out, stderr)
	}
}

func TestSampleStructuralDebugCLI(t *testing.T) {
	dir := t.TempDir()
	file := toolsWrite(t, filepath.Join(dir, "false.gt"), `package main
import "std/regex"
import "std/map"
type Plain = Tip | Link(Int, Plain)
type Custom = Custom(Int)
instance Debug[Custom] { debug(value) { "custom" } }
law falsePlain(value Plain, custom Custom, values List[Plain], table Map[Int, Plain]) {
  regex.regexMatch("a", "a") == false
}
proof falsePlain(value, custom, values, table) { sample(12) }
`)
	out, stderr, status := toolsCommand(t, dir, "", "test", file)
	if status != 1 || stderr != "" {
		t.Fatalf("status=%d stdout=%q stderr=%q", status, out, stderr)
	}
	for _, want := range []string{"counterexample (seed ", "value = Tip", "custom = custom", "values = Nil", "table = {", "0 passed, 1 failed"} {
		if !strings.Contains(out, want) {
			t.Fatalf("missing %q in %s", want, out)
		}
	}
}
