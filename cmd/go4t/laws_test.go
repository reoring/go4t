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
