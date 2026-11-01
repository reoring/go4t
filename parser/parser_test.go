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

func TestInstanceHeadsAndContexts(t *testing.T) {
	for _, tc := range []struct {
		name    string
		head    string
		params  []string
		context []ast.Constraint
	}{
		{"bare", "C[T]", nil, nil},
		{"parameterized", "C[T[A, B]]", []string{"A", "B"}, nil},
		{"one constraint", "C[T[A]] given\n D[A]", []string{"A"},
			[]ast.Constraint{{Pos: ast.Pos{Line: 3, Col: 2}, Class: "D", Param: "A"}}},
		{"two constraints", "C[T[A, B]] given\n D[A],\n E[B]", []string{"A", "B"},
			[]ast.Constraint{
				{Pos: ast.Pos{Line: 3, Col: 2}, Class: "D", Param: "A"},
				{Pos: ast.Pos{Line: 4, Col: 2}, Class: "E", Param: "B"},
			}},
		{"inline constraints", "C[T[A, B]] given D[A], E[B]", []string{"A", "B"},
			[]ast.Constraint{
				{Pos: ast.Pos{Line: 2, Col: 27}, Class: "D", Param: "A"},
				{Pos: ast.Pos{Line: 2, Col: 33}, Class: "E", Param: "B"},
			}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := parse(t, "package main\ninstance "+tc.head+" { value(x) { x } }")
			d := f.Decls[0].(*ast.InstanceDecl)
			if d.Pos != (ast.Pos{Line: 2, Col: 1}) || d.Class != "C" || d.TypeCon != "T" {
				t.Fatalf("instance = %#v", d)
			}
			if !reflect.DeepEqual(d.Params, tc.params) || !reflect.DeepEqual(d.Context, tc.context) {
				t.Errorf("params/context = %#v / %#v, want %#v / %#v", d.Params, d.Context, tc.params, tc.context)
			}
			if len(d.Fixed) != 0 {
				t.Errorf("Fixed = %v, want checker to split head arguments", d.Fixed)
			}
			if len(d.Methods) != 1 || d.Methods[0].Name != "value" ||
				!reflect.DeepEqual(d.Methods[0].Params, []string{"x"}) || exprShape(d.Methods[0].Body.Result) != "x" {
				t.Errorf("methods = %#v", d.Methods)
			}
		})
	}
}

func TestTupleAndPartialInstanceHeads(t *testing.T) {
	for _, tc := range []struct {
		head, class, typeCon string
		params               []string
		context              []string
	}{
		{"Show[(A, B)] given Show[A], Show[B]", "Show", "Tuple2", []string{"A", "B"}, []string{"A", "B"}},
		{"Show[(A, B, C)]", "Show", "Tuple3", []string{"A", "B", "C"}, nil},
		{"Functor[Result[E]]", "Functor", "Result", []string{"E"}, nil},
		{"Functor[Result[E]] given Show[E]", "Functor", "Result", []string{"E"}, []string{"E"}},
	} {
		t.Run(tc.head, func(t *testing.T) {
			d := parse(t, "package main\ninstance "+tc.head+" {}").Decls[0].(*ast.InstanceDecl)
			if d.Class != tc.class || d.TypeCon != tc.typeCon || !reflect.DeepEqual(d.Params, tc.params) || len(d.Fixed) != 0 {
				t.Fatalf("instance = %#v, want %s[%s] with Params %v and no Fixed", d, tc.class, tc.typeCon, tc.params)
			}
			if len(d.Context) != len(tc.context) {
				t.Fatalf("context = %#v, want Show constraints on %v", d.Context, tc.context)
			}
			for i, param := range tc.context {
				if d.Context[i].Class != "Show" || d.Context[i].Param != param {
					t.Errorf("constraint = %#v, want Show[%s]", d.Context[i], param)
				}
			}
		})
	}
}

func TestInstanceHeadErrorsWithPositions(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{"given without params", "instance C[T] given D[A] {}", "2:15: given requires instance type parameters"},
		{"undeclared context param", "instance C[T[A]] given D[B] {}", "2:26: undeclared instance type parameter B"},
		{"second undeclared context param", "instance C[T[A, B]] given D[A], E[C] {}", "2:35: undeclared instance type parameter C"},
		{"duplicate param", "instance C[T[A, A]] {}", "2:17: duplicate instance type parameter A"},
		{"empty head params", "instance C[T[]] {}", `2:14: expected identifier, found "]"`},
		{"nested head param", "instance C[T[A[B]]] {}", `2:15: expected "]", found "["`},
		{"empty context", "instance C[T[A]] given {}", `2:24: expected identifier, found "{"`},
		{"non-parameter context", "instance C[T[A]] given D[T[A]] {}", "2:26: undeclared instance type parameter T"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := parser.Parse("package main\n" + tc.src)
			if err == nil || err.Error() != tc.want {
				t.Fatalf("error = %v, want %q", err, tc.want)
			}
		})
	}
}

