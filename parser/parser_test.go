package parser_test

import (
	"fmt"
	"reflect"
	"strings"
	"testing"

	"gofx/ast"
	"gofx/parser"
)

func parse(t *testing.T, src string) *ast.File {
	t.Helper()
	f, err := parser.Parse(src)
	if err != nil {
		t.Fatal(err)
	}
	return f
}

func result(t *testing.T, src string) ast.Expr {
	t.Helper()
	f := parse(t, "package main\nfunc test() Int { "+src+" }")
	return f.Decls[0].(*ast.FuncDecl).Body.Result
}

func typeName(t *testing.T, typ ast.Type, name string, args ...string) {
	t.Helper()
	n, ok := typ.(*ast.TypeName)
	if !ok {
		t.Fatalf("type = %T, want TypeName", typ)
	}
	if n.Name != name || len(n.Args) != len(args) {
		t.Fatalf("type = %#v, want %s%v", n, name, args)
	}
	for i, arg := range args {
		typeName(t, n.Args[i], arg)
	}
}

func TestDesignSurfaceDeclarations(t *testing.T) {
	f := parse(t, `package main
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
func twice[F[_] Functor, A](fa F[A], f func(A) A) F[A] { fmap(fmap(fa, f), f) }
extern func httpGet(url String) IO[Int] = "gofx/rt/stdhttp".GetStatus
func main() IO[Unit] {
 do {
  code <- httpGet("https://example.com")
  println(show(code))
 }
}
`)
	if f.Package != "main" || len(f.Decls) != 7 {
		t.Fatalf("file = %#v", f)
	}
	functor := f.Decls[0].(*ast.ClassDecl)
	if functor.Name != "Functor" || functor.Param.Name != "F" || functor.Param.Arity != 1 || functor.Super != "" || len(functor.Methods) != 1 {
		t.Fatalf("Functor = %#v", functor)
	}
	method := functor.Methods[0]
	if method.Name != "fmap" || len(method.TParams) != 2 || method.TParams[0].Name != "A" || method.TParams[1].Name != "B" || len(method.Params) != 2 {
		t.Fatalf("method = %#v", method)
	}
	typeName(t, method.Params[0].Type, "F", "A")
	ft := method.Params[1].Type.(*ast.FuncType)
	if len(ft.Params) != 1 {
		t.Fatalf("function type = %#v", ft)
	}
	typeName(t, ft.Params[0], "A")
	typeName(t, ft.Ret, "B")
	typeName(t, method.Ret, "F", "B")
	monad := f.Decls[1].(*ast.ClassDecl)
	if monad.Name != "Monad" || monad.Param.Name != "M" || monad.Param.Arity != 1 || monad.Super != "Functor" || len(monad.Methods) != 2 {
		t.Fatalf("Monad = %#v", monad)
	}
	option := f.Decls[2].(*ast.TypeDecl)
	if option.Name != "Option" || !reflect.DeepEqual(option.TParams, []string{"A"}) || len(option.Ctors) != 2 || option.Ctors[0].Name != "None" || len(option.Ctors[0].Fields) != 0 || option.Ctors[1].Name != "Some" {
		t.Fatalf("Option = %#v", option)
	}
	typeName(t, option.Ctors[1].Fields[0], "A")
	instance := f.Decls[3].(*ast.InstanceDecl)
	if instance.Class != "Functor" || instance.TypeCon != "Option" || len(instance.Methods) != 1 || instance.Methods[0].Name != "fmap" || !reflect.DeepEqual(instance.Methods[0].Params, []string{"fa", "f"}) {
		t.Fatalf("instance = %#v", instance)
	}
	if m := instance.Methods[0].Body.Result.(*ast.Match); len(m.Arms) != 2 {
		t.Fatalf("match = %#v", m)
	}
	twice := f.Decls[4].(*ast.FuncDecl)
	if twice.Name != "twice" || !reflect.DeepEqual(twice.TParams, []ast.TypeParam{{Name: "F", Arity: 1, Constraints: []string{"Functor"}}, {Name: "A"}}) {
		t.Fatalf("twice = %#v", twice)
	}
	typeName(t, twice.Ret, "F", "A")
	ext := f.Decls[5].(*ast.FuncDecl)
	if ext.Name != "httpGet" || ext.Body != nil || ext.Extern == nil || *ext.Extern != (ast.ExternRef{ImportPath: "gofx/rt/stdhttp", Name: "GetStatus"}) {
		t.Fatalf("extern = %#v", ext)
	}
	typeName(t, ext.Params[0].Type, "String")
	typeName(t, ext.Ret, "IO", "Int")
	main := f.Decls[6].(*ast.FuncDecl)
	typeName(t, main.Ret, "IO", "Unit")
	d := main.Body.Result.(*ast.Do)
	if len(d.Stmts) != 1 || d.Stmts[0].(*ast.DoBind).Name != "code" {
		t.Fatalf("do = %#v", d)
	}
	if got := exprShape(d.Result); got != "println(show(code))" {
		t.Errorf("do result = %s", got)
	}
}

