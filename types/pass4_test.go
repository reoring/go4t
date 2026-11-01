package types_test

import (
	"errors"
	"reflect"
	"strings"
	"testing"

	"go4t/ast"
	"go4t/types"
)

func TestPass4NamespacesAndPatterns(t *testing.T) {
	tests := []struct{ name, source, want string }{
		{"type and constructor", `type Sku = Sku(String)
func sku() Sku { Sku("item") }
func unpack(s Sku) String { match s { Sku(value) => value } }`, ""},
		{"duplicate types", `type Sku = First; type Sku = Second`, "duplicate declaration Sku"},
		{"duplicate constructors", `type One = Sku; type Two = Sku`, "duplicate declaration Sku"},
		{"lowercase constructor", `type Sku = sku(String)`, "constructor name must be capitalised"},
		{"uppercase function", `func Sku() Int { 1 }`, "value name must start with a lower-case letter"},
		{"uppercase method", `class C[A] { Sku(a A) String }`, "value name must start with a lower-case letter"},
		{"uppercase parameter", `func f(Sku Int) Int { 1 }`, "value name must start with a lower-case letter"},
		{"unknown constructor in let", `func f() Int { let Sku = 1; 1 }`, "unknown constructor Sku"},
		{"type class collision", `type Sku = Item; class Sku[A] {}`, "duplicate declaration Sku"},
		{"builtin tuple collision", `type Tuple2 = Item`, "duplicate declaration Tuple2"},
		{"constructor not type", `type Box = Wrapped; func f(x Wrapped) Int { 1 }`, "unknown type Wrapped"},
		{"type not constructor", `type Box = Wrapped; func f() Box { Box }`, "unknown identifier Box"},
		{"constructor not instance head", `type Box = Wrapped; instance Show[Wrapped] { show(x) { "x" } }`, "unknown type constructor Wrapped"},
		{"tuple literal type pattern", `func f() (Int, String) { (1, "x") }
func first(pair (Int, String)) Int { match pair { (a, _) => a } }`, ""},
		{"nested tuples", `func f() (Int, (String, Bool), Int) { (1, ("x", true), 2) }
func nested(p (Int, (String, Bool), Int)) String { match p { (_, (s, _), _) => s } }`, ""},
		{"tuple nested exhaustive", `func f(p (Option[Int], Int)) Int { match p { (Some(x), _) => x; (None, y) => y } }`, ""},
		{"tuple nested incomplete", `func f(p (Option[Int], Int)) Int { match p { (Some(x), _) => x } }`, "non-exhaustive match"},
		{"tuple product incomplete", `func f(p (Option[Int], Option[Int])) Int { match p { (Some(x), _) => x; (_, Some(y)) => y } }`, "non-exhaustive match"},
		{"tuple recursive wildcard", `func f(p (List[Int], Option[Int])) Int { match p { (_, Some(x)) => x } }`, "non-exhaustive match"},
		{"tuple literals incomplete", `func f(p (Int, Int)) Int { match p { (1, _) => 1 } }`, "non-exhaustive match"},
		{"tuple arity mismatch", `func f(p (Int, Int)) Int { match p { (a, _, _) => a } }`, "type mismatch"},
		{"as binding", `func f(p Option[Int]) Option[Int] { match p { whole @ Some(x) => whole; whole @ None => whole } }`, ""},
		{"nested as binding", `func f(p (Option[Int], Int)) Option[Int] { match p { (whole @ Some(x), _) => whole; (whole @ None, _) => whole } }`, ""},
		{"as catchall", `func f(x Int) Int { match x { 1 => 0; whole @ _ => whole } }`, ""},
		{"as literal incomplete", `func f(x Int) Int { match x { whole @ 1 => whole } }`, "non-exhaustive match"},
		{"as duplicate", `func f(p Option[Int]) Int { match p { x @ Some(x) => x; None => 0 } }`, "duplicate pattern variable x"},
		{"tuple duplicate", `func f(p (Int, Int)) Int { match p { (x, x) => x } }`, "duplicate pattern variable x"},
		{"as sibling duplicate", `func f(p (Int, Int)) Int { match p { (x @ _, x) => x } }`, "duplicate pattern variable x"},
		{"as shadow outer", `func f(x Option[Int]) Option[Int] { match x { x @ _ => x } }`, ""},
		{"partial head missing arguments", `type Result[E, A] = Err(E) | Ok(A)
instance Functor[Result] { fmap(r, f) { r } }`, "kind mismatch"},
		{"partial head excess arguments", `type Result[E, A] = Err(E) | Ok(A)
instance Functor[Result[E, A]] { fmap(r, f) { r } }`, "kind mismatch"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			files := parseFiles(t, tt.source+"\nfunc main() IO[Unit] { ioPure(unit()) }\n")
			p, err := checkFixture(t, []*ast.Module{{Files: files}})
			if tt.want != "" {
				var typed *types.Error
				if p != nil || !errors.As(err, &typed) || !strings.Contains(typed.Msg, tt.want) {
					t.Fatalf("Check = (%v, %v), want %q", p, err, tt.want)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			walkExprs(reflect.ValueOf(files), func(e ast.Expr) { assertZonked(t, e) })
		})
	}
}

func TestTupleAnnotationsAndDictionaries(t *testing.T) {
	files := parseFiles(t, `
instance Show[(A, B)] given Show[A], Show[B] {
  show(pair) { match pair { (a, b) => "(" + show(a) + ", " + show(b) + ")" } }
}
func pair() (Int, (Int, Int)) { (1, (2, 3)) }
func display() String { show((1, (2, 3))) }
func main() IO[Unit] { ioPure(unit()) }
`)
	p, err := checkFixture(t, []*ast.Module{{Files: files}})
	if err != nil {
		t.Fatal(err)
	}
	if got := types.TypeString(types.AnnOf(p.Funcs["pair"].Decl.Body.Result).Type); got != "Tuple2[Int, Tuple2[Int, Int]]" {
		t.Fatalf("tuple type = %s", got)
	}
	dict := types.AnnOf(p.Funcs["display"].Decl.Body.Result.(*ast.Call).Fn).Dicts[0]
	assertInstanceDict(t, dict, "Show", "Tuple2")
	if dict.Instance.TypeCon != types.Tuple(2) || len(dict.Instance.Params) != 2 || len(dict.Instance.Fixed) != 0 || len(dict.Args) != 2 {
		t.Fatalf("tuple dictionary = %#v", dict)
	}
	assertInstanceDict(t, dict.Args[0], "Show", "Int")
	assertInstanceDict(t, dict.Args[1], "Show", "Tuple2")
	if len(dict.Args[1].Args) != 2 {
		t.Fatalf("nested tuple dictionaries = %#v", dict.Args[1])
	}
	for _, arg := range dict.Args[1].Args {
		assertInstanceDict(t, arg, "Show", "Int")
	}
	walkExprs(reflect.ValueOf(files), func(e ast.Expr) { assertZonked(t, e) })
}

const partialResultInstances = `
type Result[E, A] = Err(E) | Ok(A)
instance Functor[Result[E]] {
  fmap(r, f) { match r { Err(e) => Err(e); Ok(a) => Ok(f(a)) } }
}
instance Monad[Result[E]] {
  pure(a) { Ok(a) }
  bind(r, f) { match r { Err(e) => Err(e); Ok(a) => f(a) } }
}
func twice[F[_] Functor, A](fa F[A], f func(A) A) F[A] { fmap(fmap(fa, f), f) }
`

func TestPartialInstanceAnnotations(t *testing.T) {
	files := parseFiles(t, partialResultInstances+`
func mapped(r Result[String, Int]) Result[String, Int] { fmap(r, func(x) => x + 1) }
func repeated(r Result[String, Int]) Result[String, Int] { twice(r, func(x) => x + 1) }
func sequenced(r Result[String, Int]) Result[String, Int] { do { x <- r; pure(x + 1) } }
func main() IO[Unit] { ioPure(unit()) }
`)
	for range 2 {
		p, err := checkFixture(t, []*ast.Module{{Files: files}})
		if err != nil {
			t.Fatal(err)
		}
		var functor, monad *types.Instance
		for _, inst := range p.Instances {
			if inst.TypeCon.Name != "Result" {
				continue
			}
			if len(inst.Fixed) != 1 || inst.Fixed[0].Name != "E" || len(inst.Params) != 0 {
				t.Fatalf("partial instance parameters = %#v", inst)
			}
			if _, ok := inst.Fixed[0].Kind.(types.KStar); !ok {
				t.Fatalf("fixed parameter kind = %s", types.KindString(inst.Fixed[0].Kind))
			}
			if inst.Class.Name == "Functor" {
				functor = inst
			} else if inst.Class.Name == "Monad" {
				monad = inst
			}
		}
		for _, name := range []string{"mapped", "repeated"} {
			call := p.Funcs[name].Decl.Body.Result.(*ast.Call)
			ann := types.AnnOf(call.Fn)
			if len(ann.Dicts) != 1 || ann.Dicts[0].Instance != functor {
				t.Fatalf("%s dictionaries = %#v", name, ann.Dicts)
			}
			assertInstanceDict(t, ann.Dicts[0], "Functor", "Result")
			if name == "repeated" {
				app, ok := ann.Inst[0].(*types.TApp)
				if !ok || app.Fun != functor.TypeCon || len(app.Args) != 1 || app.Args[0] != types.String {
					t.Fatalf("F instantiation = %s", types.TypeString(ann.Inst[0]))
				}
			}
			if got := types.TypeString(types.AnnOf(call).Type); got != "Result[String, Int]" {
				t.Fatalf("%s type = %s", name, got)
			}
		}
		do := types.AnnOf(p.Funcs["sequenced"].Decl.Body.Result)
		if do.Monad == nil || do.Monad.Instance != monad || types.TypeString(do.Type) != "Result[String, Int]" {
			t.Fatalf("partial do annotation = %#v", do)
		}
		assertInstanceDict(t, *do.Monad, "Monad", "Result")
		walkExprs(reflect.ValueOf(files), func(e ast.Expr) { assertZonked(t, e) })
	}
}

func TestPartialInstanceContext(t *testing.T) {
	files := parseFiles(t, `
type Result[E, A] = Err(E) | Ok(A)
instance Show[String] { show(s) { s } }
instance Functor[Result[E]] given Show[E] {
  fmap(r, f) { match r { Err(e) => { show((func(x E) => x)(e)); Err(e) }; Ok(a) => Ok(f(a)) } }
}
instance Monad[Result[X]] given Show[X] {
  pure(a) { Ok(a) }
  bind(r, f) { match r { Err(e) => Err(e); Ok(a) => f(a) } }
}
func mapped(r Result[String, Int]) Result[String, Int] { fmap(r, func(x) => x) }
func generic[E Show, A](r Result[E, A]) Result[E, A] { fmap(r, func(x) => x) }
func sequenced(r Result[String, Int]) Result[String, Int] { do { x <- r; pure(x) } }
func main() IO[Unit] { ioPure(unit()) }
`)
	p, err := checkFixture(t, []*ast.Module{{Files: files}})
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"mapped", "generic"} {
		dict := types.AnnOf(p.Funcs[name].Decl.Body.Result.(*ast.Call).Fn).Dicts[0]
		assertInstanceDict(t, dict, "Functor", "Result")
		if len(dict.Args) != 1 || len(dict.Instance.Context) != 1 || dict.Instance.Context[0].Arg != dict.Instance.Fixed[0] {
			t.Fatalf("%s context dictionary = %#v", name, dict)
		}
		if name == "mapped" {
			assertInstanceDict(t, dict.Args[0], "Show", "String")
		} else {
			assertParamDict(t, dict.Args[0], "Show", "E")
		}
	}
	dict := types.AnnOf(p.Funcs["sequenced"].Decl.Body.Result).Monad
	if dict == nil || len(dict.Args) != 1 {
		t.Fatalf("partial Monad context = %#v", dict)
	}
	assertInstanceDict(t, *dict, "Monad", "Result")
	assertInstanceDict(t, dict.Args[0], "Show", "String")
	for _, inst := range p.Instances {
		if inst.Class.Name == "Functor" && inst.TypeCon.Name == "Result" {
			uses := 0
			walkExprs(reflect.ValueOf(inst.Decl.Methods[0].Body), func(e ast.Expr) {
				if id, ok := e.(*ast.Ident); ok && id.Name == "show" {
					assertParamDict(t, types.AnnOf(id).Dicts[0], "Show", "E")
					uses++
				}
			})
			if uses != 1 {
				t.Fatalf("Fixed dictionary uses = %d", uses)
			}
		}
	}
	walkExprs(reflect.ValueOf(files), func(e ast.Expr) { assertZonked(t, e) })
}

