package types_test

import (
	"errors"
	"reflect"
	"testing"

	"gofx/ast"
	"gofx/types"
)

const pass5Main = "\nfunc main() IO[Unit] { ioPure(unit()) }\n"
const pass5Show = `
instance Show[String] { show(s) { s } }
instance Show[Bool] { show(b) { if b { "true" } else { "false" } } }
`

func TestLetPolymorphism(t *testing.T) {
	for _, source := range []string{
		`func value() String { let id = func(x) => x; id(1); id("text") }`,
		`func value() IO[String] { do { let id = func(x) => x; ioPure(id(1)); ioPure(id("text")) } }`,
		`func value() (Option[Int], Option[String]) { let empty = None; (empty, empty) }`,
		`func value() (List[Int], List[String]) { let empty = []; (empty, empty) }`,
		`func value() (Option[func(Int) Int], Option[func(String) String]) { let wrapped = Some(func(x) => x); (wrapped, wrapped) }`,
	} {
		t.Run(source, func(t *testing.T) {
			files := parseFiles(t, source+pass5Main)
			p, err := types.Check(files)
			if err != nil {
				t.Fatal(err)
			}
			if len(p.Warnings) != 0 {
				t.Fatalf("unexpected warnings: %v", p.Warnings)
			}
			uses := 0
			walkExprs(reflect.ValueOf(p.Funcs["value"].Decl.Body), func(e ast.Expr) {
				assertZonked(t, e)
				ann := types.AnnOf(e)
				if ann.LetScheme != nil || len(ann.Dicts) != 0 {
					t.Errorf("unconstrained value has dictionaries: %#v", ann)
				}
				if _, ok := e.(*ast.Ident); ok && ann.Ref == types.RefLocal && len(ann.Inst) != 0 {
					uses++
				}
			})
			if uses != 2 {
				t.Fatalf("polymorphic local use sites = %d, want 2", uses)
			}
		})
	}
}

func TestConstrainedLetDictionaries(t *testing.T) {
	files := parseFiles(t, pass5Show+`
func value() String {
 let twiceShow = func(x) => show(x) + show(x)
 twiceShow(1) + twiceShow(true)
}
`+pass5Main)
	for range 2 {
		p, err := types.Check(files)
		if err != nil {
			t.Fatal(err)
		}
		body := p.Funcs["value"].Decl.Body
		value := body.Stmts[0].(*ast.LetStmt).Value
		scheme := types.AnnOf(value).LetScheme
		if scheme == nil || len(scheme.TParams) != 1 || len(scheme.Constraints) != 1 || scheme.Constraints[0].Class != "Show" || scheme.Constraints[0].Arg != scheme.TParams[0] {
			t.Fatalf("let scheme = %#v", scheme)
		}
		binary := body.Result.(*ast.Binary)
		for i, expr := range []ast.Expr{binary.L, binary.R} {
			ann := types.AnnOf(expr.(*ast.Call).Fn)
			if ann.Ref != types.RefLocal || len(ann.Inst) != 1 || len(ann.Dicts) != 1 {
				t.Fatalf("local annotation = %#v", ann)
			}
			assertInstanceDict(t, ann.Dicts[0], "Show", []string{"Int", "Bool"}[i])
		}
		walkExprs(reflect.ValueOf(value), func(e ast.Expr) {
			assertZonked(t, e)
			if id, ok := e.(*ast.Ident); ok && id.Name == "show" {
				assertParamDict(t, types.AnnOf(id).Dicts[0], "Show", scheme.TParams[0].Name)
			}
		})
	}
}

func TestLetValueRestriction(t *testing.T) {
	for _, source := range []string{
		`func make() func(Int) Int { func(x) => x }; func bad() Unit { let f = make(); f(1); f("text"); unit() }`,
		`func identity[A](x A) A { x }; func bad() Unit { let f = identity(func(x) => x); f(1); f("text"); unit() }`,
		`func bad() Unit { (func(f) => { f(1); f("text"); unit() })(func(x) => x) }`,
		`func bad() Unit { (func(x) => { let f = func(y) => x(y); f(1); f("text"); unit() })(func(x) => x) }`,
	} {
		t.Run(source, func(t *testing.T) {
			p, err := types.Check(parseFiles(t, source+pass5Main))
			var typed *types.Error
			if p != nil || !errors.As(err, &typed) || typed.Msg != "type mismatch: expected Int, got String" {
				t.Fatalf("Check = (%v, %v)", p, err)
			}
		})
	}
}

