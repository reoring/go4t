package types_test

import (
	"strings"
	"testing"

	"github.com/reoring/go4t/ast"
	"github.com/reoring/go4t/types"
)

func TestPolymorphicLawScopesAndRewrite(t *testing.T) {
	const source = `
proof identity(renamed) { if (func(value A) => true)(renamed) { refl } else { refl } }
law identity[A](x A) requires (func(value A) => true)(x) { (func(value A) => value)(x) == x }
law use(x Int, y Bool) { (x, y) == (x, y) }
proof use(x, y) { rewrite identity(x) { rewrite identity(y) { refl } } }
law recursive[A](xs List[A]) { xs == xs }
proof recursive(xs) { match xs { Nil => refl; Cons(x, rest) => rewrite recursive(rest) { refl } } }
`
	for _, named := range []bool{false, true} {
		p, err := checkLawSource(t, source, named)
		if err != nil {
			t.Fatal(err)
		}
		law := p.Laws["identity"]
		if len(law.TParams) != 1 || law.ParamTypes[0] != law.TParams[0] {
			t.Fatalf("law lost rigid parameter identity: %+v", law)
		}
		first := p.Proofs["use"].Decl.Body.(*ast.TRewrite)
		second := first.Body.(*ast.TRewrite)
		for i, rewrite := range []*ast.TRewrite{first, second} {
			ann := types.AnnOf(rewrite.Law)
			want := []types.Type{types.Int, types.Bool}[i]
			if len(ann.Inst) != 1 || ann.Inst[0] != want || ann.Type.(*types.TFunc).Params[0] != want {
				t.Fatalf("rewrite instantiation: %+v, want %s", ann, want)
			}
		}
		self := p.Proofs["recursive"].Decl.Body.(*ast.TMatch).Arms[1].Body.(*ast.TRewrite)
		if types.AnnOf(self.Law).Inst[0] != p.Laws["recursive"].TParams[0] {
			t.Fatal("induction hypothesis must retain the law's rigid type parameter")
		}
	}
}

func TestPolymorphicLawDiagnostics(t *testing.T) {
	for _, tt := range []struct{ name, source, want string }{
		{"constraint", `law bad[A Eq](x A) { x == x }; proof bad(x) { refl }`, "law type parameters cannot have class constraints"},
		{"unknown constraint", `law bad[A Missing](x A) { x == x }; proof bad(x) { refl }`, "law type parameters cannot have class constraints"},
		{"higher kind", `law bad[F[_]](x F[Int]) { x == x }; proof bad(x) { refl }`, "law type parameters cannot be higher-kinded"},
		{"duplicate", `law bad[A, A](x A) { x == x }; proof bad(x) { refl }`, "duplicate type parameter A"},
		{"split", `law bad[A](x A) { x == x }; proof bad(x) { match x { _ => refl } }`, "cannot split A"},
		{"arithmetic", `law bad[A](x A) { x + 0 == x }; proof bad(x) { refl }`, "type mismatch: expected A, got Int"},
		{"equality", `law bad[A](x A) { eq(x, x) == true }; proof bad(x) { refl }`, "Eq[A]"},
		{"ordering", `law bad[A](x A) { compare(x, x) == EQ }; proof bad(x) { refl }`, "Ord[A]"},
		{"show", `law bad[A](x A) { show(x) == show(x) }; proof bad(x) { refl }`, "Show[A]"},
		{"requires", `law bad[A](x A) requires eq(x, x) { x == x }; proof bad(x) { refl }`, "Eq[A]"},
		{"tactic", `law bad[A](x A) { x == x }; proof bad(x) { if eq(x, x) { refl } else { refl } }`, "Eq[A]"},
		{"sample", `law bad[A](x A) { x == x }; proof bad(x) { sample(10) }`, "sample does not support type parameters"},
		{"phantom sample", `law bad[A](x Int) { x == x }; proof bad(x) { sample(10) }`, "sample does not support type parameters"},
		{"rigid self rewrite", `law bad[A](xs List[A]) { xs == xs }; proof bad(xs) { rewrite bad(Cons(1, Nil)) { refl } }`, "type mismatch"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			for _, named := range []bool{false, true} {
				_, err := checkLawSource(t, tt.source, named)
				if err == nil || !strings.Contains(err.Error(), tt.want) {
					t.Fatalf("named=%v: got %v, want %q", named, err, tt.want)
				}
			}
		})
	}
}

func TestPolymorphicLawModuleRewrite(t *testing.T) {
	root := moduleFixture(t, map[string]string{
		"main.gt": `package main
import "rules"
law use(x Int, y Bool) { (x, y) == (x, y) }
proof use(x, y) { rewrite rules.identity(x) { rewrite rules.identity(y) { refl } } }
func main() IO[Unit] { ioPure(unit()) }`,
		"rules/a_proof.gt": `package rules
proof identity(x) { if (func(value A) => true)(x) { refl } else { refl } }`,
		"rules/z_law.gt": `package rules
law identity[A](x A) { (func(value A) => value)(x) == x }`,
	})
	if _, err := checkModuleFixture(t, root); err != nil {
		t.Fatal(err)
	}
}
