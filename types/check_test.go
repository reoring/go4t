package types_test

import (
	"errors"
	"reflect"
	"strings"
	"testing"

	"gofx/ast"
	"gofx/parser"
	"gofx/types"
)

const testPrelude = `package main
class Functor[F[_]] {
  fmap[A, B](fa F[A], f func(A) B) F[B]
}
class Monad[M[_]] : Functor[M] {
  pure[A](a A) M[A]
  bind[A, B](ma M[A], f func(A) M[B]) M[B]
}
class Show[A] { show(a A) String }
extern func showInt(a Int) String = "gofx/rt".ShowInt
instance Show[Int] { show(a) { showInt(a) } }
type List[A] = Nil | Cons(A, List[A])
type Option[A] = None | Some(A)
instance Functor[Option] {
  fmap(fa, f) { match fa { None => None; Some(a) => Some(f(a)) } }
}
instance Monad[Option] {
  pure(a) { Some(a) }
  bind(ma, f) { match ma { None => None; Some(a) => f(a) } }
}
extern func ioPure[A](a A) IO[A] = "gofx/rt".Pure
extern func ioBind[A, B](ma IO[A], f func(A) IO[B]) IO[B] = "gofx/rt".Bind
extern func unit() Unit = "gofx/rt".UnitValue
instance Functor[IO] {
  fmap(fa, f) { ioBind(fa, func(a) => ioPure(f(a))) }
}
instance Monad[IO] {
  pure(a) { ioPure(a) }
  bind(ma, f) { ioBind(ma, f) }
}
` + pass9Prelude

func parseFiles(t *testing.T, source string) []*ast.File {
	t.Helper()
	var files []*ast.File
	for _, src := range []string{testPrelude, "package main\n" + source} {
		f, err := parser.Parse(src)
		if err != nil {
			t.Fatalf("parse: %v", err)
		}
		files = append(files, f)
	}
	return files
}