func TestDefaultingWarnings(t *testing.T) {
	for _, tt := range []struct{ source, what string }{
		{`func value() Unit { let _x = None; unit() }`, "None"},
		{`func value() Unit { []; unit() }`, "[]"},
		{`func value() Unit { let _id = func(x) => x; unit() }`, "let _id"},
		{`func value() Unit { let _x = (func(x) => x)(None); unit() }`, "None"},
	} {
		t.Run(tt.what, func(t *testing.T) {
			files := parseFiles(t, tt.source+pass5Main)
			for range 2 {
				p, err := types.Check(files)
				if err != nil {
					t.Fatal(err)
				}
				if len(p.Warnings) != 1 {
					t.Fatalf("warnings = %v, want exactly one", p.Warnings)
				}
				warning := p.Warnings[0]
				if warning.Msg != "type of "+tt.what+" defaulted to Unit; add an annotation or use the value" || warning.Pos.Line == 0 || warning.Pos.Col == 0 {
					t.Fatalf("warning = %#v", warning)
				}
				walkExprs(reflect.ValueOf(files), func(e ast.Expr) { assertZonked(t, e) })
			}
		})
	}
	files := parseFiles(t, `func takes(xs List[Int]) Unit { unit() }; func value() Unit { takes([]) }`+pass5Main)
	p, err := types.Check(files)
	if err != nil {
		t.Fatal(err)
	}
	if len(p.Warnings) != 0 {
		t.Fatalf("context-resolved list warned: %v", p.Warnings)
	}
}

func TestRecordAnnotations(t *testing.T) {
	files := parseFiles(t, `
type User[A] = User{name: String, value: A}
func make() User[Int] { User{value: 7, name: "alice"} }
func name(u User[Int]) String { u.name }
func update(u User[Int]) User[Int] { u{value: 8, name: "bob"} }
func positional() String { match User("alice", 1) { User(name, _) => name } }
func inferred() String { let u = User{name: "alice", value: 1}; u.name }
func generic[A](u User[A]) A { u.value }
`+pass5Main)
	p, err := types.Check(files)
	if err != nil {
		t.Fatal(err)
	}
	ctor := p.DataTypes["User"].Ctors[0]
	if !reflect.DeepEqual(ctor.FieldNames, []string{"name", "value"}) {
		t.Fatalf("field names = %v", ctor.FieldNames)
	}
	for _, name := range []string{"make", "update"} {
		ann := types.AnnOf(p.Funcs[name].Decl.Body.Result)
		if ann.Ctor != ctor || !reflect.DeepEqual(ann.FieldIndexes, []int{1, 0}) || types.TypeString(ann.Type) != "User[Int]" {
			t.Fatalf("%s annotation = %#v", name, ann)
		}
	}
	ann := types.AnnOf(p.Funcs["name"].Decl.Body.Result)
	if ann.Ctor != ctor || ann.FieldIndex != 0 || ann.Type != types.String {
		t.Fatalf("field annotation = %#v", ann)
	}
	ann = types.AnnOf(p.Funcs["generic"].Decl.Body.Result)
	if ann.Ctor != ctor || ann.FieldIndex != 1 || ann.Type != p.Funcs["generic"].Scheme.TParams[0] {
		t.Fatalf("generic field annotation = %#v", ann)
	}
	walkExprs(reflect.ValueOf(files), func(e ast.Expr) {
		assertZonked(t, e)
		if id, ok := e.(*ast.Ident); ok && id.Name == "User" && types.AnnOf(id).Ctor != ctor {
			t.Error("positional record constructor lost Ctor")
		}
	})
}

func TestRecordLambdaArgumentContext(t *testing.T) {
	files := parseFiles(t, `
type Stock = Stock{sku: String, qty: Int}
instance Functor[List] {
 fmap(xs, f) { match xs { Nil => Nil; Cons(x, rest) => Cons(f(x), fmap(rest, f)) } }
}
func fold[A, B](xs List[A], init B, f func(B, A) B) B {
 match xs { Nil => init; Cons(x, rest) => fold(rest, f(init, x), f) }
}
func sameSku(a String, b String) Bool { a == b }
func total(stocks List[Stock], sku String) Int {
 fold(stocks, 0, func(acc, s) => if sameSku(sku, s.sku) { acc + s.qty } else { acc })
}
func decrement(acc List[Stock]) List[Stock] {
 fmap(acc, func(s) => s{qty: s.qty - 1})
}
`+pass5Main)
	p, err := types.Check(files)
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"total", "decrement"} {
		body := p.Funcs[name].Decl.Body
		walkExprs(reflect.ValueOf(body), func(e ast.Expr) {
			assertZonked(t, e)
			switch e := e.(type) {
			case *ast.Lambda:
				ft := types.AnnOf(e).Type.(*types.TFunc)
				if got := types.TypeString(ft.Params[len(ft.Params)-1]); got != "Stock" {
					t.Errorf("%s callback record parameter = %s", name, got)
				}
			case *ast.Field, *ast.RecordUpdate:
				if types.AnnOf(e).Ctor != p.DataTypes["Stock"].Ctors[0] {
					t.Errorf("%s callback record annotation lost constructor", name)
				}
			}
		})
	}
}

