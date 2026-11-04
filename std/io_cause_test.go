package std_test

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"go4t/ast"
	"go4t/codegen"
	"go4t/modules"
	"go4t/parser"
	"go4t/types"
)

func TestGoErrorTypeContracts(t *testing.T) {
	if got := types.KindString(types.GoError.Kind); got != "*" {
		t.Fatalf("GoError kind = %q, want *", got)
	}
	for _, tt := range []struct {
		name, source, wantError string
	}{
		{"opaque value", `func keep(e GoError) GoError { e }`, ""},
		{"qualified value", `func keep(e prelude.GoError) prelude.GoError { e }`, ""},
		{"reserved builtin", `type GoError = Wrapped`, "duplicate declaration GoError"},
		{"no value constructor", `func bad() GoError { GoError }`, "unknown identifier GoError"},
		{"no equality", `func bad(e GoError) Bool { e == e }`, "no instance Eq[GoError]"},
		{"no show", `func bad(e GoError) String { show(e) }`, "no instance Show[GoError]"},
		{"no debug", `func bad(e GoError) String { debug(e) }`, "no instance Debug[GoError]"},
		{"cause accessor", `func original(f io.IoFailure) Option[GoError] { io.cause(f) }`, ""},
	} {
		t.Run(tt.name, func(t *testing.T) {
			user, err := parser.Parse("package main\nimport \"std/prelude\"\nimport \"std/io\"\n" + tt.source + "\nfunc main() IO[Unit] { ioPure(unit()) }\n")
			if err != nil {
				t.Fatal(err)
			}
			user.Name = "go_error_contract.gt"
			mods, err := modules.LoadFiles([]*ast.File{user}, ".")
			if err != nil {
				t.Fatal(err)
			}
			_, err = types.Check(mods)
			if tt.wantError == "" {
				if err != nil {
					t.Fatal(err)
				}
			} else if err == nil || !strings.Contains(err.Error(), tt.wantError) {
				t.Fatalf("error = %v, want %q", err, tt.wantError)
			}
		})
	}
}

func TestTryIOCauseSurface(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "missing")
	source := fmt.Sprintf(`package main
import "std/io"
import "std/prelude"

func rethrow(f io.IoFailure) IO[String] {
  io.ioRaiseCause(f)
}

func localFailure() IO[String] {
  ioFail("local")
}

func inspect(result Result[io.IoFailure, String]) IO[Unit] {
  match result {
    Err(io.IoFailure{error: io.NotFound(message), cause: Some(original)}) => do {
      println(show(io.goErrorMessage(original) == message))
      let failure = io.IoFailure{error: io.NotFound(message), cause: Some(original)}
      println(show(match io.cause(failure) {
        Some(e) => io.goErrorMessage(e) == message
        None => false
      }))
      again <- io.tryIOCause(rethrow(failure))
      println(show(match again {
        Err(io.IoFailure{error: io.NotFound(next), cause: Some(e)}) => next == message && io.goErrorMessage(e) == message
        _ => false
      }))
      legacy <- io.tryIO(rethrow(failure))
      println(show(match legacy {
        Err(io.NotFound(next)) => next == message
        _ => false
      }))
    }
    _ => ioFail("expected NotFound with original Go cause")
  }
}

func main() IO[Unit] {
  do {
    captured <- io.tryIOCause(io.readFile(%q))
    inspect(captured)
    local <- io.tryIOCause(localFailure())
    println(show(match local {
      Err(io.IoFailure{error: io.Failure("local"), cause: None}) => true
      _ => false
    }))
    println(debug(io.IoFailure{error: io.Failure("local"), cause: None}))
    success <- io.tryIOCause(ioPure(42))
    println(show(match success {
      Ok(42) => true
      _ => false
    }))
  }
}
`, missing)
	user, err := parser.Parse(source)
	if err != nil {
		t.Fatal(err)
	}
	user.Name = "io_cause_test.gt"
	mods, err := modules.LoadFiles([]*ast.File{user}, ".")
	if err != nil {
		t.Fatal(err)
	}
	program, err := types.Check(mods)
	if err != nil {
		t.Fatal(err)
	}
	generated, err := codegen.Generate(program, "main")
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	root, err := filepath.Abs("..")
	if err != nil {
		t.Fatal(err)
	}
	generated["go.mod"] = []byte("module iocausetest\n\ngo 1.22\n\nrequire go4t v0.0.0\nreplace go4t => " + root + "\n")
	for name, content := range generated {
		if err := os.WriteFile(filepath.Join(dir, name), content, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	command := exec.CommandContext(ctx, "go", "run", ".")
	command.Dir = dir
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("go run generated cause program: %v\n%s", err, output)
	}
	want := "true\ntrue\ntrue\ntrue\ntrue\nIoFailure(Failure(\"local\"), None)\ntrue\n"
	if string(output) != want {
		t.Fatalf("output = %q, want %q", output, want)
	}
}
