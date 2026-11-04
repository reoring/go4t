package std_test

import (
	"strings"
	"testing"

	"github.com/reoring/go4t/ast"
	"github.com/reoring/go4t/modules"
	"github.com/reoring/go4t/parser"
	"github.com/reoring/go4t/types"
)

func TestRefTypeContracts(t *testing.T) {
	if got := types.KindString(types.Ref.Kind); got != "(*) -> *" {
		t.Fatalf("Ref kind = %q, want (*) -> *", got)
	}
	for _, tt := range []struct {
		name, source, wantError string
	}{
		{"opaque value", `func keep[A](r Ref[A]) Ref[A] { r }`, ""},
		{"qualified value", `func keep[A](r ref.Ref[A]) ref.Ref[A] { r }`, ""},
		{"higher kind argument", `extern func keep[F[_], A](fa F[A]) F[A] = "example".Keep
func use(r Ref[Int]) Ref[Int] { keep(r) }`, ""},
		{"bare constructor", `func bad(r Ref) Int { 0 }`, "kind mismatch"},
		{"too many arguments", `func bad(r Ref[Int, Bool]) Int { 0 }`, "kind mismatch applying Ref to 2 arguments"},
		{"wrong element", `func bad(r Ref[Int]) Ref[String] { r }`, "type mismatch: expected Ref[String], got Ref[Int]"},
		{"reserved builtin", `type Ref[A] = Wrapped(A)`, "duplicate declaration Ref"},
		{"no value constructor", `func bad() Ref[Int] { Ref(1) }`, "unknown identifier Ref"},
		{"no equality", `func bad(r Ref[Int]) Bool { r == r }`, "no instance Eq[Ref[Int]]"},
		{"no show", `func bad(r Ref[Int]) String { show(r) }`, "no instance Show[Ref[Int]]"},
		{"no debug", `func bad(r Ref[Int]) String { debug(r) }`, "no instance Debug[Ref[Int]]"},
		{"no pure allocation", `func bad() Ref[Int] { ref.new(1) }`, "type mismatch: expected Ref[Int], got IO["},
		{"no pure read", `func bad(r Ref[Int]) Int { ref.get(r) }`, "type mismatch: expected Int, got IO["},
		{"set keeps element type", `func bad(r Ref[Int]) IO[Unit] { ref.set(r, "wrong") }`, "type mismatch: expected Int, got String"},
		{"modify is pure", `func bad(r Ref[Int]) IO[Unit] { ref.modify(r, func(n) => ioPure(n + 1)) }`, "type mismatch"},
		{"update is pure", `func bad(r Ref[Int]) IO[Int] { ref.update(r, func(n) => ioPure((n + 1, n))) }`, "type mismatch"},
		{"all operations", `func use() IO[String] {
  do {
    r <- ref.new(1)
    ref.set(r, 2)
    ref.modify(r, func(n) => n + 1)
    previous <- ref.update(r, func(n) => (n + 1, show(n)))
    current <- ref.get(r)
    ioPure(previous + ":" + show(current))
  }
}`, ""},
	} {
		t.Run(tt.name, func(t *testing.T) {
			user, err := parser.Parse("package main\nimport \"std/ref\"\n" + tt.source + "\nfunc main() IO[Unit] { ioPure(unit()) }\n")
			if err != nil {
				t.Fatal(err)
			}
			user.Name = "ref_contract.gt"
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