func TestCheck(t *testing.T) {
	const main = "\nfunc main() IO[Unit] { ioPure(unit()) }\n"
	tests := []struct {
		name, source, wantError string
	}{
		{"HKT Option and IO", `
func twice[F[_] Functor, A](fa F[A], f func(A) A) F[A] { fmap(fmap(fa, f), f) }
func option() Option[Int] { twice(Some(1), func(a) => a + 1) }
func effect() IO[Int] { twice(ioPure(1), func(a) => a + 1) }
` + main, ""},
		{"superclass", `func mapped[M[_] Monad, A](ma M[A]) M[A] { fmap(ma, func(a) => a) }` + main, ""},
		{"do Option", `func option() Option[Int] { do { a <- Some(1); Some(2); let b = a + 2; Some(b) } }` + main, ""},
		{"do IO", `func main() IO[Unit] { do { a <- ioPure(1); ioPure(a); let b = a + 1; ioPure(unit()) } }`, ""},
		{"missing instance", `type Box[A] = Boxed(A)
func bad() Box[Int] { fmap(Boxed(1), func(x) => x) }` + main, "no instance Functor[Box]"},
		{"kind mismatch", `instance Functor[Int] { fmap(fa, f) { fa } }` + main, "kind mismatch"},
		{"non exhaustive", `func bad(x Option[Int]) Int { match x { Some(a) => a } }` + main, "non-exhaustive match"},
		{"wrong main", `func main() Int { 1 }`, "main must have type func() IO[Unit]"},
		{"missing main", `func other() Int { 1 }`, "main must have type func() IO[Unit]"},
		{"duplicate instance", `instance Functor[Option] { fmap(fa, f) { fa } }` + main, "duplicate instance Functor[Option]"},
		{"rigid type", `func bad[A](x A) A { 1 }` + main, "type mismatch: expected A, got Int"},
		{"occurs check", `func bad() Unit { let f = func(x) => x(x); unit() }` + main, "occurs check"},
		{"polymorphic let", `func okay() Unit { let id = func(x) => x; id(1); id(true); unit() }` + main, ""},
		{"call arity", `func bad() Unit { ioPure(); unit() }` + main, "arity mismatch"},
		{"mixed monads", `func bad() Option[Int] { do { a <- Some(1); ioPure(a) } }` + main, "type mismatch: expected Option["},
		{"ambiguous class", `func bad() Unit { pure(1); unit() }` + main, "unresolved class constraint"},
		{"default unconstrained", `func okay() Unit { None; let f = func(x) => x; unit() }` + main, ""},
		{"primitive comparison", `func okay() Bool { let same = func(x) => x == x; same(1) }` + main, ""},
		{"polymorphic comparison", `func bad[A](x A) Bool { x == x }` + main, "no instance Eq[A] (type parameter A has no class constraints)"},
		{"data comparison missing instance", `func bad() Bool { Some(1) == Some(1) }` + main, "no instance Eq[Option[Int]]"},
		{"bool ordering", `func okay() Bool { false < true }` + main, ""},
		{"operators", `func okay(x Int, y String, z Bool) Bool { let n = -(x + 2) * 3 / 2 - 1; if !z && n >= 0 { y < "z" || n != 2 } else { z == false } }` + main, ""},
		{"literal catchall", `func okay(x Int) Bool { match x { 1 => true; _ => false } }` + main, ""},
		{"literal incomplete", `func bad(x Bool) Int { match x { true => 1; false => 0 } }` + main, "non-exhaustive match"},
		{"nested incomplete", `func bad(x Option[Option[Int]]) Int { match x { None => 0; Some(Some(a)) => a } }` + main, "non-exhaustive match"},
		{"nested complete", `func okay(x Option[Option[Int]]) Int { match x { None => 0; Some(None) => 1; Some(Some(a)) => a } }` + main, ""},
		{"duplicate pattern binding", `type Pair[A] = PairOf(A, A)
func bad(x Pair[Int]) Int { match x { PairOf(a, a) => a } }` + main, "duplicate pattern variable a"},
		{"unknown superclass", `class Bad[F[_]] : Missing[F] { bad[A](x F[A]) F[A] }` + main, "unknown superclass Missing"},
		{"superclass cycle", `class C[A] : D[A] {}; class D[A] : C[A] {}` + main, "superclass cycle"},
		{"superclass kind", `class Bad[A] : Functor[A] {}` + main, "kind mismatch"},
		{"missing superclass instance", `type Box[A] = Boxed(A)
instance Monad[Box] { pure(a) { Boxed(a) }; bind(ma, f) { match ma { Boxed(a) => f(a) } } }` + main, "no instance Functor[Box]"},
		{"missing method", `class C[A] { c(x A) A }; instance C[Int] {}` + main, "missing method c"},
		{"extra method", `class C[A] {}; instance C[Int] { extra(x) { x } }` + main, "unknown method extra"},
		{"wrong method arity", `class C[A] { c(x A) A }; instance C[Int] { c(x, y) { x } }` + main, "arity mismatch"},
		{"wrong method body", `class C[A] { c(x A) A }; instance C[Int] { c(x) { true } }` + main, "type mismatch: expected Int, got Bool"},
		{"duplicate declaration", `func ioPure(x Int) Int { x }` + main, "duplicate declaration ioPure"},
		{"recursive signatures", `func left(x Int) Int { right(x) }; func right(x Int) Int { left(x) }` + main, ""},
		{"kind in field", `type Bad = BadValue(Option)` + main, "kind mismatch"},
		{"recursive wildcard incomplete", `type Chain = Link(Chain) | End
type Pair = PairOf(Chain, Option[Int])
func bad(x Pair) Int { match x { PairOf(_, None) => 0 } }` + main, "non-exhaustive match"},
		{"recursive match complete", `func size[A](xs List[A]) Int { match xs { Cons(_, rest) => 1 + size(rest); Nil => 0 } }` + main, ""},
		{"HKT binary arity", `type Pair[A, B] = PairOf(A, B)
class Bifunctor[F[_, _]] { bimap[A, B, C, D](pair F[A, B], f func(A) C, g func(B) D) F[C, D] }
instance Bifunctor[Pair] { bimap(pair, f, g) { match pair { PairOf(a, b) => PairOf(f(a), g(b)) } } }
func both() Pair[Int, Bool] { bimap(PairOf(1, true), func(x) => x + 1, func(x) => !x) }` + main, ""},
		{"HKT data arity mismatch", `type Pair[A, B] = PairOf(A, B)
instance Functor[Pair] { fmap(pair, f) { pair } }` + main, "kind mismatch"},
		{"unresolved higher kind", `extern func phantom[F[_]]() Unit = "example".Phantom
func bad() Unit { phantom() }` + main, "cannot infer type constructor of kind"},
		{"ordinary class constraint", `func display[A Show](a A) String { show(a) }
func use() String { display(1) }` + main, ""},
		{"constraint without binding", `func bad[F[_], A](fa F[A]) F[A] { fmap(fa, func(x) => x) }` + main, "no instance Functor[F]"},
		{"typed lambda", `func okay[A](a A) A { (func(x A) => x)(a) }` + main, ""},
		{"nullary constructor value", `func okay() Option[Int] { None }` + main, ""},
		{"nullary constructor not callable", `func bad() Option[Int] { None() }` + main, "type mismatch: expected func()"},
		{"wrong pattern arity", `func bad(x Option[Int]) Int { match x { Some(a, b) => a; None => 0 } }` + main, "arity mismatch for pattern"},
		{"wrong pattern type", `func bad(x Int) Int { match x { Some(a) => a; _ => 0 } }` + main, "type mismatch: expected Int, got Option["},
		{"same named rigid parameters", `func wrong[A, B](a A) B { a }` + main, "type mismatch: expected B, got A"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			files := parseFiles(t, tt.source)
			p, err := types.Check(files)
			if tt.wantError != "" {
				var typed *types.Error
				if !errors.As(err, &typed) || !strings.Contains(typed.Msg, tt.wantError) {
					t.Fatalf("Check error = %v; want *types.Error containing %q", err, tt.wantError)
				}
				if typed.Pos.Line == 0 || typed.Pos.Col == 0 {
					t.Fatalf("error has no source position: %v", err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if p.Main != p.Funcs["main"] || len(p.Files) != len(files) {
				t.Fatal("program lost main or input files")
			}
			walkExprs(reflect.ValueOf(files), func(e ast.Expr) { assertZonked(t, e) })
		})
	}
}

func TestAdditionAnnotations(t *testing.T) {
	tests := []struct {
		name, source string
		want         types.Type
	}{
		{"literals", `func value() String { "hello, " + "world" }`, types.String},
		{"parameters", `func value(a String, b String) String { a + b }`, types.String},
		{"inferred left operand", `func value() String { (func(x) => x + "!")("hello") }`, types.String},
		{"inferred right operand", `func value() String { (func(x) => "hello" + x)("!") }`, types.String},
		{"later argument", `func value() String { let concat = func(x) => x + x; concat("hello") }`, types.String},
		{"return context", `func value() func(String) String { func(x) => x + x }`, types.String},
		{"integer addition", `func value() Int { 1 + 2 }`, types.Int},
		{"integer default", `func value() Unit { let add = func(x) => x + x; unit() }`, types.Int},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			files := parseFiles(t, tt.source+"\nfunc main() IO[Unit] { ioPure(unit()) }\n")
			p, err := types.Check(files)
			if err != nil {
				t.Fatal(err)
			}
			additions := 0
			walkExprs(reflect.ValueOf(p.Funcs["value"].Decl.Body), func(e ast.Expr) {
				assertZonked(t, e)
				if binary, ok := e.(*ast.Binary); ok && binary.Op == "+" {
					additions++
					for _, operand := range []ast.Expr{binary, binary.L, binary.R} {
						if got := types.AnnOf(operand).Type; got != tt.want {
							t.Errorf("%T annotation = %s, want %s", operand, types.TypeString(got), types.TypeString(tt.want))
						}
					}
				}
			})
			if additions != 1 {
				t.Fatalf("checked %d additions, want 1", additions)
			}
		})
	}
}

