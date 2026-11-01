package types_test

import (
	"errors"
	"fmt"
	"reflect"
	"testing"

	"gofx/ast"
	"gofx/modules"
	"gofx/parser"
	"gofx/types"
)

func derivingFiles(t *testing.T, source string) []*ast.Module {
	t.Helper()
	file, err := parser.Parse("package main\nimport \"std/list\"\nimport \"std/prelude\"\n" + source + pass5Main)
	if err != nil {
		t.Fatal(err)
	}
	file.Name = "deriving.gfx"
	mods, err := modules.LoadFiles([]*ast.File{file}, ".")
	if err != nil {
		t.Fatal(err)
	}
	return mods
}

func TestDerivingHigherKinds(t *testing.T) {
	for _, source := range []string{
		`type Tree[A] = Tip | Branch(A, Tree[A], Tree[A]) deriving Functor, Foldable, Traversable, Debug
func value() Option[Tree[String]] { prelude.traverse(Branch(1, Tip, Tip), func(n) => Some(show(n))) }`,
		`type Nested[E, A] = Nested{tag: E, value: (A, List[Option[A]], (String, A)), children: Option[List[Nested[E, A]]]} deriving Functor, Foldable, Traversable
func value() Nested[String, String] { prelude.fmap(Nested("tag", (1, [Some(2)], ("fixed", 3)), None), show) }`,
		`type Box[A] = Box(A) deriving Functor, Foldable, Traversable
type Uses[A] = Uses(List[Box[Option[A]]]) deriving Functor, Foldable, Traversable`,
		`type Phantom[A] = Phantom(Int, func(Int) Int) deriving Functor, Foldable, Traversable`,
		`type Leading[A, B] = Leading(A, B) deriving Functor, Foldable, Traversable
func value() Option[Leading[Int, String]] { prelude.traverse(Leading(1, 2), func(n) => Some(show(n))) }`,
	} {
		t.Run(source, func(t *testing.T) {
			files := derivingFiles(t, source)
			for range 2 {
				p, err := checkFixture(t, files)
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

func TestDerivingEnumShow(t *testing.T) {
	for _, tt := range []struct {
		name, source string
		constructors []string
		params       int
	}{
		{"Currency", `type Currency = JPY | USD | EUR deriving Eq, Ord, Debug, Show
func value() String { show(JPY) + "${USD}" + show(EUR) }`, []string{"JPY", "USD", "EUR"}, 0},
		{"Singleton", `type Singleton = Only deriving prelude.Show
func value() String { show(Only) }`, []string{"Only"}, 0},
		{"Phantom", `type Phantom[A] = First | Last deriving Show
func value() String { let x: Phantom[Int] = First; show(x) }`, []string{"First", "Last"}, 1},
	} {
		t.Run(tt.name, func(t *testing.T) {
			files := derivingFiles(t, tt.source)
			for range 2 {
				p, err := checkFixture(t, files)
				if err != nil {
					t.Fatal(err)
				}
				found := 0
				for _, inst := range p.Instances {
					if inst.Class.Name != "Show" || inst.TypeCon.Name != tt.name {
						continue
					}
					found++
					if len(inst.Params) != tt.params || len(inst.Context) != tt.params {
						t.Fatalf("Show[%s] parameters/context = %d/%d, want %d", tt.name, len(inst.Params), len(inst.Context), tt.params)
					}
					for _, constraint := range inst.Context {
						if constraint.Class != "Show" {
							t.Fatalf("Show[%s] constraint = %#v", tt.name, constraint)
						}
					}
					assertEnumShowNames(t, inst, tt.constructors)
				}
				if found != 1 {
					t.Fatalf("Show[%s] instances = %d, want 1", tt.name, found)
				}
				call := p.Funcs["value"].Decl.Body.Result
				walkExprs(reflect.ValueOf(call), func(e ast.Expr) { assertZonked(t, e) })
			}
		})
	}
}

func TestDerivingEnumShowModuleNames(t *testing.T) {
	root := moduleFixture(t, map[string]string{
		"main.gfx": `package main
import "money"
func main() IO[Unit] { println("${money.JPY}|${money.USD}|${money.EUR}") }`,
		"money/domain.gfx": `package money
type Currency = JPY | USD | EUR deriving Show`,
	})
	p, err := checkModuleFixture(t, root)
	if err != nil {
		t.Fatal(err)
	}
	for _, inst := range p.Instances {
		if inst.Class.Name == "Show" && p.ModuleNames[inst.TypeCon.Name] == "Currency" {
			assertEnumShowNames(t, inst, []string{"JPY", "USD", "EUR"})
			return
		}
	}
	t.Fatal("missing imported Show[Currency] instance")
}

func assertEnumShowNames(t *testing.T, inst *types.Instance, names []string) {
	t.Helper()
	if len(inst.Decl.Methods) != 1 || inst.Decl.Methods[0].Name != "show" {
		t.Fatalf("Show methods = %#v", inst.Decl.Methods)
	}
	method := inst.Decl.Methods[0]
	match, ok := method.Body.Result.(*ast.Match)
	if !ok || len(match.Arms) != len(names) {
		t.Fatalf("Show body = %#v, want %d constructor arms", method.Body.Result, len(names))
	}
	for i, arm := range match.Arms {
		pattern, ok := arm.Pat.(*ast.PCtor)
		if !ok || len(pattern.Args) != 0 {
			t.Fatalf("Show arm %d pattern = %#v", i, arm.Pat)
		}
		literal, ok := arm.Body.(*ast.StringLit)
		if !ok || literal.Value != names[i] {
			t.Fatalf("Show arm %d body = %#v, want %q", i, arm.Body, names[i])
		}
	}
	walkExprs(reflect.ValueOf(inst.Decl), func(e ast.Expr) { assertZonked(t, e) })
}

func TestDerivingNewtypeShow(t *testing.T) {
	for _, source := range []string{
		`type AccountId = AccountId(String) deriving Show
func value() String { show(AccountId("A-1")) }`,
		`type Box[A] = Box(A) deriving Show
func value() String { show(Box(1)) }`,
		`type Box[A] = Box{value: A} deriving Show
func value() String { show(Box("raw")) }`,
		`type Display = Display; instance Show[Display] { show(a) { "display" } }
type Wrapped = Wrapped(Display) deriving Show
func value() String { show(Wrapped(Display)) }`,
	} {
		t.Run(source, func(t *testing.T) {
			files := derivingFiles(t, source)
			for range 2 {
				p, err := checkFixture(t, files)
				if err != nil {
					t.Fatal(err)
				}
				for _, inst := range p.Instances {
					if inst.Class.Name != "Show" {
						continue
					}
					for _, constraint := range inst.Context {
						if constraint.Class != "Show" {
							t.Fatalf("unexpected constraint: %#v", constraint)
						}
					}
				}
			}
		})
	}
}

func TestDerivingDiagnostics(t *testing.T) {
	cases := []struct{ source, want string }{
		{`type T = T(Int, String) deriving Show`, "deriving Show is only supported for enums and single-field newtypes; derive Debug and write instance Show[T] for a display form"},
		{`type T = Empty | T{value: Int} deriving Show`, "deriving Show is only supported for enums and single-field newtypes; derive Debug and write instance Show[T] for a display form"},
		{`type T[A] = Empty | T(A) deriving Show`, "deriving Show is only supported for enums and single-field newtypes; derive Debug and write instance Show[T] for a display form"},
		{`type Hidden = Hidden; type T = T(Hidden) deriving Show`, "no instance Show[Hidden]; hint: add 'deriving Show' or write 'instance Show[Hidden]'"},
		{`type T[A] = T(A) deriving Show; func value(x T[Unit]) String { show(x) }`, "no instance Show[Unit] (required by instance Show[T[A]])"},
		{`type T = T(func(Int) Int) deriving Show`, "no instance Show[func(Int) Int]"},
		{`type T = T deriving Show, Show`, "duplicate deriving class Show for T"},
		{`type T = T deriving Show; instance Show[T] { show(a) { "T" } }`, "duplicate instance Show[T]"},
		{`type Phantom[A] = Phantom deriving Show; func value(x Phantom[Unit]) String { show(x) }`, "no instance Show[Unit] (required by instance Show[Phantom[A]])"},
		{`type T = T deriving Unknown`, "cannot derive Unknown for T: unknown class"},
		{`type T[A] = T(A) deriving Debug, Debug`, "duplicate deriving class Debug for T"},
		{`type T[A] = T(A) deriving Functor, Functor`, "duplicate deriving class Functor for T"},
		{`type T = T deriving Debug; instance Debug[T] { debug(a) { "T" } }`, "duplicate instance Debug[T]"},
		{`type Hidden = Hidden; type T = T(Hidden) deriving Debug`, "no instance Debug[Hidden]; hint: add 'deriving Debug' or write 'instance Debug[Hidden]'"},
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
			_, err := checkFixture(t, derivingFiles(t, tt.source))
			var typed *types.Error
			if !errors.As(err, &typed) || typed.Msg != tt.want || typed.File != "deriving.gfx" || typed.Pos.Line == 0 || typed.Pos.Col == 0 {
				t.Fatalf("Check = %v, want positioned %q", err, tt.want)
			}
		})
	}
}

func TestTraversableMethodMonadDictionary(t *testing.T) {
	p, err := checkFixture(t, derivingFiles(t, `type Tree[A] = Leaf(A) | Fork(Tree[A], Tree[A]) deriving Functor, Traversable
func value() Option[Tree[Int]] { prelude.traverse(Leaf(1), func(a) => Some(a)) }`))
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