func TestTupleAndParenthesisedTypes(t *testing.T) {
	f := parse(t, `package main
func pairs[A, B](pair (A, B), pairs List[(A, B)], nested (A, (B, A)), triple (A, B, A), plain (A), apply func((A, B)) (B, A)) (A, B) { pair }
func grouped[A, B](apply (func(A) B)) ((A)) { x }
`)
	d := f.Decls[0].(*ast.FuncDecl)
	pair := tupleType(t, d.Params[0].Type, "A", "B")
	if pair.Pos != (ast.Pos{Line: 2, Col: 23}) {
		t.Errorf("tuple position = %s, want 2:23", pair.Pos)
	}
	list, ok := d.Params[1].Type.(*ast.TypeName)
	if !ok || list.Name != "List" || len(list.Args) != 1 {
		t.Fatalf("list type = %#v", d.Params[1].Type)
	}
	tupleType(t, list.Args[0], "A", "B")
	nested, ok := d.Params[2].Type.(*ast.TupleType)
	if !ok || len(nested.Elems) != 2 {
		t.Fatalf("nested tuple = %#v", d.Params[2].Type)
	}
	typeName(t, nested.Elems[0], "A")
	tupleType(t, nested.Elems[1], "B", "A")
	tupleType(t, d.Params[3].Type, "A", "B", "A")
	typeName(t, d.Params[4].Type, "A")
	ft, ok := d.Params[5].Type.(*ast.FuncType)
	if !ok || len(ft.Params) != 1 {
		t.Fatalf("function type = %#v", d.Params[5].Type)
	}
	tupleType(t, ft.Params[0], "A", "B")
	tupleType(t, ft.Ret, "B", "A")
	tupleType(t, d.Ret, "A", "B")
	grouped := f.Decls[1].(*ast.FuncDecl)
	ft, ok = grouped.Params[0].Type.(*ast.FuncType)
	if !ok || len(ft.Params) != 1 {
		t.Fatalf("parenthesised function type = %#v", grouped.Params[0].Type)
	}
	typeName(t, ft.Params[0], "A")
	typeName(t, ft.Ret, "B")
	typeName(t, grouped.Ret, "A")
}

