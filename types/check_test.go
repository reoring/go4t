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
`

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
		{"rigid type", `func bad[A](x A) A { 1 }` + main, "cannot unify"},
		{"occurs check", `func bad() Unit { let f = func(x) => x(x); unit() }` + main, "occurs check"},
		{"monomorphic let", `func bad() Unit { let id = func(x) => x; id(1); id(true); unit() }` + main, "cannot unify"},
		{"call arity", `func bad() Unit { ioPure(); unit() }` + main, "arity mismatch"},
		{"mixed monads", `func bad() Option[Int] { do { a <- Some(1); ioPure(a) } }` + main, "cannot unify"},
		{"ambiguous class", `func bad() Unit { pure(1); unit() }` + main, "unresolved class constraint"},
		{"default unconstrained", `func okay() Unit { None; let f = func(x) => x; unit() }` + main, ""},
		{"primitive comparison", `func okay() Bool { let same = func(x) => x == x; same(1) }` + main, ""},
		{"polymorphic comparison", `func bad[A](x A) Bool { x == x }` + main, "comparison requires Int, String or Bool"},
		{"data comparison", `func bad() Bool { Some(1) == Some(1) }` + main, "comparison requires Int, String or Bool"},
		{"bool ordering", `func bad() Bool { false < true }` + main, "ordering comparison requires Int or String"},
		{"operators", `func okay(x Int, y String, z Bool) Bool { let n = -(x + 2) * 3 / 2 - 1; if !z && n >= 0 { y < "z" || n != 2 } else { z == false } }` + main, ""},
		{"literal catchall", `func okay(x Int) Bool { match x { 1 => true; _ => false } }` + main, ""},
		{"literal incomplete", `func bad(x Bool) Int { match x { true => 1; false => 0 } }` + main, "non-exhaustive match"},
		{"nested incomplete", `func bad(x Option[Option[Int]]) Int { match x { None => 0; Some(Some(a)) => a } }` + main, "non-exhaustive match"},
		{"nested complete", `func okay(x Option[Option[Int]]) Int { match x { None => 0; Some(None) => 1; Some(Some(a)) => a } }` + main, ""},
		{"duplicate pattern binding", `type Pair[A] = PairOf(A, A)
func bad(x Pair[Int]) Int { match x { PairOf(a, a) => a } }` + main, "duplicate pattern variable a"},
		{"multiple constraints", `func bad[F[_] Functor Monad, A](x F[A]) F[A] { x }` + main, "at most one constraint"},
		{"unknown superclass", `class Bad[F[_]] : Missing[F] { bad[A](x F[A]) F[A] }` + main, "unknown superclass Missing"},
		{"superclass cycle", `class C[A] : D[A] {}; class D[A] : C[A] {}` + main, "superclass cycle"},
		{"superclass kind", `class Bad[A] : Functor[A] {}` + main, "kind mismatch"},
		{"missing superclass instance", `type Box[A] = Boxed(A)
instance Monad[Box] { pure(a) { Boxed(a) }; bind(ma, f) { match ma { Boxed(a) => f(a) } } }` + main, "no instance Functor[Box]"},
		{"missing method", `class C[A] { c(x A) A }; instance C[Int] {}` + main, "missing method c"},
		{"extra method", `class C[A] {}; instance C[Int] { extra(x) { x } }` + main, "unknown method extra"},
		{"wrong method arity", `class C[A] { c(x A) A }; instance C[Int] { c(x, y) { x } }` + main, "arity mismatch"},
		{"wrong method body", `class C[A] { c(x A) A }; instance C[Int] { c(x) { true } }` + main, "cannot unify"},
		{"duplicate declaration", `func ioPure(x Int) Int { x }` + main, "duplicate declaration ioPure"},
		{"recursive signatures", `func left(x Int) Int { right(x) }; func right(x Int) Int { left(x) }` + main, ""},
		{"kind in field", `type Bad = BadValue(Option)` + main, "kind mismatch"},
		{"recursive wildcard incomplete", `type List = Cons(List) | Nil
type Pair = PairOf(List, Option[Int])
func bad(x Pair) Int { match x { PairOf(_, None) => 0 } }` + main, "non-exhaustive match"},
		{"recursive match complete", `type List[A] = Cons(A, List[A]) | Nil
func size[A](xs List[A]) Int { match xs { Cons(_, rest) => 1 + size(rest); Nil => 0 } }` + main, ""},
		{"HKT binary arity", `type Pair[A, B] = PairOf(A, B)
class Bifunctor[F[_, _]] { bimap[A, B, C, D](pair F[A, B], f func(A) C, g func(B) D) F[C, D] }
instance Bifunctor[Pair] { bimap(pair, f, g) { match pair { PairOf(a, b) => PairOf(f(a), g(b)) } } }
func both() Pair[Int, Bool] { bimap(PairOf(1, true), func(x) => x + 1, func(x) => !x) }` + main, ""},
		{"HKT data arity mismatch", `type Pair[A, B] = PairOf(A, B)
instance Functor[Pair] { fmap(pair, f) { pair } }` + main, "kind mismatch"},
		{"unresolved higher kind", `extern func phantom[F[_]]() Unit = "example".Phantom
func bad() Unit { phantom() }` + main, "cannot infer type constructor of kind"},
		{"ordinary class constraint", `class Show[A] { render(a A) String }
instance Show[Int] { render(a) { "integer" } }
func display[A Show](a A) String { render(a) }
func use() String { display(1) }` + main, ""},
		{"constraint without binding", `func bad[F[_], A](fa F[A]) F[A] { fmap(fa, func(x) => x) }` + main, "no instance Functor[F]"},
		{"typed lambda", `func okay[A](a A) A { (func(x A) => x)(a) }` + main, ""},
		{"nullary constructor value", `func okay() Option[Int] { None }` + main, ""},
		{"nullary constructor not callable", `func bad() Option[Int] { None() }` + main, "cannot unify"},
		{"wrong pattern arity", `func bad(x Option[Int]) Int { match x { Some(a, b) => a; None => 0 } }` + main, "arity mismatch for pattern"},
		{"wrong pattern type", `func bad(x Int) Int { match x { Some(a) => a; _ => 0 } }` + main, "cannot unify"},
		{"same named rigid parameters", `func wrong[A, B](a A) B { a }` + main, "cannot unify"},
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
}
