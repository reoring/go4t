package types_test

import (
	"errors"
	"reflect"
	"testing"

	"gofx/ast"
	"gofx/parser"
	"gofx/types"
)

// Keep operator tests independent of the shipped prelude while exercising the
// same classes, primitive implementations and recursive dictionary contracts.
const pass9Prelude = `
class Eq[A] { eq(a A, b A) Bool }
type Ordering = LT | EQ | GT
class Ord[A] : Eq[A] { compare(a A, b A) Ordering }
extern func eqInt(a Int, b Int) Bool = "gofx/rt".EqInt
extern func ltInt(a Int, b Int) Bool = "gofx/rt".LtInt
extern func eqString(a String, b String) Bool = "gofx/rt".EqString
extern func ltString(a String, b String) Bool = "gofx/rt".LtString
extern func eqBool(a Bool, b Bool) Bool = "gofx/rt".EqBool
instance Eq[Int] { eq(a, b) { eqInt(a, b) } }
instance Ord[Int] { compare(a, b) { if eqInt(a, b) { EQ } else if ltInt(a, b) { LT } else { GT } } }
instance Eq[String] { eq(a, b) { eqString(a, b) } }
instance Ord[String] { compare(a, b) { if eqString(a, b) { EQ } else if ltString(a, b) { LT } else { GT } } }
instance Eq[Bool] { eq(a, b) { eqBool(a, b) } }
instance Ord[Bool] { compare(a, b) { if eqBool(a, b) { EQ } else if a { GT } else { LT } } }
instance Eq[Unit] { eq(a, b) { true } }
`

const pass9OptionInstances = `
instance Eq[Option[A]] given Eq[A] {
 eq(a, b) { match (a, b) { (None, None) => true; (Some(x), Some(y)) => x == y; _ => false } }
}
instance Ord[Option[A]] given Ord[A] {
 compare(a, b) { match (a, b) { (None, None) => EQ; (None, Some(_)) => LT; (Some(_), None) => GT; (Some(x), Some(y)) => compare(x, y) } }
}
`

func TestAnnotatedLets(t *testing.T) {
	for _, source := range []string{
		`func value() Unit { let _xs: List[Int] = []; unit() }`,
		`func value() IO[Unit] { do { let _xs: List[Int] = []; ioPure(unit()) } }`,
		`func value[A](a A) List[A] { let xs: List[A] = []; xs }`,
		`func value[A](a A) IO[A] { do { let x: A = a; ioPure(x) } }`,
		`type Entry = Entry{n: Int}; func value() Int { let get: func(Entry) Int = func(e) => e.n; get(Entry(3)) }`,
	} {
		t.Run(source, func(t *testing.T) {
			files := parseFiles(t, source+pass5Main)
			for range 2 {
				p, err := checkFixture(t, []*ast.Module{{Files: files}})
				if err != nil {
					t.Fatal(err)
				}
				if len(p.Warnings) != 0 {
					t.Fatalf("annotated let warned: %v", p.Warnings)
				}
				walkExprs(reflect.ValueOf(p.Funcs["value"].Decl.Body), func(e ast.Expr) {
					assertZonked(t, e)
					ann := types.AnnOf(e)
					if ann.LetScheme != nil {
						t.Fatalf("annotated let generalized: %#v", ann.LetScheme)
					}
					if list, ok := e.(*ast.ListLit); ok {
						item := types.AnnOf(list).Type.(*types.TApp).Args[0]
						want := types.Type(types.Int)
						if params := p.Funcs["value"].Scheme.TParams; len(params) != 0 {
							want = params[0]
						}
						if item != want {
							t.Fatalf("list element = %s, want %s", types.TypeString(item), types.TypeString(want))
						}
					}
				})
			}
		})
	}
}

