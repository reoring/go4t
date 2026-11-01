package codegen

import (
	"strings"
	"testing"

	"gofx/ast"
	"gofx/parser"
	"gofx/types"
)

func TestLocalRecursionCompiles(t *testing.T) {
	const support = `package main
extern func main() IO[Unit] = "gofx/rt".UnitValue
class Show[A] { show(x A) String }
extern func showInt(x Int) String = "gofx/rt".ShowInt
instance Show[Int] { show(x) { showInt(x) } }
instance Show[Bool] { show(x) { if x { "true" } else { "false" } } }
class Functor[F[_]] { fmap[A, B](fa F[A], f func(A) B) F[B] }
class Monad[M[_]] : Functor[M] { pure[A](a A) M[A]; bind[A, B](ma M[A], f func(A) M[B]) M[B] }
type Option[A] = None | Some(A)
instance Functor[Option] { fmap(fa, f) { match fa { None => None; Some(x) => Some(f(x)) } } }
instance Monad[Option] {
  pure(a) { Some(a) }
  bind(ma, f) { match ma { None => None; Some(x) => f(x) } }
}
`
	for _, tc := range []struct {
		name, source string
		fragments    []string
	}{
		{"recursive polymorphic let", `func f() (Int, Bool) { let loop = func(done, x) => if done { x } else { loop(true, x) }; (loop(false, 7), loop(false, true)) }`, []string{"var loop_ func(any, any) any", "loop_ = func("}},
		{"constrained recursive let", `func f() String { let loop = func(done, x) => if done { show(x) } else { loop(true, x) }; loop(false, 7) + loop(false, true) }`, []string{"var loop_ func(any, any) any", "Show_Int{}", "Show_Bool{}"}},
		{"recursive let captures shadowed scope", `func f() Int { let x = 7; let loop = func(done) => if done { x } else { loop(true) }; let saved = loop; let x = 9; let loop = func(x) => x; saved(false) + loop(x) }`, []string{"var loop_ func(any) any"}},
		{"mutual group and unused function", `func f() Bool { let base = true; func first(done Bool) Bool { if done { base } else { second(true) } }; func second(done Bool) Bool { first(done) }; func unused() Int { 7 }; first(false) }`, []string{"var first_ func(any) any", "var second_ func(any) any", "var unused_ func() any", "_ = unused_"}},
		{"group captures enclosing dictionary", `func f[A Show](x A) String { func first(done Bool) String { if done { show(x) } else { second() } }; func second() String { first(true) }; first(false) }`, []string{"var first_ func(any) any", "(dict_A).Show(x_)"}},
		{"do local group captures bind", `func f() Option[Int] { do { x <- Some(7); func first(done Bool) Int { if done { x } else { second() } }; func second() Int { first(true) }; Some(first(false)) } }`, []string{"var x_ int64", "var first_ func(any) any", "var second_ func() any"}},
		{"do recursive let", `func f() Option[String] { do { let loop = func(done, x) => if done { show(x) } else { loop(true, x) }; Some(loop(false, 7) + loop(false, true)) } }`, []string{"var loop_ func(any, any) any", "Show_Int{}", "Show_Bool{}"}},
		{"shadowed self is nonrecursive", `func f() Int { let loop = func(loop) => loop; loop(7) }`, []string{"var loop_ any = func("}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			file, err := parser.Parse(support + tc.source)
			if err != nil {
				t.Fatal(err)
			}
			program, err := types.Check([]*ast.File{file})
			if err != nil {
				t.Fatal(err)
			}
			out := assertSource(t, program, tc.fragments...)
			if tc.name == "mutual group and unused function" && strings.Index(out, "var second_") > strings.Index(out, "first_ = func(") {
				t.Fatal("mutually recursive variables must all be declared before assignments")
			}
		})
	}
}