func TestPartialInstanceContextFailures(t *testing.T) {
	for _, tt := range []struct{ name, source, want string }{
		{"missing Fixed dictionary", `
type Result[E, A] = Err(E) | Ok(A)
instance Functor[Result[E]] given Show[E] {
  fmap(r, f) { match r { Err(e) => Err(e); Ok(a) => Ok(f(a)) } }
}
func mapped(r Result[Unit, Int]) Result[Unit, Int] { fmap(r, func(x) => x) }
`, "no instance Show[Unit] (required by instance Functor[Result[E]])"},
		{"Fixed remains rigid", `
type Result[E, A] = Err(E) | Ok(A)
instance Functor[Result[E]] {
  fmap(r, f) { match r { Err(e) => { showInt(e); Err(e) }; Ok(a) => Ok(f(a)) } }
}
`, "type mismatch: expected Int, got E"},
		{"Fixed superclass positions", `
type Result[E, X, A] = Err(E) | Extra(X) | Ok(A)
instance Functor[Result[E, X]] given Show[E] {
  fmap(r, f) { match r { Err(e) => Err(e); Extra(x) => Extra(x); Ok(a) => Ok(f(a)) } }
}
instance Monad[Result[E, X]] given Show[X] {
  pure(a) { Ok(a) }
  bind(r, f) { match r { Err(e) => Err(e); Extra(x) => Extra(x); Ok(a) => f(a) } }
}
`, "superclass instance Functor[Result] context is not implied"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			files := parseFiles(t, tt.source+"\nfunc main() IO[Unit] { ioPure(unit()) }\n")
			p, err := checkFixture(t, []*ast.Module{{Files: files}})
			var typed *types.Error
			if p != nil || !errors.As(err, &typed) || typed.Msg != tt.want {
				t.Fatalf("Check = (%v, %v), want %q", p, err, tt.want)
			}
		})
	}
}