func TestPass9Diagnostics(t *testing.T) {
	for _, tt := range []struct{ source, want string }{
		{`func value() Unit { let x: String = 1; unit() }`, "type mismatch: expected String, got Int"},
		{`func value() IO[Unit] { do { let x: String = 1; ioPure(unit()) } }`, "type mismatch: expected String, got Int"},
		{`func value[A](a A) A { let x: A = 1; x }`, "type mismatch: expected A, got Int"},
		{`func value() Unit { let id: func(Int) Int = func(x) => x; id(true); unit() }`, "type mismatch: expected Int, got Bool"},
		{`func value() List[String] { let xs: List[Int] = []; xs }`, "type mismatch: expected List[String], got List[Int]"},
		{`func value[A Show](x A) Bool { x == x }`, "no instance Eq[A] (type parameter A is constrained only by Show)"},
		{`func value[A](x A) Bool { x == x }`, "no instance Eq[A] (type parameter A has no class constraints)"},
		{`func value[A Eq](x A) Bool { x < x }`, "no instance Ord[A] (type parameter A is constrained only by Eq)"},
		{`func value() Int { (func(Some(x)) => x)(Some(1)) }`, "lambda parameter pattern must be irrefutable"},
		{`func value() Int { (func(1) => 2)(1) }`, "lambda parameter pattern must be irrefutable"},
		{`func value() Int { (func((x, Some(y))) => x + y)((1, Some(2))) }`, "lambda parameter pattern must be irrefutable"},
		{`type Shape = Dot{x: Int} | Empty; func value() Int { (func(Dot{x: x}) => x)(Dot(1)) }`, "lambda parameter pattern must be irrefutable"},
		{`func value() Int { (func((x, x)) => x)((1, 2)) }`, "duplicate pattern variable x"},
		{`func value() Int { (func((x, y), x) => y)((1, 2), 3) }`, "duplicate parameter x"},
		{`func value(x Option[Int]) Int { match x { Some(n) if n => n; _ => 0 } }`, "type mismatch: expected Bool, got Int"},
		{`func value(x Int) Int { match x { n if true => n } }`, "non-exhaustive match; missing: _"},
		{`func value(x Option[Int]) Int { match x { None => 0; Some(n) if true => n } }`, "non-exhaustive match; missing: Some(_)"},
		{`func value(x Option[Int]) Int { match x { Some(n) => n; _ if true => 0 } }`, "non-exhaustive match; missing: None"},
	} {
		t.Run(tt.source, func(t *testing.T) {
			p, err := checkFixture(t, []*ast.Module{{Files: parseFiles(t, tt.source+pass5Main)}})
			var typed *types.Error
			if p != nil || !errors.As(err, &typed) || typed.Msg != tt.want || typed.Pos.Line == 0 || typed.Pos.Col == 0 {
				t.Fatalf("Check = (%v, %v), want positioned %q", p, err, tt.want)
			}
		})
	}
}

func TestComparisonDictionaries(t *testing.T) {
	files := parseFiles(t, pass9OptionInstances+`
type Token = Token(Int)
instance Eq[Token] { eq(a, b) { match (a, b) { (Token(x), Token(y)) => x == y } } }
func user() Bool { Token(1) == Token(2) }
func nested() Bool { Some(1) < Some(2) }
func generic[A Eq](a A, b A) Bool { a != b }
func superclass[A Ord](a A, b A) Bool { a == b }
func ordered[A Ord](a A, b A) Bool { a >= b }
`+pass5Main)
	for range 2 {
		p, err := checkFixture(t, []*ast.Module{{Files: files}})
		if err != nil {
			t.Fatal(err)
		}
		for _, tt := range []struct{ name, class, operand string }{
			{"user", "Eq", "Token"}, {"nested", "Ord", "Option[Int]"},
			{"generic", "Eq", "A"}, {"superclass", "Eq", "A"}, {"ordered", "Ord", "A"},
		} {
			ann := types.AnnOf(p.Funcs[tt.name].Decl.Body.Result)
			if ann.Type != types.Bool || len(ann.Inst) != 1 || types.TypeString(ann.Inst[0]) != tt.operand || len(ann.Dicts) != 1 || ann.Dicts[0].Class != tt.class {
				t.Fatalf("%s binary annotation = %#v", tt.name, ann)
			}
		}
		user := types.AnnOf(p.Funcs["user"].Decl.Body.Result).Dicts[0]
		assertInstanceDict(t, user, "Eq", "Token")
		if user.Instance.Decl != files[1].Decls[3] {
			t.Fatal("user equality did not resolve to the declared instance")
		}
		nested := types.AnnOf(p.Funcs["nested"].Decl.Body.Result).Dicts[0]
		assertInstanceDict(t, nested, "Ord", "Option")
		if len(nested.Args) != 1 {
			t.Fatalf("nested Ord dictionary = %#v", nested)
		}
		assertInstanceDict(t, nested.Args[0], "Ord", "Int")
		assertParamDict(t, types.AnnOf(p.Funcs["generic"].Decl.Body.Result).Dicts[0], "Eq", "A")
		assertParamDict(t, types.AnnOf(p.Funcs["ordered"].Decl.Body.Result).Dicts[0], "Ord", "A")
		super := types.AnnOf(p.Funcs["superclass"].Decl.Body.Result).Dicts[0]
		if super.Kind != types.DictSuper || super.Super == nil {
			t.Fatalf("Eq superclass dictionary = %#v", super)
		}
		assertParamDict(t, *super.Super, "Ord", "A")
		walkExprs(reflect.ValueOf(files), func(e ast.Expr) { assertZonked(t, e) })
	}
}