func TestDeclarationVariants(t *testing.T) {
	f := parse(t, `package other;
type Color = Red | RGB(Int, Int, Int)
type Pair[A, B] = Pair(A, B)
class Bifunctor[F[_,_]] { map[A](f F[A, A]) F[A, A] }
instance Bifunctor[Pair] { make() { Pair(1, 2) }; identity(x) { x } }
extern func pure[A](x A) IO[A] = "gofx/rt".Pure
func consume[F[_] Functor Monad, A](x F[A]) func(A, A) A { func(a, b) => a }
`)
	if f.Package != "other" || len(f.Decls) != 6 {
		t.Fatalf("file = %#v", f)
	}
	color := f.Decls[0].(*ast.TypeDecl)
	if color.Name != "Color" || len(color.TParams) != 0 || len(color.Ctors) != 2 || len(color.Ctors[1].Fields) != 3 {
		t.Fatalf("Color = %#v", color)
	}
	for _, field := range color.Ctors[1].Fields {
		typeName(t, field, "Int")
	}
	pair := f.Decls[1].(*ast.TypeDecl)
	if !reflect.DeepEqual(pair.TParams, []string{"A", "B"}) {
		t.Fatalf("Pair = %#v", pair)
	}
	if c := f.Decls[2].(*ast.ClassDecl); c.Param.Arity != 2 {
		t.Errorf("HKT arity = %d", c.Param.Arity)
	}
	if i := f.Decls[3].(*ast.InstanceDecl); len(i.Methods) != 2 || len(i.Methods[0].Params) != 0 {
		t.Fatalf("instance = %#v", i)
	}
	if e := f.Decls[4].(*ast.FuncDecl); len(e.TParams) != 1 || e.TParams[0].Name != "A" {
		t.Fatalf("extern = %#v", e)
	}
	consume := f.Decls[5].(*ast.FuncDecl)
	if !reflect.DeepEqual(consume.TParams[0].Constraints, []string{"Functor", "Monad"}) {
		t.Fatalf("constraints = %v", consume.TParams[0].Constraints)
	}
	ft := consume.Ret.(*ast.FuncType)
	if len(ft.Params) != 2 {
		t.Fatalf("return function type = %#v", ft)
	}
}

func TestLambdas(t *testing.T) {
	for _, tc := range []struct {
		src   string
		typed bool
	}{
		{"func(x, y) => x + y", false},
		{"func(x Int, y Int) => x + y", true},
	} {
		t.Run(tc.src, func(t *testing.T) {
			l := result(t, tc.src).(*ast.Lambda)
			if len(l.Params) != 2 || l.Params[0].Name != "x" || l.Params[1].Name != "y" {
				t.Fatalf("lambda = %#v", l)
			}
			for _, p := range l.Params {
				if tc.typed {
					typeName(t, p.Type, "Int")
				} else if p.Type != nil {
					t.Errorf("untyped param has type %v", p.Type)
				}
			}
			if got := exprShape(l.Body); got != "(x + y)" {
				t.Errorf("body = %s", got)
			}
		})
	}
	if l := result(t, "func() => 1").(*ast.Lambda); len(l.Params) != 0 {
		t.Errorf("params = %v", l.Params)
	}
}

func TestDoStatementsAndResult(t *testing.T) {
	d := result(t, `do {
 x <-
  read()
 let y = x + 1
 print(y)
 pure(y)
}`).(*ast.Do)
	if len(d.Stmts) != 3 {
		t.Fatalf("statements = %#v", d.Stmts)
	}
	bind := d.Stmts[0].(*ast.DoBind)
	if bind.Name != "x" || exprShape(bind.X) != "read()" {
		t.Fatalf("bind = %#v", bind)
	}
	let := d.Stmts[1].(*ast.DoLet)
	if let.Name != "y" || exprShape(let.Value) != "(x + 1)" {
		t.Fatalf("let = %#v", let)
	}
	if got := exprShape(d.Stmts[2].(*ast.DoExpr).X); got != "print(y)" {
		t.Errorf("expr = %s", got)
	}
	if got := exprShape(d.Result); got != "pure(y)" {
		t.Errorf("result = %s", got)
	}
}

