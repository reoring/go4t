package types_test

import (
	"errors"
	"fmt"
	"reflect"
	"testing"

	"gofx/ast"
	"gofx/parser"
	"gofx/prelude"
	"gofx/types"
)

func derivingFiles(t *testing.T, source string) []*ast.File {
	t.Helper()
	var files []*ast.File
	for i, source := range []string{prelude.Source, "package main\n" + source + pass5Main} {
		file, err := parser.Parse(source)
		if err != nil {
			t.Fatal(err)
		}
		file.Name = []string{"prelude.gfx", "deriving.gfx"}[i]
		files = append(files, file)
	}
	return files
}

func TestDerivingHigherKinds(t *testing.T) {
	for _, source := range []string{
		`type Tree[A] = Tip | Branch(A, Tree[A], Tree[A]) deriving Functor, Foldable, Traversable, Debug
func value() Option[Tree[String]] { traverse(Branch(1, Tip, Tip), func(n) => Some(show(n))) }`,
		`type Nested[E, A] = Nested{tag: E, value: (A, List[Option[A]], (String, A)), children: Option[List[Nested[E, A]]]} deriving Functor, Foldable, Traversable
func value() Nested[String, String] { fmap(Nested("tag", (1, [Some(2)], ("fixed", 3)), None), show) }`,
		`type Box[A] = Box(A) deriving Functor, Foldable, Traversable
type Uses[A] = Uses(List[Box[Option[A]]]) deriving Functor, Foldable, Traversable`,
		`type Phantom[A] = Phantom(Int, func(Int) Int) deriving Functor, Foldable, Traversable`,
		`type Leading[A, B] = Leading(A, B) deriving Functor, Foldable, Traversable
func value() Option[Leading[Int, String]] { traverse(Leading(1, 2), func(n) => Some(show(n))) }`,
	} {
		t.Run(source, func(t *testing.T) {
			files := derivingFiles(t, source)
			for range 2 {
				p, err := types.Check(files)
				if err != nil {
					t.Fatal(err)
				}
				for _, inst := range p.Instances {
					if inst.Decl.Class != "Functor" && inst.Decl.Class != "Foldable" && inst.Decl.Class != "Traversable" {
						continue
					}
					if len(inst.Context) != 0 {
						t.Fatalf("higher-kinded instance has unnecessary constraints: %#v", inst)
					}
					walkExprs(reflect.ValueOf(inst.Decl), func(e ast.Expr) { assertZonked(t, e) })
				}
			}
		})
	}
}

func TestDerivingDiagnostics(t *testing.T) {
	cases := []struct{ source, want string }{
		{`type T = T deriving Show`, "deriving Show is not supported; derive Debug and write instance Show if a display form is needed"},
		{`type T = T deriving Unknown`, "cannot derive Unknown for T: unknown class"},
		{`type T[A] = T(A) deriving Debug, Debug`, "duplicate deriving class Debug for T"},
		{`type T[A] = T(A) deriving Functor, Functor`, "duplicate deriving class Functor for T"},
		{`type T = T deriving Debug; instance Debug[T] { debug(a) { "T" } }`, "duplicate instance Debug[T]"},
		{`type Hidden = Hidden; type T = T(Hidden) deriving Debug`, "no instance Debug[Hidden]"},
		{`type T = T(func(Int) Int) deriving Debug`, "no instance Debug[func(Int) Int]"},
		{`type T[A] = T(A) deriving Traversable`, "no instance Functor[T] (required superclass)"},
	}
	for _, class := range []string{"Functor", "Foldable", "Traversable"} {
		cases = append(cases,
			struct{ source, want string }{fmt.Sprintf(`type T = T deriving %s`, class), fmt.Sprintf("cannot derive %s for T: requires a last type parameter", class)},
			struct{ source, want string }{fmt.Sprintf(`type T[A] = T{run: func(Int) A} deriving %s`, class), fmt.Sprintf("cannot derive %s for T: constructor T field run: type parameter A occurs in a function type", class)},
			struct{ source, want string }{fmt.Sprintf(`type T[A] = T(List[func(A) Int]) deriving %s`, class), fmt.Sprintf("cannot derive %s for T: constructor T field 1: type parameter A occurs in a function type", class)},
			struct{ source, want string }{fmt.Sprintf(`type Pair[E, A] = Pair(E, A); type T[A] = T(Option[Pair[A, Int]]) deriving %s`, class), fmt.Sprintf("cannot derive %s for T: constructor T field 1: type parameter A occurs as a non-last argument of Pair", class)},
		)
	}
	for _, tt := range cases {
		t.Run(tt.source, func(t *testing.T) {
			_, err := types.Check(derivingFiles(t, tt.source))
			var typed *types.Error
			if !errors.As(err, &typed) || typed.Msg != tt.want || typed.File != "deriving.gfx" || typed.Pos.Line == 0 || typed.Pos.Col == 0 {
				t.Fatalf("Check = %v, want positioned %q", err, tt.want)
			}
		})
	}
}

func TestTraversableMethodMonadDictionary(t *testing.T) {
	p, err := types.Check(derivingFiles(t, `type Tree[A] = Leaf(A) | Fork(Tree[A], Tree[A]) deriving Functor, Traversable
func value() Option[Tree[Int]] { traverse(Leaf(1), func(a) => Some(a)) }`))
	if err != nil {
		t.Fatal(err)
	}
	call := p.Funcs["value"].Decl.Body.Result.(*ast.Call)
	dicts := types.AnnOf(call.Fn).Dicts
	if len(dicts) != 2 {
		t.Fatalf("traverse dictionaries = %#v", dicts)
	}
	assertInstanceDict(t, dicts[0], "Traversable", "Tree")
	assertInstanceDict(t, dicts[1], "Monad", "Option")
	for _, inst := range p.Instances {
		if inst.Class.Name != "Traversable" {
			continue
		}
		uses := 0
		walkExprs(reflect.ValueOf(inst.Decl), func(e ast.Expr) {
			id, ok := e.(*ast.Ident)
			if !ok || (id.Name != "pure" && id.Name != "bind") {
				return
			}
			uses++
			assertParamDict(t, types.AnnOf(id).Dicts[0], "Monad", "M")
		})
		if uses == 0 {
			t.Fatalf("Traversable[%s] did not use its method-level Monad dictionary", inst.TypeCon.Name)
		}
	}
}