func TestPrimitiveComparisonDictionaries(t *testing.T) {
	for _, op := range []string{"==", "!=", "<", "<=", ">", ">="} {
		for _, operands := range []struct{ left, right, con string }{
			{"1", "2", "Int"}, {`"a"`, `"b"`, "String"}, {"false", "true", "Bool"},
		} {
			t.Run(operands.con+op, func(t *testing.T) {
				p, err := checkFixture(t, []*ast.Module{{Files: parseFiles(t, "func value() Bool { "+operands.left+op+operands.right+" }"+pass5Main)}})
				if err != nil {
					t.Fatal(err)
				}
				ann := types.AnnOf(p.Funcs["value"].Decl.Body.Result)
				class := "Ord"
				if op == "==" || op == "!=" {
					class = "Eq"
				}
				if len(ann.Dicts) != 1 || len(ann.Inst) != 1 || types.TypeString(ann.Inst[0]) != operands.con {
					t.Fatalf("comparison annotation = %#v", ann)
				}
				assertInstanceDict(t, ann.Dicts[0], class, operands.con)
			})
		}
	}
}

func TestComparisonDefaulting(t *testing.T) {
	p, err := checkFixture(t, []*ast.Module{{Files: parseFiles(t, `func value() Unit { func(x) => x == x; unit() }`+pass5Main)}})
	if err != nil {
		t.Fatal(err)
	}
	if len(p.Warnings) != 1 || p.Warnings[0].Msg != "type of Lambda defaulted to Unit; add an annotation or use the value" {
		t.Fatalf("comparison warnings = %v", p.Warnings)
	}
	lambda := p.Funcs["value"].Decl.Body.Stmts[0].(*ast.ExprStmt).X.(*ast.Lambda)
	ann := types.AnnOf(lambda.Body)
	if len(ann.Inst) != 1 || ann.Inst[0] != types.Unit || len(ann.Dicts) != 1 {
		t.Fatalf("defaulted comparison = %#v", ann)
	}
	assertInstanceDict(t, ann.Dicts[0], "Eq", "Unit")
}

func TestComparisonLetGeneralization(t *testing.T) {
	p, err := checkFixture(t, []*ast.Module{{Files: parseFiles(t, `func value() Bool { let same = func(x) => x == x; same(1) && same(true) }`+pass5Main)}})
	if err != nil {
		t.Fatal(err)
	}
	body := p.Funcs["value"].Decl.Body
	lambda := body.Stmts[0].(*ast.LetStmt).Value.(*ast.Lambda)
	scheme := types.AnnOf(lambda).LetScheme
	if scheme == nil || len(scheme.TParams) != 1 || len(scheme.Constraints) != 1 || scheme.Constraints[0].Class != "Eq" {
		t.Fatalf("comparison let scheme = %#v", scheme)
	}
	assertParamDict(t, types.AnnOf(lambda.Body).Dicts[0], "Eq", scheme.TParams[0].Name)
	binary := body.Result.(*ast.Binary)
	for i, operand := range []ast.Expr{binary.L, binary.R} {
		ann := types.AnnOf(operand.(*ast.Call).Fn)
		if len(ann.Dicts) != 1 {
			t.Fatalf("comparison let use = %#v", ann)
		}
		assertInstanceDict(t, ann.Dicts[0], "Eq", []string{"Int", "Bool"}[i])
	}
}

func TestComparisonRequiresPrelude(t *testing.T) {
	for _, op := range []string{"==", "!=", "<", "<=", ">", ">="} {
		t.Run(op, func(t *testing.T) {
			file, err := parser.Parse(`package main
extern func main() IO[Unit] = "example".Main
func value() Bool { 1 ` + op + ` 2 }
`)
			if err != nil {
				t.Fatal(err)
			}
			p, err := checkFixture(t, []*ast.Module{{Files: []*ast.File{file}}})
			class := "Ord"
			if op == "==" || op == "!=" {
				class = "Eq"
			}
			var typed *types.Error
			want := "operator " + op + " requires prelude " + class
			if p != nil || !errors.As(err, &typed) || typed.Msg != want {
				t.Fatalf("Check = (%v, %v), want %q", p, err, want)
			}
		})
	}
}