func tupleType(t *testing.T, typ ast.Type, names ...string) *ast.TupleType {
	t.Helper()
	tuple, ok := typ.(*ast.TupleType)
	if !ok || len(tuple.Elems) != len(names) {
		t.Fatalf("type = %#v, want tuple with elements %v", typ, names)
	}
	for i, name := range names {
		typeName(t, tuple.Elems[i], name)
	}
	return tuple
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

func TestTupleAndAsPatterns(t *testing.T) {
	for _, tc := range []struct{ src, want string }{
		{"(a, (b, c))", "(a, (b, c))"},
		{"(a, b, c)", "(a, b, c)"},
		{"(x)", "x"},
		{"((Some(x)))", "Some(x)"},
		{"Some(whole @ MkStock(sku, qty))", "Some(whole @ MkStock(sku, qty))"},
		{"whole @ (left, right)", "whole @ (left, right)"},
		{"(whole @ MkStock(sku, qty), rest)", "(whole @ MkStock(sku, qty), rest)"},
		{"outer @ inner @ Some(x)", "outer @ inner @ Some(x)"},
		{"whole @ _", "whole @ _"},
		{"value @ 1", "value @ 1"},
		{"α @ Some(x)", "α @ Some(x)"},
	} {
		t.Run(tc.src, func(t *testing.T) {
			m := result(t, "match value { | "+tc.src+" => value | _ => fallback }").(*ast.Match)
			if len(m.Arms) != 2 {
				t.Fatalf("arms = %#v, want two arms", m.Arms)
			}
			if got := patternShape(m.Arms[0].Pat); got != tc.want {
				t.Errorf("pattern = %s, want %s", got, tc.want)
			}
			if _, ok := m.Arms[1].Pat.(*ast.PWild); !ok {
				t.Errorf("second pattern = %T, want wildcard", m.Arms[1].Pat)
			}
			if exprShape(m.Arms[0].Body) != "value" || exprShape(m.Arms[1].Body) != "fallback" {
				t.Errorf("arm bodies = %#v", m.Arms)
			}
		})
	}
	m := result(t, "match value { (x, whole @ Some(y)) => x }").(*ast.Match)
	tuple := m.Arms[0].Pat.(*ast.PTuple)
	if tuple.Pos != (ast.Pos{Line: 2, Col: 33}) {
		t.Errorf("tuple position = %s, want 2:33", tuple.Pos)
	}
	as := tuple.Elems[1].(*ast.PAs)
	if as.Pos != (ast.Pos{Line: 2, Col: 37}) {
		t.Errorf("as-pattern position = %s, want 2:37", as.Pos)
	}
	if pat := result(t, "match x { (x) => x }").(*ast.Match).Arms[0].Pat; patternShape(pat) != "x" {
		t.Errorf("parenthesised variable = %T, want PVar", pat)
	}
}

func patternShape(p ast.Pattern) string {
	switch p := p.(type) {
	case *ast.PVar:
		return p.Name
	case *ast.PWild:
		return "_"
	case *ast.PLit:
		return exprShape(p.X)
	case *ast.PCtor:
		args := make([]string, len(p.Args))
		for i, arg := range p.Args {
			args[i] = patternShape(arg)
		}
		return p.Name + "(" + strings.Join(args, ", ") + ")"
	case *ast.PRecord:
		fields := make([]string, len(p.Names))
		for i, name := range p.Names {
			fields[i] = name + ": " + patternShape(p.Pats[i])
		}
		if p.Rest {
			fields = append(fields, "..")
		}
		return p.Ctor + "{" + strings.Join(fields, ", ") + "}"
	case *ast.PTuple:
		elems := make([]string, len(p.Elems))
		for i, elem := range p.Elems {
			elems[i] = patternShape(elem)
		}
		return "(" + strings.Join(elems, ", ") + ")"
	case *ast.PAs:
		return p.Name + " @ " + patternShape(p.Pat)
	default:
		return fmt.Sprintf("%T", p)
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

func TestIfElseNewlineLayout(t *testing.T) {
	for _, gap := range []string{"\n", "\n\n  ", " // trailing\n// between\n "} {
		t.Run(gap, func(t *testing.T) {
			e := result(t, "if a { 1 }"+gap+"else if b { 2 }"+gap+"else { 3 }").(*ast.If)
			if exprShape(e.Cond) != "a" || exprShape(e.Then.(*ast.BlockExpr).Block.Result) != "1" {
				t.Fatalf("if = %#v", e)
			}
			nested := e.Else.(*ast.If)
			if exprShape(nested.Cond) != "b" || exprShape(nested.Then.(*ast.BlockExpr).Block.Result) != "2" ||
				exprShape(nested.Else.(*ast.BlockExpr).Block.Result) != "3" {
				t.Fatalf("else if = %#v", nested)
			}
		})
	}
}

func TestListLiterals(t *testing.T) {
	for _, tc := range []struct {
		src  string
		want string
		size int
	}{
		{"[]", "[]", 0},
		{"[1, 2]", "[1, 2]", 2},
		{"[[1], []]", "[[1], []]", 2},
		{"[1 + 2, f(3)]", "[(1 + 2), f(3)]", 2},
		{"[\n 1,\n 2,\n]", "[1, 2]", 2},
	} {
		t.Run(tc.src, func(t *testing.T) {
			e := result(t, tc.src)
			list, ok := e.(*ast.ListLit)
			if !ok {
				t.Fatalf("expression = %T, want ListLit", e)
			}
			if list.GetPos() != (ast.Pos{Line: 2, Col: 19}) || len(list.Elems) != tc.size {
				t.Errorf("list = %#v, want %d elements at 2:19", list, tc.size)
			}
			if got := exprShape(list); got != tc.want {
				t.Errorf("shape = %s, want %s", got, tc.want)
			}
		})
	}
	call := result(t, "f([1, 2], [])").(*ast.Call)
	if got := exprShape(call); got != "f([1, 2], [])" {
		t.Errorf("list call = %s", got)
	}
	if len(call.Args) != 2 {
		t.Fatalf("arguments = %#v, want two lists", call.Args)
	}
	for _, arg := range call.Args {
		if _, ok := arg.(*ast.ListLit); !ok {
			t.Errorf("argument = %T, want ListLit", arg)
		}
	}
}

func TestTupleLiteralsAndParentheses(t *testing.T) {
	for _, tc := range []struct {
		src, want string
		size      int
	}{
		{"(a, b)", "(a, b)", 2},
		{"(a, (b, c))", "(a, (b, c))", 2},
		{"(a, b, c)", "(a, b, c)", 3},
		{"(a + b, f(c))", "((a + b), f(c))", 2},
		{"(\n a,\n b)", "(a, b)", 2},
	} {
		t.Run(tc.src, func(t *testing.T) {
			e := result(t, tc.src)
			tuple, ok := e.(*ast.TupleLit)
			if !ok || len(tuple.Elems) != tc.size {
				t.Fatalf("expression = %#v, want tuple with %d elements", e, tc.size)
			}
			if tuple.GetPos() != (ast.Pos{Line: 2, Col: 19}) || exprShape(tuple) != tc.want {
				t.Errorf("tuple = %s at %s, want %s at 2:19", exprShape(tuple), tuple.GetPos(), tc.want)
			}
		})
	}
	for _, src := range []string{"(x)", "((x))"} {
		if e, ok := result(t, src).(*ast.Ident); !ok || e.Name != "x" {
			t.Errorf("%s is not a parenthesised identifier: %#v", src, e)
		}
	}
	call := result(t, "f((a, b), [(c, d)])").(*ast.Call)
	if len(call.Args) != 2 || exprShape(call) != "f((a, b), [(c, d)])" {
		t.Errorf("tuple call/list = %#v", call)
	}
}

func TestTupleAndAsPatternErrors(t *testing.T) {
	for _, tc := range []struct{ name, decl, want string }{
		{"empty expression", "func f() Int { () }", `expected expression, found ")"`},
		{"singleton expression comma", "func f() Int { (a,) }", `expected expression, found ")"`},
		{"expression trailing comma", "func f() Int { (a, b,) }", `expected expression, found ")"`},
		{"empty type", "func f(x ()) Int { x }", `expected identifier, found ")"`},
		{"singleton type comma", "func f(x (A,)) Int { x }", `expected identifier, found ")"`},
		{"type trailing comma", "func f(x (A, B,)) Int { x }", `expected identifier, found ")"`},
		{"empty pattern", "func f() Int { match x { () => 0 } }", `expected pattern, found ")"`},
		{"singleton pattern comma", "func f() Int { match x { (a,) => 0 } }", `expected pattern, found ")"`},
		{"pattern trailing comma", "func f() Int { match x { (a, b,) => 0 } }", `expected pattern, found ")"`},
		{"uppercase binder", "func f() Int { match x { Whole @ Some(a) => 0 } }", "as-pattern name must be a lower-case identifier"},
		{"underscore binder", "func f() Int { match x { _whole @ Some(a) => 0 } }", "as-pattern name must be a lower-case identifier"},
		{"wildcard binder", "func f() Int { match x { _ @ Some(a) => 0 } }", `expected "=>", found "@"`},
		{"literal binder", "func f() Int { match x { 1 @ Some(a) => 0 } }", `expected "=>", found "@"`},
		{"parenthesised binder", "func f() Int { match x { (whole) @ Some(a) => 0 } }", `expected "=>", found "@"`},
		{"constructor binder", "func f() Int { match x { Some(a) @ other => 0 } }", `expected "=>", found "@"`},
		{"missing inner pattern", "func f() Int { match x { whole @ => 0 } }", `expected pattern, found "=>"`},
		{"as in expression", "func f() Int { whole @ other }", `expected expression, found "@"`},
		{"empty tuple head", "instance Show[()] {}", `expected identifier, found ")"`},
		{"singleton tuple head", "instance Show[(A)] {}", "tuple instance head needs at least two elements"},
		{"tuple head trailing comma", "instance Show[(A, B,)] {}", `expected identifier, found ")"`},
		{"duplicate tuple head param", "instance Show[(A, A)] {}", "duplicate instance type parameter A"},
		{"undeclared tuple context param", "instance Show[(A, B)] given Show[C] {}", "undeclared instance type parameter C"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := parser.Parse("package main\n" + tc.decl)
			if err == nil || !strings.HasSuffix(err.Error(), ": "+tc.want) {
				t.Fatalf("error = %v, want suffix %q", err, tc.want)
			}
		})
	}
}

func TestListIsNotIndexing(t *testing.T) {
	f := parse(t, "package main\nfunc f() List[Int] { xs[1] }")
	b := f.Decls[0].(*ast.FuncDecl).Body
	if len(b.Stmts) != 1 || exprShape(b.Stmts[0].(*ast.ExprStmt).X) != "xs" {
		t.Fatalf("statements = %#v, want xs expression", b.Stmts)
	}
	if _, ok := b.Result.(*ast.ListLit); !ok || exprShape(b.Result) != "[1]" {
		t.Fatalf("result = %#v, want separate [1] list literal", b.Result)
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
	case *ast.StringLit:
		return fmt.Sprintf("%q", e.Value)
	case *ast.BoolLit:
		return fmt.Sprint(e.Value)
	case *ast.Field:
		return exprShape(e.X) + "." + e.Name
	case *ast.RecordLit:
		return e.Ctor + recordShape(e.Names, e.Values)
	case *ast.RecordUpdate:
		return exprShape(e.X) + recordShape(e.Names, e.Values)
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
	case *ast.ListLit:
		elems := make([]string, len(e.Elems))
		for i, elem := range e.Elems {
			elems[i] = exprShape(elem)
		}
		return "[" + strings.Join(elems, ", ") + "]"
	case *ast.TupleLit:
		elems := make([]string, len(e.Elems))
		for i, elem := range e.Elems {
			elems[i] = exprShape(elem)
		}
		return "(" + strings.Join(elems, ", ") + ")"
	case *ast.Lambda:
		return "lambda"
	default:
		return fmt.Sprintf("%T", e)
	}
}

func TestInterpolation(t *testing.T) {
	e := result(t, `"hello ${name}: ${1 + 2}!"`)
	interp, ok := e.(*ast.Interp)
	if !ok {
		t.Fatalf("expression = %T, want Interp", e)
	}
	if !reflect.DeepEqual(interp.Texts, []string{"hello ", ": ", "!"}) || len(interp.Exprs) != 2 {
		t.Fatalf("interpolation = %#v", interp)
	}
	if exprShape(interp.Exprs[0]) != "name" || exprShape(interp.Exprs[1]) != "(1 + 2)" {
		t.Errorf("interpolated expressions = %#v", interp.Exprs)
	}
}

func TestRecordDeclarations(t *testing.T) {
	f := parse(t, "package main\ntype Pair[A, B] = Pair{first: A, second: B} | Empty | Positional(A, B)")
	d := f.Decls[0].(*ast.TypeDecl)
	if len(d.Ctors) != 3 {
		t.Fatalf("constructors = %#v", d.Ctors)
	}
	record := d.Ctors[0]
	if record.Name != "Pair" || !reflect.DeepEqual(record.FieldNames, []string{"first", "second"}) || len(record.Fields) != 2 {
		t.Fatalf("record = %#v", record)
	}
	typeName(t, record.Fields[0], "A")
	typeName(t, record.Fields[1], "B")
	for _, ctor := range d.Ctors[1:] {
		if ctor.FieldNames != nil {
			t.Errorf("positional constructor has field names: %#v", ctor)
		}
	}
}

func recordShape(names []string, values []ast.Expr) string {
	fields := make([]string, len(names))
	for i, name := range names {
		fields[i] = name + ": " + exprShape(values[i])
	}
	return "{" + strings.Join(fields, ", ") + "}"
}

func TestInterpolationNestedExpressions(t *testing.T) {
	for _, tc := range []struct{ src, want string }{
		{`"${show(Point{a: 1}.a)}"`, "show(Point{a: 1}.a)"},
		{`"${show(Point{a: "quoted \"text\"", b: Inner{n: 1}}.a)}"`, `show(Point{a: "quoted \"text\"", b: Inner{n: 1}}.a)`},
		{`"${"nested"}"`, `"nested"`},
		{`"${show("{braces}")}"`, `show("{braces}")`},
		{`"${show("{{braces}}")}"`, `show("{{braces}}")`},
		{`"${show("$${not interp}")}"`, `show("${not interp}")`},
		{`"${show(r{a: 1}.a)}"`, "show(r{a: 1}.a)"},
		{"\"${x + // } and \" are comment text\n 1}\"", "(x + 1)"},
	} {
		t.Run(tc.src, func(t *testing.T) {
			interp := result(t, tc.src).(*ast.Interp)
			if !reflect.DeepEqual(interp.Texts, []string{"", ""}) || len(interp.Exprs) != 1 {
				t.Fatalf("interpolation = %#v", interp)
			}
			if got := exprShape(interp.Exprs[0]); got != tc.want {
				t.Errorf("expression = %s, want %s", got, tc.want)
			}
		})
	}
	outer := result(t, `"${show("${Point{a: "x"}.a}")}"`).(*ast.Interp)
	call := outer.Exprs[0].(*ast.Call)
	inner := call.Args[0].(*ast.Interp)
	if !reflect.DeepEqual(inner.Texts, []string{"", ""}) || len(inner.Exprs) != 1 ||
		exprShape(inner.Exprs[0]) != `Point{a: "x"}.a` {
		t.Errorf("nested interpolation = %#v", inner)
	}
	conditional := result(t, `"${if r.ok { 1 } else { 2 }}"`).(*ast.Interp).Exprs[0].(*ast.If)
	if exprShape(conditional.Cond) != "r.ok" {
		t.Errorf("interpolated condition = %#v", conditional.Cond)
	}
}

func TestInterpolationLiteralTextAndPositions(t *testing.T) {
	for _, tc := range []struct{ src, want string }{
		{`"plain"`, "plain"},
		{`"{not interp}"`, "{not interp}"},
		{`"{x}"`, "{x}"},
		{`"{{x}}"`, "{{x}}"},
		{`"{"`, "{"},
		{`"}}"`, "}}"},
		{`"$"`, "$"},
		{`"$value"`, "$value"},
		{`"$$"`, "$"},
		{`"$$$"`, "$$"},
		{`"$$$$"`, "$$"},
		{`"$${x}"`, "${x}"},
		{`"$${"`, "${"},
		{`"\x7bx\u007d"`, "{x}"},
	} {
		e, ok := result(t, tc.src).(*ast.StringLit)
		if !ok || e.Value != tc.want {
			t.Errorf("%s = %#v, want StringLit %q", tc.src, e, tc.want)
		}
	}
	interp := result(t, `"α\n{${x}}${y}\t"`).(*ast.Interp)
	if !reflect.DeepEqual(interp.Texts, []string{"α\n{", "}", "\t"}) || len(interp.Exprs) != 2 {
		t.Fatalf("interpolation = %#v", interp)
	}
	if interp.GetPos() != (ast.Pos{Line: 2, Col: 19}) ||
		interp.Exprs[0].GetPos() != (ast.Pos{Line: 2, Col: 26}) ||
		interp.Exprs[1].GetPos() != (ast.Pos{Line: 2, Col: 31}) {
		t.Errorf("positions = %s, %s, %s", interp.GetPos(), interp.Exprs[0].GetPos(), interp.Exprs[1].GetPos())
	}
	interp = result(t, `"$$${x} $$ $"`).(*ast.Interp)
	if !reflect.DeepEqual(interp.Texts, []string{"$", " $ $"}) || len(interp.Exprs) != 1 ||
		exprShape(interp.Exprs[0]) != "x" || interp.Exprs[0].GetPos() != (ast.Pos{Line: 2, Col: 24}) {
		t.Errorf("interpolation with literal dollars = %#v", interp)
	}
}

func TestInterpolationParseErrorsInOriginalFile(t *testing.T) {
	for _, tc := range []struct{ src, want string }{
		{`  "prefix ${1 + )}"`, `3:17: expected expression, found ")"`},
		{`  "α ${x + )}"`, `3:12: expected expression, found ")"`},
		{`  "outer ${show("inner ${1 + )}")}"`, `3:30: expected expression, found ")"`},
		{`  "${show("\q")}"`, "3:11: bad string literal:"},
		{`  "${}"`, "3:6: expected expression, found end of file"},
		{`  "${x y}"`, `3:8: expected end of expression, found "y"`},
		{`  "${x; y}"`, `3:9: expected end of expression, found "y"`},
		{`  "bad ${x"`, "3:8: unterminated interpolation"},
	} {
		t.Run(tc.src, func(t *testing.T) {
			_, err := parser.Parse("package main\nfunc f() String {\n" + tc.src + "\n}")
			if err == nil || !strings.HasPrefix(err.Error(), tc.want) {
				t.Fatalf("error = %v, want prefix %q", err, tc.want)
			}
		})
	}
}

func TestParseExprStartingPosition(t *testing.T) {
	e, err := parser.ParseExpr("α +\n  x", ast.Pos{Line: 7, Col: 11})
	if err != nil {
		t.Fatal(err)
	}
	binary := e.(*ast.Binary)
	if binary.GetPos() != (ast.Pos{Line: 7, Col: 13}) ||
		binary.L.GetPos() != (ast.Pos{Line: 7, Col: 11}) || binary.R.GetPos() != (ast.Pos{Line: 8, Col: 3}) {
		t.Errorf("offset positions = %s, %s, %s", binary.GetPos(), binary.L.GetPos(), binary.R.GetPos())
	}
	for _, tc := range []struct{ src, want string }{
		{"x y", `7:13: expected end of expression, found "y"`},
		{"x +\n )", `8:2: expected expression, found ")"`},
		{"x + $", "7:15: unexpected character '$'"},
		{"", "7:11: expected expression, found end of file"},
	} {
		t.Run(tc.src, func(t *testing.T) {
			_, err := parser.ParseExpr(tc.src, ast.Pos{Line: 7, Col: 11})
			if err == nil || err.Error() != tc.want {
				t.Fatalf("error = %v, want %q", err, tc.want)
			}
		})
	}
}

func TestRecordExpressions(t *testing.T) {
	for _, tc := range []struct{ src, want string }{
		{"Point{y: 2, x: 1}", "Point{y: 2, x: 1}"},
		{"Point{\n x: 1,\n y: 2,\n}", "Point{x: 1, y: 2}"},
		{"x.a.b", "x.a.b"},
		{"x{a: 1}", "x{a: 1}"},
		{"x{a: 1}.a", "x{a: 1}.a"},
		{"f().a(1){b: 2}.c", "f().a(1){b: 2}.c"},
		{"Point{x: 1, y: 2}{x: 3}.x + 4", "(Point{x: 1, y: 2}{x: 3}.x + 4)"},
		{"r{a: Inner{b: 1}, c: f(2),}", "r{a: Inner{b: 1}, c: f(2)}"},
		{"-x.a * r{b: 2}.b", "((-x.a) * r{b: 2}.b)"},
		{"Point(1, 2)", "Point(1, 2)"},
	} {
		t.Run(tc.src, func(t *testing.T) {
			if got := exprShape(result(t, tc.src)); got != tc.want {
				t.Errorf("expression = %s, want %s", got, tc.want)
			}
		})
	}
	field := result(t, "x.a.b").(*ast.Field)
	if field.Name != "b" || field.Index != -1 || field.GetPos() != (ast.Pos{Line: 2, Col: 22}) {
		t.Fatalf("outer field = %#v", field)
	}
	inner := field.X.(*ast.Field)
	if inner.Name != "a" || inner.Index != -1 || inner.GetPos() != (ast.Pos{Line: 2, Col: 20}) || exprShape(inner.X) != "x" {
		t.Errorf("inner field = %#v", inner)
	}
	record := result(t, "Point{y: 2, x: 1}").(*ast.RecordLit)
	if record.GetPos() != (ast.Pos{Line: 2, Col: 19}) ||
		!reflect.DeepEqual(record.Names, []string{"y", "x"}) || len(record.Values) != 2 {
		t.Errorf("record = %#v", record)
	}
	update := result(t, "r{a: 1}").(*ast.RecordUpdate)
	if update.GetPos() != (ast.Pos{Line: 2, Col: 20}) || exprShape(update.X) != "r" ||
		!reflect.DeepEqual(update.Names, []string{"a"}) || len(update.Values) != 1 {
		t.Errorf("update = %#v", update)
	}
}

func TestRecordsDoNotConsumeControlFlowBlocks(t *testing.T) {
	for _, src := range []string{
		"if r.ok { yes } else { no }",
		"if r.ok { f(1) } else { f(2) }",
		"if r.ok { Point{x: 1} } else { Point{x: 2} }",
	} {
		e := result(t, src).(*ast.If)
		if exprShape(e.Cond) != "r.ok" {
			t.Errorf("condition = %#v", e.Cond)
		}
	}
	for _, src := range []string{
		"match p { | Point(x, y) => x + y }",
		"match p { Point(x, y) => x + y }",
	} {
		m := result(t, src).(*ast.Match)
		if exprShape(m.Scrut) != "p" || len(m.Arms) != 1 ||
			patternShape(m.Arms[0].Pat) != "Point(x, y)" || exprShape(m.Arms[0].Body) != "(x + y)" {
			t.Errorf("match = %#v", m)
		}
	}
	b := parse(t, "package main\nfunc f() Int { x\n{ y } }").Decls[0].(*ast.FuncDecl).Body
	if len(b.Stmts) != 1 || exprShape(b.Stmts[0].(*ast.ExprStmt).X) != "x" {
		t.Fatalf("statements = %#v, want separate x expression", b.Stmts)
	}
	if block, ok := b.Result.(*ast.BlockExpr); !ok || exprShape(block.Block.Result) != "y" {
		t.Errorf("result = %#v, want separate block", b.Result)
	}
}

func TestRecordDeclarationLayout(t *testing.T) {
	d := parse(t, "package main\ntype R[A] = R{\n value: A,\n apply: func(A) A,\n}").Decls[0].(*ast.TypeDecl)
	c := d.Ctors[0]
	if !reflect.DeepEqual(c.FieldNames, []string{"value", "apply"}) || len(c.Fields) != 2 {
		t.Fatalf("record = %#v", c)
	}
	typeName(t, c.Fields[0], "A")
	fn := c.Fields[1].(*ast.FuncType)
	if len(fn.Params) != 1 {
		t.Fatalf("function field = %#v", fn)
	}
	typeName(t, fn.Params[0], "A")
	typeName(t, fn.Ret, "A")
}

func TestRecordSyntaxErrors(t *testing.T) {
	for _, tc := range []struct{ src, want string }{
		{"type Empty = Empty{}", "2:14: record constructor needs at least one field"},
		{"type R = R{a: Int, String}", `expected ":", found "}"`},
		{"type R = R(Int, b: String)", `expected ")", found ":"`},
		{"type R = R{a: Int}(String)", `expected declaration, found "("`},
		{"func f() R { R{a: } }", `expected expression, found "}"`},
		{"func f() R { r{a: 1, 2} }", `expected identifier, found "2"`},
	} {
		t.Run(tc.src, func(t *testing.T) {
			_, err := parser.Parse("package main\n" + tc.src)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error = %v, want %q", err, tc.want)
			}
		})
	}
	for _, src := range []string{"r\n{a: 1}", "Point\n{a: 1}"} {
		_, err := parser.ParseExpr(src, ast.Pos{Line: 1, Col: 1})
		if err == nil || err.Error() != `2:1: expected end of expression, found "{"` {
			t.Errorf("error for %q = %v, want separate brace after newline", src, err)
		}
	}
}

func TestRecordPatterns(t *testing.T) {
	for _, tc := range []struct {
		src   string
		names []string
		pats  []string
		rest  bool
	}{
		{"Point{x: 0, y: value}", []string{"x", "y"}, []string{"0", "value"}, false},
		{"Point{y: value, x: _}", []string{"y", "x"}, []string{"value", "_"}, false},
		{"Point{x: 0, ..}", []string{"x"}, []string{"0"}, true},
		{"Point{..}", nil, nil, true},
		{"Point{x: 0,}", []string{"x"}, []string{"0"}, false},
		{"Point{x: 0, ..,}", []string{"x"}, []string{"0"}, true},
		{"Point{\n x: 0,\n y: value,\n}", []string{"x", "y"}, []string{"0", "value"}, false},
		{"Point{\n x: 0,\n ..\n}", []string{"x"}, []string{"0"}, true},
	} {
		t.Run(tc.src, func(t *testing.T) {
			e, err := parser.ParseExpr("match p { "+tc.src+" => 1 | Point{..} => 2 }", ast.Pos{Line: 1, Col: 1})
			if err != nil {
				t.Fatal(err)
			}
			m := e.(*ast.Match)
			if exprShape(m.Scrut) != "p" || len(m.Arms) != 2 {
				t.Fatalf("match = %#v", m)
			}
			record, ok := m.Arms[0].Pat.(*ast.PRecord)
			if !ok {
				t.Fatalf("pattern = %T, want PRecord", m.Arms[0].Pat)
			}
			if record.Ctor != "Point" || record.Pos != (ast.Pos{Line: 1, Col: 11}) ||
				!reflect.DeepEqual(record.Names, tc.names) || len(record.Pats) != len(tc.pats) || record.Rest != tc.rest {
				t.Fatalf("record = %#v", record)
			}
			for i, pat := range record.Pats {
				if got := patternShape(pat); got != tc.pats[i] {
					t.Errorf("pattern %d = %s, want %s", i, got, tc.pats[i])
				}
			}
			if got := patternShape(m.Arms[1].Pat); got != "Point{..}" {
				t.Errorf("second pattern = %s, want Point{..}", got)
			}
			if exprShape(m.Arms[0].Body) != "1" || exprShape(m.Arms[1].Body) != "2" {
				t.Errorf("arm bodies = %#v", m.Arms)
			}
		})
	}
}

func TestNestedRecordPatterns(t *testing.T) {
	for _, src := range []string{
		"Point{x: Pair(a, b), y: (c, d)}",
		"Box{value: Point{x: 0, ..}, ..}",
		"whole @ Point{x: value @ Some(n), ..}",
		"Some(Point{..})",
		"(Point{..}, Point{x: _, ..})",
		`Value{string: "text", bool: true, other: false}`,
	} {
		t.Run(src, func(t *testing.T) {
			m := result(t, "match p { "+src+" => 0 }").(*ast.Match)
			if got := patternShape(m.Arms[0].Pat); got != src {
				t.Errorf("pattern = %s, want %s", got, src)
			}
		})
	}
}

func TestRecordPatternErrors(t *testing.T) {
	for _, tc := range []struct{ src, want string }{
		{"Point{}", "1:11: record pattern needs fields or .."},
		{"Point{x: 0, x: y}", "1:23: duplicate field x in record pattern"},
		{"Point{.., x: 0}", "1:17: .. must be last in record pattern"},
		{"Point{x: 0, .., y: 1}", "1:23: .. must be last in record pattern"},
		{"Point{.., ..}", "1:17: .. must be last in record pattern"},
		{"Point{.. x: 0}", "1:17: .. must be last in record pattern"},
		{"Point{y, ..}", `1:18: expected ":", found ","`},
		{"Point{y}", `1:18: expected ":", found "}"`},
		{"Point{x: }", `1:20: expected pattern, found "}"`},
		{"point{x: 0}", `1:16: expected "=>", found "{"`},
		{"..", `1:11: expected pattern, found ".."`},
	} {
		t.Run(tc.src, func(t *testing.T) {
			_, err := parser.ParseExpr("match p { "+tc.src+" => 0 }", ast.Pos{Line: 1, Col: 1})
			if err == nil || err.Error() != tc.want {
				t.Fatalf("error = %v, want %q", err, tc.want)
			}
		})
	}
}

func TestTupleFieldAccess(t *testing.T) {
	for _, tc := range []struct {
		src     string
		names   []string
		indexes []int
		cols    []int
	}{
		{"p.0", []string{"0"}, []int{0}, []int{2}},
		{"p.1", []string{"1"}, []int{1}, []int{2}},
		{"p.12", []string{"12"}, []int{12}, []int{2}},
		{"p.00", []string{"00"}, []int{0}, []int{2}},
		{"p.0.1", []string{"0", "1"}, []int{0, 1}, []int{2, 4}},
		{"p.0.name", []string{"0", "name"}, []int{0, -1}, []int{2, 4}},
		{"p.name.0", []string{"name", "0"}, []int{-1, 0}, []int{2, 7}},
		{"p.a.b", []string{"a", "b"}, []int{-1, -1}, []int{2, 4}},
	} {
		t.Run(tc.src, func(t *testing.T) {
			e, err := parser.ParseExpr(tc.src, ast.Pos{Line: 1, Col: 1})
			if err != nil {
				t.Fatal(err)
			}
			for i := len(tc.names) - 1; i >= 0; i-- {
				field, ok := e.(*ast.Field)
				if !ok {
					t.Fatalf("expression = %T, want Field", e)
				}
				if field.Name != tc.names[i] || field.Index != tc.indexes[i] || field.GetPos() != (ast.Pos{Line: 1, Col: tc.cols[i]}) {
					t.Errorf("field %d = %#v", i, field)
				}
				e = field.X
			}
			if got := exprShape(e); got != "p" {
				t.Errorf("base = %s, want p", got)
			}
		})
	}
	for _, tc := range []struct{ src, want string }{
		{"p.0 + p.1 * 2", "(p.0 + (p.1 * 2))"},
		{"f().0.name(1).2", "f().0.name(1).2"},
		{"(a, b).1", "(a, b).1"},
		{"p.0{x: 1}.x", "p.0{x: 1}.x"},
		{"p\n.0\n.name", "p.0.name"},
	} {
		t.Run(tc.src, func(t *testing.T) {
			if got := exprShape(result(t, tc.src)); got != tc.want {
				t.Errorf("expression = %s, want %s", got, tc.want)
			}
		})
	}
}

func TestTupleFieldSyntaxErrors(t *testing.T) {
	for _, tc := range []struct{ src, want string }{
		{"p.-1", `1:3: expected identifier, found "-"`},
		{"p..0", `1:2: expected end of expression, found ".."`},
		{"p.999999999999999999999999999999999999", "1:3: bad tuple index 999999999999999999999999999999999999"},
		{"Point{x: 0, ..}", `1:13: expected identifier, found ".."`},
	} {
		t.Run(tc.src, func(t *testing.T) {
			_, err := parser.ParseExpr(tc.src, ast.Pos{Line: 1, Col: 1})
			if err == nil || err.Error() != tc.want {
				t.Fatalf("error = %v, want %q", err, tc.want)
			}
		})
	}
}