func TestDiagnosticMessages(t *testing.T) {
	tests := []struct {
		name, source, want string
	}{
		{"mixed addition", `func bad() Int { 1 + "two" }`, "type mismatch: expected Int, got String"},
		{"reversed mixed addition", `func bad() String { "one" + 2 }`, "type mismatch: expected String, got Int"},
		{"return type", `func bad() Int { "wrong" }`, "type mismatch: expected Int, got String"},
		{"argument type", `func takes(n Int) Int { n }; func bad() Int { takes("wrong") }`, "type mismatch: expected Int, got String"},
		{"if condition", `func bad() Int { if 1 { 2 } else { 3 } }`, "type mismatch: expected Bool, got Int"},
		{"if branches", `func bad() Int { if true { 1 } else { false } }`, "type mismatch: expected Int, got Bool"},
		{"match arms", `func bad(x Option[Int]) Int { match x { Some(n) => n; None => false } }`, "type mismatch: expected Int, got Bool"},
		{"literal pattern", `func bad(x Int) Int { match x { true => 1; _ => 0 } }`, "type mismatch: expected Int, got Bool"},
		{"constructor pattern", `type Token = Tok; func bad(x Int) Int { match x { Tok => 1; _ => 0 } }`, "type mismatch: expected Int, got Token"},
		{"unary operand", `func bad() Int { -true }`, "type mismatch: expected Int, got Bool"},
		{"boolean operand", `func bad() Bool { true && 1 }`, "type mismatch: expected Bool, got Int"},
		{"comparison operand", `func bad() Bool { 1 == "one" }`, "type mismatch: expected Int, got String"},
		{"nested return type", `func bad() func(Int) Int { func(x Int) => true }`, "type mismatch: expected func(Int) Int, got func(Int) Bool"},
		{"nested argument type", `func takes(f func(Int) Int) Int { f(1) }; func bad() Int { takes(func(x Int) => false) }`, "type mismatch: expected func(Int) Int, got func(Int) Bool"},
		{"nested parameter type", `func takes(f func(Int) Int) Int { f(1) }; func bad() Int { takes(func(x String) => 1) }`, "type mismatch: expected func(Int) Int, got func(String) Int"},
		{"nested application type", `func takes(x Option[Int]) Int { 1 }; func bad() Int { takes(Some(true)) }`, "type mismatch: expected Option[Int], got Option[Bool]"},
		{"function type arity", `func bad() func(Int) Int { func(x Int, y Int) => x }`, "type mismatch: expected func(Int) Int, got func(Int, Int) Int"},
		{"constrained parameter", `func bad[F[_] Functor, A](fa F[A]) F[A] { do { fa } }`, "no instance Monad[F] (type parameter F is constrained only by Functor)"},
		{"unconstrained parameter", `func bad[F[_], A](fa F[A]) F[A] { do { fa } }`, "no instance Monad[F] (type parameter F has no class constraints)"},
		{"method constraint", `func bad[F[_] Functor, A](fa F[A]) F[A] { bind(fa, func(a) => fa) }`, "no instance Monad[F] (type parameter F is constrained only by Functor)"},
		{"unconstrained method", `func bad[F[_], A](fa F[A]) F[A] { fmap(fa, func(a) => a) }`, "no instance Functor[F] (type parameter F has no class constraints)"},
		{"ordinary parameter", `class Render[A] { render(a A) String }; func bad[A](a A) String { render(a) }`, "no instance Render[A] (type parameter A has no class constraints)"},
		{"concrete missing instance", `type Box[A] = Boxed(A); func bad() Box[Int] { fmap(Boxed(1), func(x) => x) }`, "no instance Functor[Box]"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			files := parseFiles(t, tt.source+"\nfunc main() IO[Unit] { ioPure(unit()) }\n")
			p, err := types.Check(files)
			var typed *types.Error
			if p != nil || !errors.As(err, &typed) || typed.Msg != tt.want || typed.Pos.Line == 0 || typed.Pos.Col == 0 {
				t.Fatalf("Check = (%v, %v), want positioned error %q", p, err, tt.want)
			}
		})
	}
}