func TestMatchPatterns(t *testing.T) {
	m := result(t, `match value {
 Some(Pair(1, _)) => 1
 None => 2
 "text" => 3
 true => 4
 false => 5
 other => other
 _ => 0
}`).(*ast.Match)
	if exprShape(m.Scrut) != "value" || len(m.Arms) != 7 {
		t.Fatalf("match = %#v", m)
	}
	some := m.Arms[0].Pat.(*ast.PCtor)
	if some.Name != "Some" || len(some.Args) != 1 {
		t.Fatalf("Some = %#v", some)
	}
	pair := some.Args[0].(*ast.PCtor)
	if pair.Name != "Pair" || len(pair.Args) != 2 || pair.Args[0].(*ast.PLit).X.(*ast.IntLit).Value != 1 {
		t.Fatalf("Pair = %#v", pair)
	}
	if _, ok := pair.Args[1].(*ast.PWild); !ok {
		t.Fatalf("nested wildcard = %T", pair.Args[1])
	}
	if c := m.Arms[1].Pat.(*ast.PCtor); c.Name != "None" || len(c.Args) != 0 {
		t.Fatalf("None = %#v", c)
	}
	if s := m.Arms[2].Pat.(*ast.PLit).X.(*ast.StringLit); s.Value != "text" {
		t.Errorf("literal = %q", s.Value)
	}
	if !m.Arms[3].Pat.(*ast.PLit).X.(*ast.BoolLit).Value || m.Arms[4].Pat.(*ast.PLit).X.(*ast.BoolLit).Value {
		t.Error("boolean pattern values incorrect")
	}
	if v := m.Arms[5].Pat.(*ast.PVar); v.Name != "other" {
		t.Errorf("variable = %s", v.Name)
	}
	if _, ok := m.Arms[6].Pat.(*ast.PWild); !ok {
		t.Errorf("wildcard = %T", m.Arms[6].Pat)
	}
}