func TestRecordErrors(t *testing.T) {
	for _, tt := range []struct{ source, want string }{
		{`type R = R{a: Int, a: String}`, "duplicate field a in constructor R"},
		{`type R = R{A: Int}`, "value name must start with a lower-case letter"},
		{`type R = R{dict_a: Int}`, `identifier "dict_a" uses reserved prefix`},
		{`type R = R{a: Int, b: Int}; func f() R { R{a: 1} }`, "missing field b in constructor R"},
		{`type R = R{a: Int}; func f() R { R{a: 1, a: 2} }`, "duplicate field a in constructor R"},
		{`type R = R{a: Int}; func f() R { R{b: 1} }`, "unknown field b in constructor R"},
		{`type R = R{a: Int}; func f() R { R{a: true} }`, "type mismatch: expected Int, got Bool"},
		{`type R = R(Int); func f() R { R{a: 1} }`, "constructor R is not a record"},
		{`type R = R{a: Int} | Empty; func f(x R) Int { x.a }`, "field a requires a single-constructor record type"},
		{`type R = R{a: Int} | Empty; func f(x R) R { x{a: 1} }`, "field a requires a single-constructor record type"},
		{`func f(x Int) Int { x.a }`, "field a requires a single-constructor record type"},
		{`func f(x Int) Int { x{a: 1} }`, "field a requires a single-constructor record type"},
		{`type R = R{a: Int}; func f(x R) Int { x.b }`, "field b requires a single-constructor record type"},
		{`type R = R(Int); func f(x R) Int { x.a }`, "field a requires a single-constructor record type"},
		{`func f() Unit { let get = func(x) => x.a; unit() }`, "cannot infer record type of x.a; annotate"},
		{`func f() Unit { let set = func(x) => x{a: 1}; unit() }`, "cannot infer record type of x.a; annotate"},
		{`type R = R{a: Int}; func f(x R) R { x{a: 1, a: 2} }`, "duplicate field a in constructor R"},
		{`type R = R{a: Int}; func f(x R) R { x{a: true} }`, "type mismatch: expected Int, got Bool"},
	} {
		t.Run(tt.source, func(t *testing.T) {
			p, err := types.Check(parseFiles(t, tt.source+pass5Main))
			var typed *types.Error
			if p != nil || !errors.As(err, &typed) || typed.Msg != tt.want {
				t.Fatalf("Check = (%v, %v), want %q", p, err, tt.want)
			}
		})
	}
}

func TestInterpolationDictionaries(t *testing.T) {
	files := parseFiles(t, pass5Show+`
type Label = Label(String)
instance Show[Label] { show(x) { match x { Label(s) => s } } }
func value() String { "int ${1}, string ${"text"}, user ${Label("name")}" }
func generic[A Show](a A) String { "${a}" }
func local() String { let render = func(x) => "${x}"; render(1) + render(true) }
`+pass5Main)
	p, err := types.Check(files)
	if err != nil {
		t.Fatal(err)
	}
	ann := types.AnnOf(p.Funcs["value"].Decl.Body.Result)
	if ann.Type != types.String || len(ann.ShowDicts) != 3 {
		t.Fatalf("interpolation annotation = %#v", ann)
	}
	for i, con := range []string{"Int", "String", "Label"} {
		assertInstanceDict(t, ann.ShowDicts[i], "Show", con)
	}
	ann = types.AnnOf(p.Funcs["generic"].Decl.Body.Result)
	if len(ann.ShowDicts) != 1 {
		t.Fatalf("generic interpolation dictionaries = %#v", ann.ShowDicts)
	}
	assertParamDict(t, ann.ShowDicts[0], "Show", "A")
	walkExprs(reflect.ValueOf(files), func(e ast.Expr) { assertZonked(t, e) })
}

func TestInterpolationMissingInstances(t *testing.T) {
	for _, tt := range []struct{ source, want string }{
		{`type Hidden = Hidden; func f() String { "${Hidden}" }`, "no instance Show[Hidden]; hint: write 'instance Show[Hidden]' (or 'deriving Debug' for a structural form)"},
		{`func f[A](x A) String { "${x}" }`, "no instance Show[A] (type parameter A has no class constraints)"},
		{`func f() String { "${unit()}" }`, "no instance Show[Unit]"},
	} {
		t.Run(tt.source, func(t *testing.T) {
			files := parseFiles(t, tt.source+pass5Main)
			p, err := types.Check(files)
			var typed *types.Error
			if p != nil || !errors.As(err, &typed) || typed.Msg != tt.want {
				t.Fatalf("Check = (%v, %v), want %q", p, err, tt.want)
			}
			fn := files[1].Decls[len(files[1].Decls)-2].(*ast.FuncDecl)
			expr := fn.Body.Result.(*ast.Interp).Exprs[0]
			if typed.Pos != expr.GetPos() {
				t.Fatalf("error position = %s, want %s", typed.Pos, expr.GetPos())
			}
		})
	}
}