func TestDoMismatchContext(t *testing.T) {
	for _, body := range []string{"do { ioPure(1); x <- 2; ioPure(x) }", "do { ioPure(1); 2; ioPure(3) }", "do { ioPure(1); 2 }"} {
		t.Run(body, func(t *testing.T) {
			files := parseFiles(t, "func bad() IO[Int] { "+body+" }\nfunc main() IO[Unit] { ioPure(unit()) }\n")
			p, err := types.Check(files)
			var typed *types.Error
			// The item type is not yet inferred, so its meta ID is intentionally
			// unspecified; the expected outer IO application must be retained.
			if p != nil || !errors.As(err, &typed) || !strings.HasPrefix(typed.Msg, "type mismatch: expected IO[?") || !strings.HasSuffix(typed.Msg, "], got Int") {
				t.Fatalf("Check = (%v, %v), want expected IO[?], got Int", p, err)
			}
		})
	}
}

func TestReservedIdentifiers(t *testing.T) {
	declarations := map[string]string{
		"function":                `func RESERVED() Int { 1 }`,
		"extern":                  `extern func RESERVED() Unit = "example".Value`,
		"class":                   `class RESERVED[A] {}`,
		"class parameter":         `class C[RESERVED] {}`,
		"method":                  `class C[A] { RESERVED(a A) A }`,
		"method type parameter":   `class C[A] { c[RESERVED](a A) A }`,
		"method parameter":        `class C[A] { c(RESERVED A) A }`,
		"type":                    `type RESERVED = Value`,
		"data parameter":          `type Data[RESERVED] = Value`,
		"constructor":             `type Data = RESERVED`,
		"function type parameter": `func f[RESERVED](x Int) Int { x }`,
		"function parameter":      `func f(RESERVED Int) Int { RESERVED }`,
		"instance method":         `class C[A] {}; instance C[Int] { RESERVED() { 1 } }`,
		"instance parameter":      `class C[A] { c(x A) A }; instance C[Int] { c(RESERVED) { RESERVED } }`,
		"instance type parameter": `instance Show[Option[RESERVED]] { show(o) { "x" } }`,
		"lambda parameter":        `func f() Unit { let f = func(RESERVED) => RESERVED; unit() }`,
		"let binding":             `func f() Int { let RESERVED = 1; RESERVED }`,
		"pattern binding":         `func f(x Int) Int { match x { RESERVED => 1 } }`,
		"do binding":              `func f() IO[Int] { do { RESERVED <- ioPure(1); ioPure(RESERVED) } }`,
		"do let":                  `func f() IO[Int] { do { let RESERVED = 1; ioPure(RESERVED) } }`,
	}
	for _, prefix := range []string{"dict_", "ext_"} {
		for kind, source := range declarations {
			t.Run(prefix+kind, func(t *testing.T) {
				name := prefix + "reserved"
				source = strings.ReplaceAll(source, "RESERVED", name)
				files := parseFiles(t, source+"\nfunc main() IO[Unit] { ioPure(unit()) }\n")
				p, err := types.Check(files)
				var typed *types.Error
				want := "identifier \"" + name + "\" uses reserved prefix"
				if p != nil || !errors.As(err, &typed) || typed.Msg != want || typed.Pos.Line == 0 || typed.Pos.Col == 0 {
					t.Fatalf("Check = (%v, %v), want positioned error %q", p, err, want)
				}
			})
		}
	}
}

func TestDictionaryAnnotations(t *testing.T) {
	files := parseFiles(t, `
func twice[F[_] Functor, A](fa F[A], f func(A) A) F[A] { fmap(fmap(fa, f), f) }
func mapped[M[_] Monad, A](ma M[A]) M[A] { fmap(ma, func(a) => a) }
func sequenced[M[_] Monad, A](ma M[A]) M[A] { do { a <- ma; pure(a) } }
func option() Option[Int] { twice(Some(1), func(a) => a) }
func effect() IO[Int] { twice(ioPure(1), func(a) => a) }
func main() IO[Unit] { do { ioPure(unit()) } }
`)
	p, err := types.Check(files)
	if err != nil {
		t.Fatal(err)
	}
	fmap := p.Funcs["mapped"].Decl.Body.Result.(*ast.Call).Fn
	a := types.AnnOf(fmap)
	if a.Ref != types.RefMethod || len(a.Inst) != 2 || len(a.Dicts) != 1 {
		t.Fatalf("method annotation = %#v", a)
	}
	d := a.Dicts[0]
	if d.Kind != types.DictSuper || d.Class != "Functor" || d.Super == nil || d.Super.Kind != types.DictParam || d.Super.Param != "M" || d.Super.ParamClass != "Monad" {
		t.Fatalf("superclass dictionary = %#v", d)
	}
	for name, con := range map[string]string{"option": "Option", "effect": "IO"} {
		a = types.AnnOf(p.Funcs[name].Decl.Body.Result.(*ast.Call).Fn)
		if a.Ref != types.RefFunc || len(a.Inst) != 2 || types.TypeString(a.Inst[0]) != con || types.TypeString(a.Inst[1]) != "Int" || len(a.Dicts) != 1 || a.Dicts[0].Kind != types.DictInstance || a.Dicts[0].Instance.TypeCon.Name != con {
			t.Fatalf("%s annotation = %#v", name, a)
		}
	}
	for _, name := range []string{"main", "sequenced"} {
		d := types.AnnOf(p.Funcs[name].Decl.Body.Result).Monad
		if d == nil || d.Class != "Monad" {
			t.Fatalf("%s do dictionary = %#v", name, d)
		}
		if name == "main" && (d.Kind != types.DictInstance || d.Instance.TypeCon.Name != "IO") {
			t.Fatalf("IO dictionary = %#v", d)
		}
		if name == "sequenced" && (d.Kind != types.DictParam || d.Param != "M") {
			t.Fatalf("polymorphic dictionary = %#v", d)
		}
	}
}