func TestLambdaParameterPatterns(t *testing.T) {
	for _, source := range []string{
		`func value() Int { (func((a, b)) => a + b)((1, 2)) }`,
		`func value() Int { (func(_) => 1)(true) }`,
		`func value() Int { (func(_, _) => 1)(true, "ignored") }`,
		`func value() Int { (func(Entry{n: n, ..}) => n)(Entry(2, "name")) }`,
		`func value() String { (func(Entry(_n, s)) => s)(Entry(2, "name")) }`,
		`func value() Entry { (func(e @ Entry{..}) => e)(Entry(2, "name")) }`,
		`func value() (Int, String) { let first = func((a, _)) => a; (first((1, false)), first(("s", 2))) }`,
		`func apply(f func((Entry, Int)) Int) Int { f((Entry(2, "name"), 3)) }; func value() Int { apply(func((e, n)) => e.n + n) }`,
		`func value() Int { let get: func((Entry, Int)) Int = func((e, n)) => e.n + n; get((Entry(2, "name"), 3)) }`,
	} {
		t.Run(source, func(t *testing.T) {
			files := parseFiles(t, "type Entry = Entry{n: Int, label: String}\n"+source+pass5Main)
			for range 2 {
				p, err := checkFixture(t, []*ast.Module{{Files: files}})
				if err != nil {
					t.Fatal(err)
				}
				if len(p.Warnings) != 0 {
					t.Fatalf("lambda pattern warnings = %v", p.Warnings)
				}
				for _, resolved := range p.RecordPats {
					if resolved.Ctor != p.DataTypes["Entry"].Ctors[0] {
						t.Fatalf("lambda record resolution = %#v", resolved)
					}
				}
				walkExprs(reflect.ValueOf(files), func(e ast.Expr) { assertZonked(t, e) })
			}
		})
	}
}

func TestMatchGuards(t *testing.T) {
	for _, source := range []string{
		`func value(x Option[Int]) Int { match x { Some(n) if n > 0 => n; Some(n) => -n; None => 0 } }`,
		`func value(x Int) Int { match x { 1 if false => 2; n => n; _ if true => 0 } }`,
		`type Entry = Entry{n: Int}; func value(x Entry) Int { match x { Entry{n: n} if n == 1 => n; Entry{n: n} => n } }`,
	} {
		t.Run(source, func(t *testing.T) {
			files := parseFiles(t, source+pass5Main)
			p, err := checkFixture(t, []*ast.Module{{Files: files}})
			if err != nil {
				t.Fatal(err)
			}
			match := p.Funcs["value"].Decl.Body.Result.(*ast.Match)
			if match.Arms[0].Guard == nil || types.AnnOf(match.Arms[0].Guard).Type != types.Bool {
				t.Fatal("guard lacks Bool annotation")
			}
			walkExprs(reflect.ValueOf(files), func(e ast.Expr) { assertZonked(t, e) })
		})
	}
}

func TestSuperclassContextProjection(t *testing.T) {
	file, err := parser.Parse(`package main
class Eq[A] {}
class Ord[A] : Eq[A] {}
class Total[A] : Ord[A] {}
class Parent[A] {}
class Child[A] : Parent[A] {}
type Pair[A, B] = Pair(A, B)
instance Parent[Pair[A, B]] given Eq[A], Eq[B] {}
instance Child[Pair[X, Y]] given Total[Y], Ord[X] {}
extern func main() IO[Unit] = "example".Main
`)
	if err != nil {
		t.Fatal(err)
	}
	for range 2 {
		p, err := checkFixture(t, []*ast.Module{{Files: []*ast.File{file}}})
		if err != nil {
			t.Fatal(err)
		}
		child := p.Instances[1]
		if len(child.SuperArgs) != 2 {
			t.Fatalf("superclass arguments = %#v", child.SuperArgs)
		}
		for i, tt := range []struct {
			param, bound string
			hops         []string
		}{
			{"X", "Ord", []string{"Eq"}},
			{"Y", "Total", []string{"Eq", "Ord"}},
		} {
			dict := child.SuperArgs[i]
			for _, class := range tt.hops {
				if dict.Kind != types.DictSuper || dict.Class != class || dict.Super == nil {
					t.Fatalf("superclass projection %d: %#v, want %s", i, dict, class)
				}
				dict = *dict.Super
			}
			assertParamDict(t, dict, tt.bound, tt.param)
		}
	}
}
