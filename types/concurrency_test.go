package types_test

import (
	"strings"
	"testing"

	"go4t/ast"
	"go4t/types"
)

func TestFiberType(t *testing.T) {
	if got := types.KindString(types.Fiber.Kind); got != "(*) -> *" {
		t.Fatalf("Fiber kind = %q, want (*) -> *", got)
	}
	const main = "\nfunc main() IO[Unit] { ioPure(unit()) }\n"
	for _, tt := range []struct {
		name, source, wantError string
	}{
		{"opaque value", `func keep[A](f Fiber[A]) Fiber[A] { f }`, ""},
		{"higher kind argument", `extern func keep[F[_], A](fa F[A]) F[A] = "example".Keep
func use(f Fiber[Int]) Fiber[Int] { keep(f) }`, ""},
		{"bare constructor", `func bad(f Fiber) Int { 0 }`, "kind mismatch"},
		{"too many arguments", `func bad(f Fiber[Int, Bool]) Int { 0 }`, "kind mismatch applying Fiber to 2 arguments"},
		{"wrong element", `func bad(f Fiber[Int]) Fiber[String] { f }`, "type mismatch: expected Fiber[String], got Fiber[Int]"},
		{"reserved builtin", `type Fiber[A] = Wrapped(A)`, "duplicate declaration Fiber"},
		{"no show", `func bad(f Fiber[Int]) String { show(f) }`, "no instance Show[Fiber[Int]]"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			_, err := checkFixture(t, []*ast.Module{{Files: parseFiles(t, tt.source+main)}})
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