func TestMethodConstraintsAndSuperclassChain(t *testing.T) {
	files := parseFiles(t, `
class Strong[M[_]] : Monad[M] {}
class Convert[X] { convert[F[_] Functor, A](x X, fa F[A], f func(A) A) F[A] }
instance Convert[Int] { convert(x, fa, f) { fmap(fa, f) } }
func through[M[_] Strong, A](ma M[A]) M[A] { fmap(ma, func(a) => a) }
func conversion() Option[Int] { convert(1, Some(1), func(x) => x + 1) }
func main() IO[Unit] { ioPure(unit()) }
`)
	p, err := types.Check(files)
	if err != nil {
		t.Fatal(err)
	}
	ann := types.AnnOf(p.Funcs["through"].Decl.Body.Result.(*ast.Call).Fn)
	dict := &ann.Dicts[0]
	for _, class := range []string{"Functor", "Monad"} {
		if dict.Kind != types.DictSuper || dict.Class != class || dict.Super == nil {
			t.Fatalf("superclass hop %s = %#v", class, dict)
		}
		dict = dict.Super
	}
	if dict.Kind != types.DictParam || dict.Class != "Strong" || dict.Param != "M" || dict.ParamClass != "Strong" {
		t.Fatalf("superclass root = %#v", dict)
	}
	ann = types.AnnOf(p.Funcs["conversion"].Decl.Body.Result.(*ast.Call).Fn)
	if len(ann.Inst) != 2 || types.TypeString(ann.Inst[0]) != "Option" || types.TypeString(ann.Inst[1]) != "Int" || len(ann.Dicts) != 2 {
		t.Fatalf("method instantiation = %#v", ann)
	}
	for i, want := range []struct{ class, con string }{{"Convert", "Int"}, {"Functor", "Option"}} {
		dict := ann.Dicts[i]
		if dict.Kind != types.DictInstance || dict.Class != want.class || dict.Instance.TypeCon.Name != want.con {
			t.Fatalf("method dictionary %d = %#v", i, dict)
		}
	}
	inst := p.Instances[len(p.Instances)-1]
	ann = types.AnnOf(inst.Decl.Methods[0].Body.Result.(*ast.Call).Fn)
	if len(ann.Dicts) != 1 || ann.Dicts[0].Kind != types.DictParam || ann.Dicts[0].Param != "F" || ann.Dicts[0].ParamClass != "Functor" {
		t.Fatalf("instance method's own constraint = %#v", ann)
	}
	walkExprs(reflect.ValueOf(files), func(e ast.Expr) { assertZonked(t, e) })
}

func TestDefaultingAndRecheck(t *testing.T) {
	files := parseFiles(t, `
func unused() Unit { None; let identity = func(x) => x; unit() }
func main() IO[Unit] { ioPure(unit()) }
`)
	for range 2 {
		p, err := types.Check(files)
		if err != nil {
			t.Fatal(err)
		}
		none := p.Funcs["unused"].Decl.Body.Stmts[0].(*ast.ExprStmt).X
		ann := types.AnnOf(none)
		if ann.Ref != types.RefCtor || len(ann.Inst) != 1 || ann.Inst[0] != types.Unit || types.TypeString(ann.Type) != "Option[Unit]" {
			t.Fatalf("defaulted constructor = %#v", ann)
		}
		identity := p.Funcs["unused"].Decl.Body.Stmts[1].(*ast.LetStmt).Value
		if got := types.TypeString(types.AnnOf(identity).Type); got != "func(Unit) Unit" {
			t.Fatalf("defaulted lambda type = %s", got)
		}
		walkExprs(reflect.ValueOf(files), func(e ast.Expr) { assertZonked(t, e) })
	}
}

func TestErrorPosition(t *testing.T) {
	files := parseFiles(t, `
func bad() Int { 1 + true }
func main() IO[Unit] { ioPure(unit()) }
`)
	bad := files[1].Decls[0].(*ast.FuncDecl).Body.Result.(*ast.Binary).R
	p, err := types.Check(files)
	var typed *types.Error
	if p != nil || !errors.As(err, &typed) || typed.Pos != bad.GetPos() {
		t.Fatalf("Check = (%v, %v), want error at %s", p, err, bad.GetPos())
	}
}

func TestListLiteralAnnotations(t *testing.T) {
	tests := []struct {
		name, source, want string
	}{
		{"elements", `func value() List[Int] { [1, 2, 3] }`, "List[Int]"},
		{"empty context", `func value() List[Int] { [] }`, "List[Int]"},
		{"empty default", `func value() Unit { []; unit() }`, "List[Unit]"},
		{"parameter", `func value[A](a A) List[A] { [a] }`, "List[A]"},
		{"later context", `func value() List[Int] { let xs = []; xs }`, "generalized"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			files := parseFiles(t, tt.source+"\nfunc main() IO[Unit] { ioPure(unit()) }\n")
			p, err := types.Check(files)
			if err != nil {
				t.Fatal(err)
			}
			count := 0
			walkExprs(reflect.ValueOf(p.Funcs["value"].Decl.Body), func(e ast.Expr) {
				assertZonked(t, e)
				if _, ok := e.(*ast.ListLit); ok {
					count++
					if tt.want == "generalized" {
						app, ok := types.AnnOf(e).Type.(*types.TApp)
						if !ok || len(app.Args) != 1 {
							t.Fatalf("generalized list type = %#v", types.AnnOf(e).Type)
						}
						if _, ok := app.Args[0].(*types.TVar); !ok {
							t.Fatalf("list element = %T, want generalized variable", app.Args[0])
						}
						if got := types.TypeString(types.AnnOf(p.Funcs["value"].Decl.Body.Result).Type); got != "List[Int]" {
							t.Fatalf("list use type = %s, want List[Int]", got)
						}
						return
					}
					if got := types.TypeString(types.AnnOf(e).Type); got != tt.want {
						t.Errorf("list type = %s, want %s", got, tt.want)
					}
				}
			})
			if count != 1 {
				t.Fatalf("checked %d list literals, want 1", count)
			}
		})
	}
}