func TestMatchArmSeparators(t *testing.T) {
	for _, tc := range []struct{ name, src string }{
		{"leading pipe", "match o { | Some(n) => n | None => 0 }"},
		{"inline pipe", "match o { Some(n) => n | None => 0 }"},
		{"mixed newline and pipe", `match o {
 ; | Some(n) => n;
 ; | ;
 None => 0
}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m := result(t, tc.src).(*ast.Match)
			if exprShape(m.Scrut) != "o" || len(m.Arms) != 2 {
				t.Fatalf("match = %#v", m)
			}
			some := m.Arms[0].Pat.(*ast.PCtor)
			if some.Name != "Some" || len(some.Args) != 1 || some.Args[0].(*ast.PVar).Name != "n" {
				t.Fatalf("Some = %#v", some)
			}
			if got := exprShape(m.Arms[0].Body); got != "n" {
				t.Errorf("first body = %s, want n", got)
			}
			none := m.Arms[1].Pat.(*ast.PCtor)
			if none.Name != "None" || len(none.Args) != 0 {
				t.Fatalf("None = %#v", none)
			}
			if got := exprShape(m.Arms[1].Body); got != "0" {
				t.Errorf("second body = %s, want 0", got)
			}
		})
	}
}

func TestMatchArmLogicalOrIsNotSeparator(t *testing.T) {
	m := result(t, "match o { Some(a) => a || b | None => false }").(*ast.Match)
	if len(m.Arms) != 2 {
		t.Fatalf("arms = %#v, want two arms", m.Arms)
	}
	if got := exprShape(m.Arms[0].Body); got != "(a || b)" {
		t.Errorf("first body = %s, want (a || b)", got)
	}
	if none := m.Arms[1].Pat.(*ast.PCtor); none.Name != "None" {
		t.Errorf("second pattern = %#v, want None", none)
	}
	if body, ok := m.Arms[1].Body.(*ast.BoolLit); !ok || body.Value {
		t.Errorf("second body = %#v, want false", m.Arms[1].Body)
	}
}

func TestDataTypeConstructorPipeSeparator(t *testing.T) {
	f := parse(t, "package main\ntype T = A | B")
	if len(f.Decls) != 1 {
		t.Fatalf("declarations = %#v, want one", f.Decls)
	}
	d := f.Decls[0].(*ast.TypeDecl)
	if d.Name != "T" || len(d.Ctors) != 2 || d.Ctors[0].Name != "A" || d.Ctors[1].Name != "B" {
		t.Fatalf("type = %#v, want T with constructors A and B", d)
	}
}

func TestIfElseIfElse(t *testing.T) {
	e := result(t, "if a { 1 } else if b { 2 } else { 3 }").(*ast.If)
	if exprShape(e.Cond) != "a" || exprShape(e.Then.(*ast.BlockExpr).Block.Result) != "1" {
		t.Fatalf("if = %#v", e)
	}
	nested := e.Else.(*ast.If)
	if exprShape(nested.Cond) != "b" || exprShape(nested.Then.(*ast.BlockExpr).Block.Result) != "2" || exprShape(nested.Else.(*ast.BlockExpr).Block.Result) != "3" {
		t.Fatalf("else if = %#v", nested)
	}
}

func TestExpressionPrecedenceAndCalls(t *testing.T) {
	for _, tc := range []struct{ src, want string }{
		{"1 + 2 * 3", "(1 + (2 * 3))"},
		{"a == b && c", "((a == b) && c)"},
		{"a || b && c == d + e * f", "(a || (b && (c == (d + (e * f)))))"},
		{"8 - 3 - 1", "((8 - 3) - 1)"},
		{"8 / 2 * 3", "((8 / 2) * 3)"},
		{"a != b || c <= d && e >= f", "((a != b) || ((c <= d) && (e >= f)))"},
		{"a < b == c > d", "(((a < b) == c) > d)"},
		{"-f(a) * 2", "((-f(a)) * 2)"},
		{"!!a", "(!(!a))"},
		{"f(a)(b)", "f(a)(b)"},
		{"(1 + 2) * 3", "((1 + 2) * 3)"},
		{"(func(x) => x)(1)", "lambda(1)"},
		{"1 +\n 2", "(1 + 2)"},
		{"func() =>\n 1", "lambda"},
	} {
		t.Run(tc.src, func(t *testing.T) {
			if got := exprShape(result(t, tc.src)); got != tc.want {
				t.Errorf("shape = %s, want %s", got, tc.want)
			}
		})
	}
}

func TestBlockExpressions(t *testing.T) {
	b := result(t, `{ let x = 1
 print(x)
 x + 2;
}`).(*ast.BlockExpr).Block
	if len(b.Stmts) != 2 {
		t.Fatalf("statements = %#v", b.Stmts)
	}
	let := b.Stmts[0].(*ast.LetStmt)
	if let.Name != "x" || exprShape(let.Value) != "1" {
		t.Fatalf("let = %#v", let)
	}
	if got := exprShape(b.Stmts[1].(*ast.ExprStmt).X); got != "print(x)" {
		t.Errorf("expr statement = %s", got)
	}
	if got := exprShape(b.Result); got != "(x + 2)" {
		t.Errorf("result = %s", got)
	}
}

func TestErrorsWithPositions(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{"unterminated string", "package main\nfunc f() String { \"bad", "2:19: unterminated string"},
		{"empty block", "package main\nfunc f() Int {}", "2:15: block must end with an expression"},
		{"let-only block", "package main\nfunc f() Int {\n let x = 1\n}", "4:1: block must end with an expression"},
		{"empty do", "package main\nfunc f() Int { do {} }", "2:20: do block must end with an expression"},
		{"bind-only do", "package main\nfunc f() Int { do { x <- read(); } }", "2:34: do block must end with an expression"},
		{"empty match", "package main\nfunc f() Int { match x {} }", "2:16: match needs at least one arm"},
		{"bad superclass", "package main\nclass Monad[M[_]] : Functor[F] {}", "2:1: superclass must be applied to M"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := parser.Parse(tc.src)
			if err == nil || err.Error() != tc.want {
				t.Fatalf("error = %v, want %q", err, tc.want)
			}
		})
	}
}

// exprShape preserves grouping and call nesting while omitting source positions.
func exprShape(e ast.Expr) string {
	switch e := e.(type) {
	case *ast.Ident:
		return e.Name
	case *ast.IntLit:
		return fmt.Sprint(e.Value)
	case *ast.Binary:
		return "(" + exprShape(e.L) + " " + e.Op + " " + exprShape(e.R) + ")"
	case *ast.Unary:
		return "(" + e.Op + exprShape(e.X) + ")"
	case *ast.Call:
		args := make([]string, len(e.Args))
		for i, arg := range e.Args {
			args[i] = exprShape(arg)
		}
		return exprShape(e.Fn) + "(" + strings.Join(args, ", ") + ")"
	case *ast.Lambda:
		return "lambda"
	default:
		return fmt.Sprintf("%T", e)
	}
}
