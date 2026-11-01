package types_test

import (
	"errors"
	"strings"
	"testing"

	"gofx/ast"
	"gofx/types"
)

func TestSourceAliasDiagnostics(t *testing.T) {
	for _, tt := range []struct{ name, source, want string }{
		{"return", `type Currency = USD; alias Rates = Map[Currency, Float]; func rates() Rates { 1 }`, "type mismatch: expected Rates, got Int"},
		{"argument", `alias Name = String; func use(n Name) Int { 1 }; func bad() Int { use(2) }`, "type mismatch: expected Name, got Int"},
		{"actual", `alias Name = String; func bad(n Name) Int { n }`, "type mismatch: expected Int, got Name"},
		{"nested", `alias Name = String; func bad() List[Name] { [1] }`, "type mismatch: expected List[Name], got List[Int]"},
		{"generic", `alias Pair[A] = (A, A); func use[A](p Pair[A], x A) A { x }; func bad() Int { use((1, 2), true) }`, "type mismatch: expected Int, got Bool"},
		{"generic alias", `alias Pair[A] = (A, A); func use[A](x A, p Pair[A]) A { x }; func bad() Int { use(1, (true, false)) }`, "type mismatch: expected Pair[Int], got Tuple2[Bool, Bool]"},
		{"annotation", `alias Name = String; func bad() Int { let n: Name = "ok"; n }`, "type mismatch: expected Int, got Name"},
		{"unrelated literal", `alias Count = Int; func bad() String { 1 }`, "type mismatch: expected String, got Int"},
		{"rigid parameter", `alias Count = Int; func bad[Count](n Count) Int { n }`, "type mismatch: expected Int, got Count"},
		{"inferred scalar", `alias Count = Int; func id[A](x A) A { x }; func bad(n Count) String { id(n) }`, "type mismatch: expected String, got Int"},
		{"inferred nested", `alias Count = Int; func id[A](x A) A { x }; func bad(n List[Count]) String { id(n) }`, "type mismatch: expected String, got List[Int]"},
		{"alias function", `alias Handler = func(Int) Int; func use(f Handler) Int { f(1) }; func bad() Int { use(func(x) => true) }`, "type mismatch: expected Handler, got func(Int) Bool"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			_, err := types.Check([]*ast.Module{{Files: parseFiles(t, tt.source+pass5Main)}})
			var checked *types.Error
			if !errors.As(err, &checked) || checked.Msg != tt.want {
				t.Fatalf("Check = %v, want %q", err, tt.want)
			}
		})
	}
}

func TestTypePrintingElisionBoundary(t *testing.T) {
	for _, length := range []int{159, 160, 161} {
		// Three applications put the leaf at depth 3 (the root is depth 0).
		leaf := strings.Repeat("界", length-9)
		con := &types.TCon{Name: "T"}
		typ := types.App(con, types.App(con, types.App(con, &types.TCon{Name: leaf})))
		full := "T[T[T[" + leaf + "]]]"
		want := full
		if length > 160 {
			want = "T[T[T[…]]]"
		}
		if got := types.TypeString(typ); got != want {
			t.Errorf("length %d: got %q, want %q", length, got, want)
		}
		if got := types.TypeStringFull(typ); got != full {
			t.Errorf("full length %d: got %q", length, got)
		}
	}
	shallow := &types.TCon{Name: strings.Repeat("X", 161)}
	if got := types.TypeString(shallow); got != shallow.Name {
		t.Fatalf("shallow name was truncated: %q", got)
	}
}

func TestAliasRepresentationIsTransparent(t *testing.T) {
	p, err := types.Check([]*ast.Module{{Files: parseFiles(t, `
alias Number = Int
alias Pair[A] = (A, A)
alias Handler = func(Number) Number
alias Names = List[String]
alias Maybe[A] = Option[A]
class Label[A] { label(x A) String }
instance Label[List[A]] { label(xs) { match xs { Nil => "empty"; Cons(_, _) => "names" } } }
func apply(f Handler, n Number) Number { f(n) }
func first(p Pair[Number]) Number { p.0 }
func plus(n Number) Number { n + 1 }
func display(xs Names) String { label(xs) }
func present(x Maybe[Number]) Number { match x { Some(n) => n; None => 0 } }
`+pass5Main)}})
	if err != nil {
		t.Fatal(err)
	}
	if got := types.TypeString(p.Funcs["apply"].Scheme.Type); got != "func(Handler, Number) Number" {
		t.Fatal(got)
	}
}