const showOptionInstance = `
instance Show[Option[A]] given Show[A] {
  show(o) { match o { None => "None"; Some(a) => show(a) } }
}
`

func TestConstrainedInstanceDictionaries(t *testing.T) {
	files := parseFiles(t, showOptionInstance+`
instance Show[List[A]] given Show[A] {
  show(xs) { match xs { Nil => "[]"; Cons(a, rest) => show(a) + show(rest) } }
}
func option() String { show(Some(1)) }
func nested() String { show(Some(Some(1))) }
func list() String { show([1, 2]) }
func generic[A Show](a A) String { show(Some(a)) }
func main() IO[Unit] { ioPure(unit()) }
`)
	for range 2 {
		p, err := types.Check(files)
		if err != nil {
			t.Fatal(err)
		}
		for name, heads := range map[string][]string{
			"option": {"Option", "Int"},
			"nested": {"Option", "Option", "Int"},
			"list":   {"List", "Int"},
		} {
			ann := types.AnnOf(p.Funcs[name].Decl.Body.Result.(*ast.Call).Fn)
			if ann.Ref != types.RefMethod || len(ann.Dicts) != 1 {
				t.Fatalf("%s annotation = %#v", name, ann)
			}
			dict := ann.Dicts[0]
			for i, head := range heads {
				assertInstanceDict(t, dict, "Show", head)
				if i == len(heads)-1 {
					if len(dict.Args) != 0 {
						t.Fatalf("Show_Int arguments = %#v", dict.Args)
					}
				} else {
					if len(dict.Args) != 1 {
						t.Fatalf("%s dictionary arguments = %#v", head, dict.Args)
					}
					dict = dict.Args[0]
				}
			}
		}
		dict := types.AnnOf(p.Funcs["generic"].Decl.Body.Result.(*ast.Call).Fn).Dicts[0]
		assertInstanceDict(t, dict, "Show", "Option")
		if len(dict.Args) != 1 {
			t.Fatalf("generic dictionary arguments = %#v", dict.Args)
		}
		assertParamDict(t, dict.Args[0], "Show", "A")
		for _, inst := range p.Instances {
			if inst.Class.Name != "Show" || len(inst.Params) == 0 {
				continue
			}
			if len(inst.Params) != 1 || inst.Params[0].Name != "A" || len(inst.Context) != 1 || inst.Context[0].Class != "Show" || inst.Context[0].Arg != inst.Params[0] {
				t.Fatalf("instance parameters/context = %#v / %#v", inst.Params, inst.Context)
			}
			if _, ok := inst.Params[0].Kind.(types.KStar); !ok {
				t.Fatalf("instance parameter kind = %s", types.KindString(inst.Params[0].Kind))
			}
			paramUses, recursiveUses := 0, 0
			walkExprs(reflect.ValueOf(inst.Decl.Methods[0].Body), func(e ast.Expr) {
				id, ok := e.(*ast.Ident)
				if !ok || id.Name != "show" {
					return
				}
				d := types.AnnOf(id).Dicts[0]
				if d.Kind == types.DictParam {
					paramUses++
					assertParamDict(t, d, "Show", "A")
				} else {
					recursiveUses++
					assertInstanceDict(t, d, "Show", "List")
					if d.Instance != inst || len(d.Args) != 1 {
						t.Fatalf("recursive dictionary = %#v", d)
					}
					assertParamDict(t, d.Args[0], "Show", "A")
				}
			})
			wantRecursive := 0
			if inst.TypeCon.Name == "List" {
				wantRecursive = 1
			}
			if paramUses != 1 || recursiveUses != wantRecursive {
				t.Fatalf("%s method dictionary uses = (%d, %d), want (1, %d)", inst.TypeCon.Name, paramUses, recursiveUses, wantRecursive)
			}
		}
		walkExprs(reflect.ValueOf(files), func(e ast.Expr) { assertZonked(t, e) })
	}
}

func TestConstrainedInstanceDiagnostics(t *testing.T) {
	tests := []struct {
		name, source, want string
	}{
		{"list elements", `func bad() List[Int] { [1, true] }`, "type mismatch: expected Int, got Bool"},
		{"missing context instance", showOptionInstance + `func bad() String { show(Some(unit())) }`, "no instance Show[Unit] (required by instance Show[Option[A]])"},
		{"context kind", `instance Show[Option[A]] given Monad[A] { show(o) { "x" } }`, "kind mismatch: got *, want (*) -> *"},
		{"head kind", `instance Functor[Option[A]] { fmap(fa, f) { fa } }`, "kind mismatch: got (*) -> *, want *"},
		{"head arity", `instance Show[Option[A, B]] { show(o) { "x" } }`, "kind mismatch applying Option to 2 arguments"},
		{"unknown context class", `instance Show[Option[A]] given Missing[A] { show(o) { "x" } }`, "unknown class Missing"},
		{"duplicate constrained instance", showOptionInstance + `instance Show[Option[B]] given Show[B] { show(o) { "x" } }`, "duplicate instance Show[Option]"},
		{"rigid instance parameter", `instance Show[Option[A]] given Show[A] { show(o) { match o { None => "None"; Some(a) => showInt(a) } } }`, "type mismatch: expected Int, got A"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			files := parseFiles(t, tt.source+"\nfunc main() IO[Unit] { ioPure(unit()) }\n")
			p, err := types.Check(files)
			var typed *types.Error
			if p != nil || !errors.As(err, &typed) || typed.Msg != tt.want || typed.Pos.Line == 0 || typed.Pos.Col == 0 {
				t.Fatalf("Check = (%v, %v), want positioned error %q", p, err, tt.want)
			}
		})
	}
}

