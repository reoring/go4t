package types_test

import (
	"errors"
	"os"
	"strings"
	"testing"

	"gofx/ast"
	"gofx/parser"
	"gofx/prelude"
	"gofx/types"
)

func checkIOEffects(t *testing.T, source string) (*types.Program, error) {
	t.Helper()
	var files []*ast.File
	for _, src := range []string{prelude.Source, source} {
		file, err := parser.Parse(src)
		if err != nil {
			t.Fatal(err)
		}
		files = append(files, file)
	}
	return types.Check(files)
}

func TestPreludeResultDoTyping(t *testing.T) {
	p, err := checkIOEffects(t, `package main
func validate(text String) Result[String, Int] {
  do {
    n <- okOr(parseInt(text), "not an int")
    if n > 0 { Ok(n) } else { Err("not positive") }
    pure(n + 1)
  }
}
func generic[E, A](r Result[E, A]) Result[E, A] { do { a <- r; pure(a) } }
func captured() IO[Result[String, Int]] { tryIO(liftResult(validate("5"))) }
func main() IO[Unit] { ioPure(unit()) }
`)
	if err != nil {
		t.Fatal(err)
	}
	for name, want := range map[string]string{"validate": "Result[String, Int]", "generic": "Result[E, A]"} {
		ann := types.AnnOf(p.Funcs[name].Decl.Body.Result)
		if got := types.TypeString(ann.Type); got != want {
			t.Errorf("%s type = %s, want %s", name, got, want)
		}
		if ann.Monad == nil {
			t.Fatalf("%s has no Monad dictionary", name)
		}
		assertInstanceDict(t, *ann.Monad, "Monad", "Result")
	}
}

func TestPreludeResultTypingErrors(t *testing.T) {
	for _, tt := range []struct{ name, source, want string }{
		{"reserved prelude type", `type Result[E, A] = Failed(E) | Passed(A)`, "duplicate declaration Result"},
		{"mixed monads", `func bad() Result[String, Int] { do { x <- Some(1); Ok(x) } }`, "type mismatch"},
		{"mixed errors", `func bad(r Result[Int, Int]) Result[String, Int] { do { x <- r; Ok(x) } }`, "type mismatch"},
		{"missing Show", `type Failure = Failure
func bad() IO[Int] { ioFailWith(Failure) }`, "no instance Show[Failure]"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			p, err := checkIOEffects(t, "package main\n"+tt.source+"\nfunc main() IO[Unit] { ioPure(unit()) }")
			var typed *types.Error
			if p != nil || !errors.As(err, &typed) || !strings.Contains(typed.Msg, tt.want) || typed.Pos.Line < 1 || typed.Pos.Col < 1 {
				t.Fatalf("Check = (%v, %v), want positioned %q", p, err, tt.want)
			}
		})
	}
}

func TestMigratedResultExamples(t *testing.T) {
	for _, name := range []string{"bank", "calc", "inventory", "io-effects"} {
		t.Run(name, func(t *testing.T) {
			source, err := os.ReadFile("../examples/" + name + "/main.gfx")
			if err != nil {
				t.Fatal(err)
			}
			if _, err := checkIOEffects(t, string(source)); err != nil {
				t.Fatal(err)
			}
		})
	}
}
