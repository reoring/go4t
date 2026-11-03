package codegen

import (
	"bytes"
	"testing"

	"go4t/ast"
	"go4t/modules"
	"go4t/parser"
	"go4t/types"
)

func generateIOTailFixture(t *testing.T, source string) map[string][]byte {
	t.Helper()
	file, err := parser.Parse(source)
	if err != nil {
		t.Fatal(err)
	}
	loaded, err := modules.LoadFiles([]*ast.File{file}, ".")
	if err != nil {
		t.Fatal(err)
	}
	program, err := types.Check(loaded)
	if err != nil {
		t.Fatal(err)
	}
	generated, err := Generate(program, "main")
	if err != nil {
		t.Fatal(err)
	}
	return generated
}

func TestIOTailLiftCodegen(t *testing.T) {
	for _, tc := range []struct{ name, implicit, explicit string }{
		{"tail", `42`, `ioPure(42)`},
		{"call", `identity(42)`, `ioPure(identity(42))`},
		{"mixed if", `if flag { 1 } else { ioPure(2) }`, `if flag { ioPure(1) } else { ioPure(2) }`},
		{"pure if", `if flag { 1 } else { 2 }`, `ioPure(if flag { 1 } else { 2 })`},
		{"pure match", `match flag { true => 1; false => 2 }`, `ioPure(match flag { true => 1; false => 2 })`},
		{"mixed match", `match flag { true => 1; false => do { ioPure(0); 2 } }`, `match flag { true => ioPure(1); false => do { ioPure(0); ioPure(2) } }`},
		{"do", `do { n <- ioPure(1); n + 1 }`, `do { n <- ioPure(1); ioPure(n + 1) }`},
		{"let", `let action: IO[Int] = 42; action`, `let action: IO[Int] = ioPure(42); action`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			prefix := "package main\nfunc identity[A](a A) A { a }\nfunc value(flag Bool) IO[Int] { "
			suffix := " }\nfunc main() IO[Unit] { do { n <- value(true); println(show(n)) } }\n"
			implicit := generateIOTailFixture(t, prefix+tc.implicit+suffix)
			explicit := generateIOTailFixture(t, prefix+tc.explicit+suffix)
			if len(implicit) != len(explicit) {
				t.Fatalf("generated files = %d, want %d", len(implicit), len(explicit))
			}
			for name, want := range explicit {
				if !bytes.Equal(implicit[name], want) {
					t.Errorf("%s differs between implicit and explicit ioPure", name)
				}
			}
		})
	}
}

func TestIOTailLiftShadowRuntime(t *testing.T) {
	generated := generateIOTailFixture(t, `package main
func ioPure(n Int) Bool { false }
func value() IO[Int] { let ioPure = func(n Int) => false; 42 }
func main() IO[Unit] { do { n <- value(); println(show(n)); unit() } }
`)
	if got := runGeneratedModules(t, generated); got != "42\n" {
		t.Fatalf("output = %q, want 42", got)
	}
}