func TestInvalidInstanceAST(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*ast.InstanceDecl)
		want   string
	}{
		{"duplicate head parameter", func(inst *ast.InstanceDecl) {
			inst.Params[1] = inst.Params[0]
		}, "duplicate type parameter A"},
		{"unknown context parameter", func(inst *ast.InstanceDecl) {
			inst.Context[0].Param = "Unknown"
		}, "unknown instance type parameter Unknown"},
		{"context without parameters", func(inst *ast.InstanceDecl) {
			inst.TypeCon, inst.Params = "Bool", nil
		}, "unknown instance type parameter A"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			files := parseFiles(t, `
type Pair[A, B] = PairOf(A, B)
instance Show[Pair[A, B]] given Show[A] { show(p) { "pair" } }
func main() IO[Unit] { ioPure(unit()) }
`)
			tt.mutate(files[1].Decls[1].(*ast.InstanceDecl))
			p, err := types.Check(files)
			var typed *types.Error
			if p != nil || !errors.As(err, &typed) || typed.Msg != tt.want {
				t.Fatalf("Check = (%v, %v), want %q", p, err, tt.want)
			}
		})
	}
}

func TestConstrainedInstanceContextOrderAndSuperclasses(t *testing.T) {
	files := parseFiles(t, showOptionInstance+`
class Describe[X] : Show[X] {}
instance Describe[Int] {}
type Pair[A, B] = PairOf(A, B)
instance Show[Pair[A, B]] given Show[B], Describe[A] {
  show(pair) { match pair { PairOf(a, b) => show(a) + show(b) } }
}
func pair() String { show(PairOf(1, Some(2))) }
func main() IO[Unit] { ioPure(unit()) }
`)
	p, err := types.Check(files)
	if err != nil {
		t.Fatal(err)
	}
	dict := types.AnnOf(p.Funcs["pair"].Decl.Body.Result.(*ast.Call).Fn).Dicts[0]
	assertInstanceDict(t, dict, "Show", "Pair")
	if len(dict.Args) != 2 {
		t.Fatalf("pair dictionary arguments = %#v", dict.Args)
	}
	assertInstanceDict(t, dict.Args[0], "Show", "Option")
	if len(dict.Args[0].Args) != 1 {
		t.Fatalf("option dictionary arguments = %#v", dict.Args[0].Args)
	}
	assertInstanceDict(t, dict.Args[0].Args[0], "Show", "Int")
	assertInstanceDict(t, dict.Args[1], "Describe", "Int")
	method := dict.Instance.Decl.Methods[0].Body.Result.(*ast.Match).Arms[0].Body.(*ast.Binary)
	left := types.AnnOf(method.L.(*ast.Call).Fn).Dicts[0]
	if left.Kind != types.DictSuper || left.Class != "Show" || left.Super == nil {
		t.Fatalf("context superclass dictionary = %#v", left)
	}
	assertParamDict(t, *left.Super, "Describe", "A")
	assertParamDict(t, types.AnnOf(method.R.(*ast.Call).Fn).Dicts[0], "Show", "B")
	walkExprs(reflect.ValueOf(files), func(e ast.Expr) { assertZonked(t, e) })
}

func TestParameterizedInstanceMethodScopes(t *testing.T) {
	files := parseFiles(t, `
class Inspect[X] { inspect[B Show](x X, b B) String }
instance Inspect[Option[A]] given Show[A] {
  inspect(o, b) { show(b) + (match o { None => ""; Some(a) => show((func(x A) => x)(a)) }) }
}
instance Show[Option[A]] { show(o) { "option" } }
func value() String { inspect(Some(1), 2) }
func option() String { show(Some(unit())) }
func main() IO[Unit] { ioPure(unit()) }
`)
	p, err := types.Check(files)
	if err != nil {
		t.Fatal(err)
	}
	ann := types.AnnOf(p.Funcs["value"].Decl.Body.Result.(*ast.Call).Fn)
	if len(ann.Dicts) != 2 {
		t.Fatalf("inspect dictionaries = %#v", ann.Dicts)
	}
	assertInstanceDict(t, ann.Dicts[0], "Inspect", "Option")
	if len(ann.Dicts[0].Args) != 1 {
		t.Fatalf("instance context arguments = %#v", ann.Dicts[0].Args)
	}
	assertInstanceDict(t, ann.Dicts[0].Args[0], "Show", "Int")
	assertInstanceDict(t, ann.Dicts[1], "Show", "Int")
	method := ann.Dicts[0].Instance.Decl.Methods[0].Body.Result.(*ast.Binary)
	assertParamDict(t, types.AnnOf(method.L.(*ast.Call).Fn).Dicts[0], "Show", "B")
	member := method.R.(*ast.Match).Arms[1].Body.(*ast.Call)
	assertParamDict(t, types.AnnOf(member.Fn).Dicts[0], "Show", "A")
	dict := types.AnnOf(p.Funcs["option"].Decl.Body.Result.(*ast.Call).Fn).Dicts[0]
	assertInstanceDict(t, dict, "Show", "Option")
	if len(dict.Args) != 0 {
		t.Fatalf("context-free parameterized instance arguments = %#v", dict.Args)
	}
	walkExprs(reflect.ValueOf(files), func(e ast.Expr) { assertZonked(t, e) })
}

