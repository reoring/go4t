package main

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"go4t/types"
)

func TestFullTypesFlag(t *testing.T) {
	file := filepath.Join(t.TempDir(), "bad.gt")
	nested := strings.Repeat("Option[", 24) + "Int" + strings.Repeat("]", 24)
	source := "package main\nfunc bad() " + nested + " { true }\nfunc main() IO[Unit] { ioPure(unit()) }\n"
	if err := os.WriteFile(file, []byte(source), 0600); err != nil {
		t.Fatal(err)
	}
	for _, command := range []string{"check", "build", "run"} {
		// A default call after a full call must not inherit process-global state.
		for _, full := range []bool{false, true, false} {
			args := []string{command, file}
			if full {
				args = append(args, "--full-types")
			}
			err := run(args)
			var checked *types.Error
			if !errors.As(err, &checked) {
				t.Fatalf("%v: %v", args, err)
			}
			want := "type mismatch: expected Option[Option[Option[…]]], got Bool"
			if full {
				want = "type mismatch: expected " + nested + ", got Bool"
			}
			if checked.Msg != want {
				t.Errorf("%v: %q, want %q", args, checked.Msg, want)
			}
		}
		err := run([]string{command, "--full-types", file})
		var checked *types.Error
		if !errors.As(err, &checked) || strings.Contains(checked.Msg, "…") {
			t.Errorf("leading flag %s: %v", command, err)
		}
	}
	if err := run([]string{"check", "--full-types"}); err == nil || err.Error() != usage {
		t.Fatalf("missing input: %v", err)
	}
	err := run([]string{"run", file, "--", "--full-types"})
	var checked *types.Error
	if !errors.As(err, &checked) || !strings.Contains(checked.Msg, "…") {
		t.Fatalf("program argument changed compiler printing: %v", err)
	}
}

func TestAliasCodegenTransparency(t *testing.T) {
	dir := t.TempDir()
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	mod := "module aliasprinttest\n\ngo 1.22\n\nrequire go4t v0.0.0\nreplace go4t => " + root + "\n"
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte(mod), 0600); err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(dir, "main.gt")
	source := `package main
alias Number = Int
alias Pair[A] = (A, A)
alias Handler = func(Number) Number
alias Text = String
alias Names = List[Text]
alias Effect[A] = IO[A]
alias Done = Unit
extern func native(xs Names, separator Text) Text = "strings".Join
func apply(f Handler, n Number) Number { f(n) }
func first(p Pair[Number]) Number { p.0 }
func increment(n Number) Number { n + 1 }
func render(xs Names) String { show(xs) }
func main() Effect[Done] {
 do {
  println(show(apply(increment, first((1, 2)))))
  println(native(["native", "alias"], " "))
  println(render(["alias"]))
 }
}
`
	if err := os.WriteFile(file, []byte(source), 0600); err != nil {
		t.Fatal(err)
	}
	if err := run([]string{"build", "--full-types", file}); err != nil {
		t.Fatal(err)
	}
	command := exec.Command("go", "run", "./main_gen")
	command.Dir = dir
	output, err := command.CombinedOutput()
	if err != nil || string(output) != "2\nnative alias\n[alias]\n" {
		t.Fatalf("generated aliases: %v\n%s", err, output)
	}
}

func TestFullTypesForeignSignature(t *testing.T) {
	dir := t.TempDir()
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	mod := "module foreignsigtest\n\ngo 1.22\n\nrequire go4t v0.0.0\nreplace go4t => " + root + "\n"
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte(mod), 0600); err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(dir, "foreign.gt")
	nested := strings.Repeat("Option[", 24) + "Int" + strings.Repeat("]", 24)
	source := "package main\nalias Text = String\nextern func bad(x " + nested + ") Text = \"strings\".ToUpper\nfunc main() IO[Unit] { ioPure(unit()) }\n"
	if err := os.WriteFile(file, []byte(source), 0600); err != nil {
		t.Fatal(err)
	}
	for _, command := range []string{"build", "run"} {
		for _, full := range []bool{false, true} {
			args := []string{command, file}
			want := "does not match declared type func(Option[Option[…]]) Text"
			if full {
				args = append(args, "--full-types")
				want = "does not match declared type func(" + nested + ") Text"
			}
			err := run(args)
			if err == nil || !strings.Contains(err.Error(), want) {
				t.Fatalf("%v: %v, want %q", args, err, want)
			}
		}
	}
}
