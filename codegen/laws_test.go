package codegen

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"go4t/modules"
	"go4t/types"
)

func TestLawsAndProofsDoNotChangeGeneratedGo(t *testing.T) {
	file := filepath.Join(t.TempDir(), "main.gt")
	const base = "package main\nfunc identity(n Int) Int { n }\nfunc main() IO[Unit] { println(show(identity(42))) }\n"
	generateSource := func(source string) map[string][]byte {
		t.Helper()
		if err := os.WriteFile(file, []byte(source), 0o644); err != nil {
			t.Fatal(err)
		}
		loaded, err := modules.Load([]string{file})
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
	without := generateSource(base)
	// Deliberately false: generation must neither verify nor emit proofs.
	with := generateSource(base + "law identityZero(n Int) { identity(n) == 0 }\nproof identityZero(n) { refl }\n")
	if len(with) != len(without) {
		t.Fatalf("generated files: with laws %d, without %d", len(with), len(without))
	}
	for name, want := range without {
		if !bytes.Equal(with[name], want) {
			t.Errorf("laws changed generated %s:\n%s", name, with[name])
		}
	}
	if got := runGeneratedModules(t, with); got != "42\n" {
		t.Fatalf("generated program output = %q", got)
	}
}