func TestListLiteralRequiresPrelude(t *testing.T) {
	for _, declaration := range []string{"", "type List[A] = Empty | Full(A)"} {
		t.Run(declaration, func(t *testing.T) {
			file, err := parser.Parse("package main\n" + declaration + `
extern func ioPure[A](a A) IO[A] = "gofx/rt".Pure
extern func unit() Unit = "gofx/rt".UnitValue
func main() IO[Unit] { []; ioPure(unit()) }
`)
			if err != nil {
				t.Fatal(err)
			}
			p, err := types.Check([]*ast.File{file})
			var typed *types.Error
			if p != nil || !errors.As(err, &typed) || typed.Msg != "list literal requires prelude List" {
				t.Fatalf("Check = (%v, %v), want missing prelude List error", p, err)
			}
		})
	}
}

func TestSuperclassInstanceContext(t *testing.T) {
	tests := []struct {
		name, parent, child string
		wantError           bool
	}{
		{"matching", "Functor[Pair[A, B]] given Show[A], Other[B]", "Monad[Pair[X, Y]] given Show[X], Other[Y]", false},
		{"extra child constraint", "Functor[Pair[A, B]]", "Monad[Pair[A, B]] given Show[A]", false},
		{"missing child constraint", "Functor[Pair[A, B]] given Show[A]", "Monad[Pair[A, B]]", true},
		{"different classes", "Functor[Pair[A, B]] given Show[A]", "Monad[Pair[A, B]] given Other[A]", true},
		{"different order", "Functor[Pair[A, B]] given Show[A], Other[B]", "Monad[Pair[A, B]] given Other[B], Show[A]", false},
		{"different parameters", "Functor[Pair[A, B]] given Show[A]", "Monad[Pair[A, B]] given Show[B]", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			file, err := parser.Parse(`package main
class Functor[F] {}
class Monad[M] : Functor[M] {}
class Show[A] {}
class Other[A] {}
type Pair[A, B] = PairOf(A, B)
extern func main() IO[Unit] = "example".Main
instance ` + tt.parent + " {}\ninstance " + tt.child + " {}\n")
			if err != nil {
				t.Fatal(err)
			}
			p, err := types.Check([]*ast.File{file})
			if !tt.wantError {
				if err != nil {
					t.Fatal(err)
				}
				return
			}
			var typed *types.Error
			if p != nil || !errors.As(err, &typed) || typed.Msg != "superclass instance Functor[Pair] context is not implied" {
				t.Fatalf("Check = (%v, %v), want superclass context mismatch", p, err)
			}
		})
	}
}

func assertInstanceDict(t *testing.T, dict types.DictRef, class, con string) {
	t.Helper()
	if dict.Kind != types.DictInstance || dict.Class != class || dict.Instance == nil || dict.Instance.Class.Name != class || dict.Instance.TypeCon.Name != con {
		t.Fatalf("dictionary = %#v, want instance %s_%s", dict, class, con)
	}
}

func assertParamDict(t *testing.T, dict types.DictRef, class, param string) {
	t.Helper()
	if dict.Kind != types.DictParam || dict.Class != class || dict.Param != param || dict.ParamClass != class {
		t.Fatalf("dictionary = %#v, want parameter %s %s", dict, param, class)
	}
}

// Reflection walks only surface AST fields and deliberately skips annotations,
// whose dictionary references point back into the same declarations.
func walkExprs(v reflect.Value, visit func(ast.Expr)) {
	if !v.IsValid() {
		return
	}
	if v.Kind() == reflect.Interface || v.Kind() == reflect.Pointer {
		if v.IsNil() {
			return
		}
		if v.Kind() == reflect.Pointer && v.CanInterface() {
			if e, ok := v.Interface().(ast.Expr); ok {
				visit(e)
			}
		}
		walkExprs(v.Elem(), visit)
		return
	}
	switch v.Kind() {
	case reflect.Slice:
		for i := range v.Len() {
			walkExprs(v.Index(i), visit)
		}
	case reflect.Struct:
		for i := range v.NumField() {
			if v.Type().Field(i).Name != "Ann" {
				walkExprs(v.Field(i), visit)
			}
		}
	}
}

func assertZonked(t *testing.T, e ast.Expr) {
	t.Helper()
	a, ok := (*e.AnnSlot()).(*types.Ann)
	if !ok {
		t.Fatalf("%T at %s has no annotation", e, e.GetPos())
	}
	var check func(types.Type)
	check = func(typ types.Type) {
		switch typ := typ.(type) {
		case *types.TMeta:
			t.Fatalf("%T at %s contains meta %s", e, e.GetPos(), types.TypeString(typ))
		case *types.TApp:
			check(typ.Fun)
			for _, arg := range typ.Args {
				check(arg)
			}
		case *types.TFunc:
			for _, param := range typ.Params {
				check(param)
			}
			check(typ.Ret)
		case *types.TVar, *types.TCon:
		default:
			t.Fatalf("%T at %s has invalid type %T", e, e.GetPos(), typ)
		}
	}
	check(a.Type)
	for _, inst := range a.Inst {
		check(inst)
	}
	if a.LetScheme != nil {
		check(a.LetScheme.Type)
		for _, constraint := range a.LetScheme.Constraints {
			check(constraint.Arg)
		}
	}
}