func TestDiagnosticFileNames(t *testing.T) {
	files := parseFiles(t, `func value() Unit { None; []; unit() }`+pass5Main)
	files[0].Name, files[1].Name = "prelude.gfx", "user.gfx"
	p, err := types.Check(files)
	if err != nil {
		t.Fatal(err)
	}
	if len(p.Warnings) != 2 {
		t.Fatalf("warnings = %v", p.Warnings)
	}
	for _, warning := range p.Warnings {
		if warning.File != "user.gfx" {
			t.Fatalf("warning file = %q", warning.File)
		}
	}
	first := p.Funcs["value"].Decl.Body.Stmts[0].(*ast.ExprStmt).X
	second := p.Funcs["value"].Decl.Body.Stmts[1].(*ast.ExprStmt).X
	if p.Warnings[0].Pos != first.GetPos() || p.Warnings[1].Pos != second.GetPos() {
		t.Fatalf("warning positions = %v", p.Warnings)
	}
	files[0], files[1] = files[1], files[0]
	files[0].Name, files[1].Name = "prelude.gfx", "user.gfx"
	p, err = types.Check(files)
	if err != nil {
		t.Fatal(err)
	}
	for _, warning := range p.Warnings {
		if warning.File != "prelude.gfx" {
			t.Fatalf("prelude warning file = %q", warning.File)
		}
	}
	files = parseFiles(t, `func bad() String { 1 }`+pass5Main)
	files[0].Name, files[1].Name = "prelude.gfx", "user.gfx"
	_, err = types.Check(files)
	var typed *types.Error
	if !errors.As(err, &typed) || typed.File != "user.gfx" {
		t.Fatalf("error file = %v", err)
	}
	// Place a source with the same diagnostic in the first, prelude-named file.
	files[0], files[1] = files[1], files[0]
	files[0].Name, files[1].Name = "prelude.gfx", "user.gfx"
	_, err = types.Check(files)
	if !errors.As(err, &typed) || typed.File != "prelude.gfx" {
		t.Fatalf("prelude error file = %v", err)
	}
}

func TestNestedConstrainedLets(t *testing.T) {
	for _, source := range []string{
		`func value() String { let outer = func(x) => { let inner = func(y) => show(y); inner(x) }; outer(1) + outer(true) }`,
		`func value() String { let outer = func(x) => { let inner = func(_y) => show(x); inner(1) }; outer(1) + outer(true) }`,
		`func value() IO[String] { do { let render = func(x) => show(x); ioPure(render(1)); ioPure(render(true)) } }`,
		`func value() String { let render = Some(func(x) => show(x)); match render { Some(f) => f(1); None => "" } }`,
		`func value() String { let render = func(x) => show(Some(x)); render(1) + render(true) }`,
	} {
		t.Run(source, func(t *testing.T) {
			files := parseFiles(t, pass5Show+showOptionInstance+source+pass5Main)
			p, err := types.Check(files)
			if err != nil {
				t.Fatal(err)
			}
			if len(p.Warnings) != 0 {
				t.Fatalf("unexpected warnings: %v", p.Warnings)
			}
			walkExprs(reflect.ValueOf(files), func(e ast.Expr) { assertZonked(t, e) })
		})
	}
}

func TestMonomorphicLetAnnotations(t *testing.T) {
	files := parseFiles(t, `func identity[A](x A) A { x }
func value() Int { let f = identity(func(x) => x); f(1) }`+pass5Main)
	p, err := types.Check(files)
	if err != nil {
		t.Fatal(err)
	}
	body := p.Funcs["value"].Decl.Body
	value := body.Stmts[0].(*ast.LetStmt).Value
	use := body.Result.(*ast.Call).Fn
	if types.AnnOf(value).LetScheme != nil || len(types.AnnOf(use).Inst) != 0 || len(types.AnnOf(use).Dicts) != 0 {
		t.Fatal("monomorphic let has scheme annotations")
	}
}

func TestGeneralizedConstraintCapturesEnvironment(t *testing.T) {
	files := parseFiles(t, `func value() String {
 (func(captured) => {
  let render = func(x) => { show(x); captured }
  (func(s String) => s)(render(1))
 })("text")
 }`+pass5Main)
	p, err := types.Check(files)
	if err != nil {
		t.Fatal(err)
	}
	walkExprs(reflect.ValueOf(p.Files), func(e ast.Expr) { assertZonked(t, e) })
}
