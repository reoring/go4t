package types_test

import (
	"errors"
	"reflect"
	"testing"

	"gofx/ast"
	"gofx/types"
)

func TestAliasExpansion(t *testing.T) {
	files := parseFiles(t, `
alias Name = String
alias Pair[A] = (A, A)
alias Handler = func(Request) Response
alias Nested[A] = Pair[Pair[A]]
alias Forward = Later
alias Later = Name
alias Identity[A] = A
type Request = Request{name: Name}
type Response = Response{message: Forward}
type Box[A] = Box{value: Identity[A]}
func name(x Name) String { x }
func pair[A](x A) Pair[A] { (x, x) }
func nested() Nested[Int] { ((1, 2), (3, 4)) }
func handle(f Handler, r Request) Response { f(r) }
func annotated() Name { let n: Name = "Ada"; (func(x Name) => x)(n) }
func nestedIdentity(x Identity[Identity[Int]]) Int { x }
func scoped[Name](x Name) Name { x }
`+pass5Main)
	for range 2 {
		p, err := types.Check([]*ast.Module{{Files: files}})
		if err != nil {
			t.Fatal(err)
		}
		for name, want := range map[string]string{
			"name":           "func(String) String",
			"pair":           "func(A) Tuple2[A, A]",
			"nested":         "func() Tuple2[Tuple2[Int, Int], Tuple2[Int, Int]]",
			"handle":         "func(func(Request) Response, Request) Response",
			"annotated":      "func() String",
			"nestedIdentity": "func(Int) Int",
			"scoped":         "func(Name) Name",
		} {
			if got := types.TypeString(p.Funcs[name].Scheme.Type); got != want {
				t.Errorf("%s: %s, want %s", name, got, want)
			}
		}
		if p.DataTypes["Name"] != nil || p.DataTypes["Pair"] != nil {
			t.Fatal("aliases acquired runtime data representations")
		}
		walkExprs(reflect.ValueOf(files), func(e ast.Expr) { assertZonked(t, e) })
	}
}

func TestAliasErrors(t *testing.T) {
	for _, tt := range []struct{ name, source, want string }{
		{"missing arguments", `alias Pair[A] = (A, A); func f(x Pair) Int { 1 }`, "arity mismatch for alias Pair: want 1 arguments, got 0"},
		{"too many arguments", `alias Pair[A] = (A, A); func f(x Pair[Int, String]) Int { 1 }`, "arity mismatch for alias Pair: want 1 arguments, got 2"},
		{"zero arity", `alias Name = String; func f(x Name[Int]) Int { 1 }`, "arity mismatch for alias Name: want 0 arguments, got 1"},
		{"unused direct cycle", `alias Loop = Loop`, "type alias cycle involving Loop"},
		{"unused indirect cycle", `alias A = B; alias B = List[A]`, "type alias cycle involving A"},
		{"generic cycle", `alias Loop[A] = Loop[List[A]]`, "type alias cycle involving Loop"},
		{"unknown target", `alias Name = Missing`, "unknown type Missing"},
		{"duplicate parameter", `alias Pair[A, A] = A`, "duplicate type parameter A"},
		{"alias collision", `alias Name = Int; alias Name = String`, "duplicate declaration Name"},
		{"data collision", `type Name = Named; alias Name = String`, "duplicate declaration Name"},
		{"reverse data collision", `alias Name = String; type Name = Named`, "duplicate declaration Name"},
		{"class collision", `class Name[A] {}; alias Name = String`, "duplicate declaration Name"},
		{"reverse class collision", `alias Name = String; class Name[A] {}`, "duplicate declaration Name"},
		{"builtin collision", `alias Int = String`, "duplicate declaration Int"},
		{"tuple collision", `alias Tuple2 = String`, "duplicate declaration Tuple2"},
		{"expanded diagnostic", `alias Name = String; func f() Name { 1 }`, "type mismatch: expected String, got Int"},
		{"argument kind", `alias Pair[A] = (A, A); func f(x Pair[List]) Int { 1 }`, "kind mismatch: got (*) -> *, want *"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			files := parseFiles(t, tt.source+pass5Main)
			files[1].Name = "aliases.gfx"
			p, err := types.Check([]*ast.Module{{Files: files}})
			var typed *types.Error
			if p != nil || !errors.As(err, &typed) || typed.Msg != tt.want {
				t.Fatalf("Check = (%v, %v), want %q", p, err, tt.want)
			}
			if typed.File != "aliases.gfx" || typed.Pos.Line == 0 || typed.Pos.Col == 0 {
				t.Fatalf("unpositioned diagnostic: %#v", typed)
			}
		})
	}
}

func TestAliasInstanceHeads(t *testing.T) {
	files := parseFiles(t, `
class Label[A] { label(x A) String }
alias Name = String
instance Label[Name] { label(x) { x } }
type Box[A, B] = Box{first: A, second: B}
alias Flipped[A, B] = Box[B, A]
instance Label[Flipped[A, B]] given Show[A], Show[B] {
 label(x) { show(x.first) + show(x.second) }
}
alias Boxes[A] = Box[A]
instance Functor[Boxes[E]] {
 fmap(x, f) { Box(x.first, f(x.second)) }
}
func display() String { label(Box(1, 2)) + label("name") }
func mapped() Box[String, Int] { fmap(Box("name", 2), func(n) => n + 1) }
`+pass5Main)
	for range 2 {
		p, err := types.Check([]*ast.Module{{Files: files}})
		if err != nil {
			t.Fatal(err)
		}
		dict := types.AnnOf(p.Funcs["mapped"].Decl.Body.Result.(*ast.Call).Fn).Dicts[0]
		assertInstanceDict(t, dict, "Functor", "Box")
		walkExprs(reflect.ValueOf(files), func(e ast.Expr) { assertZonked(t, e) })
	}
}

func TestAliasInstanceHeadErrors(t *testing.T) {
	for _, tt := range []struct{ source, want string }{
		{`alias Handler = func(Int) Int; instance Label[Handler] {}`, "instance head must be a type constructor"},
		{`alias Identity[A] = A; instance Label[Identity[A]] {}`, "instance head must be a type constructor"},
		{`alias Pair[A] = (A, A); instance Label[Pair[A]] {}`, "instance head must be a type constructor"},
		{`alias Names = List[String]; instance Label[Names] {}`, "instance head must be a type constructor"},
		{`alias Nested[A] = List[List[A]]; instance Label[Nested[A]] {}`, "instance head must be a type constructor"},
		{`alias Erased[A] = Int; instance Label[Erased[A]] {}`, "instance head must be a type constructor"},
		{`alias Names[A] = List[A]; instance Label[Names] {}`, "arity mismatch for alias Names: want 1 arguments, got 0"},
		{`alias Name = Int; instance Label[Name] {}; instance Label[Int] {}`, "duplicate instance Label[Int]"},
	} {
		t.Run(tt.source, func(t *testing.T) {
			p, err := types.Check([]*ast.Module{{Files: parseFiles(t, "class Label[A] {}; "+tt.source+pass5Main)}})
			var typed *types.Error
			if p != nil || !errors.As(err, &typed) || typed.Msg != tt.want {
				t.Fatalf("Check = (%v, %v), want %q", p, err, tt.want)
			}
		})
	}
}

func TestCommonFields(t *testing.T) {
	files := parseFiles(t, `
alias Name = String
type Shape[A] = Circle{id: Int, value: A, name: Name} | Rect{name: String, value: A, id: Int, w: Int}
alias Drawing[A] = Shape[A]
func id[A](s Drawing[A]) Int { s.id }
func value[A](s Shape[A]) A { s.value }
func reset[A](s Shape[A], v A) Shape[A] { s{value: v, id: 0} }
func callback(s Shape[Int]) Int { (func(x Shape[Int]) => x.id)(s) }
type Choice[A] = One{run: func(A) A, pair: (A, Name)} | Two{pair: (A, String), run: func(A) A}
func run[A](x Choice[A], a A) A { x.run(a) }
func pair[A](x Choice[A]) (A, String) { x.pair }
`+pass5Main)
	p, err := types.Check([]*ast.Module{{Files: files}})
	if err != nil {
		t.Fatal(err)
	}
	if got := types.TypeString(p.Funcs["value"].Scheme.Type); got != "func(Shape[A]) A" {
		t.Fatal(got)
	}
	for name, want := range map[string][][]int{
		"id": {{0}, {2}}, "value": {{1}, {1}}, "reset": {{1, 0}, {1, 2}},
	} {
		ann := types.AnnOf(p.Funcs[name].Decl.Body.Result)
		if len(ann.RecordCases) != len(want) {
			t.Fatalf("%s cases = %#v", name, ann.RecordCases)
		}
		for i, indexes := range want {
			if ann.RecordCases[i].Ctor != p.DataTypes["Shape"].Ctors[i] || !reflect.DeepEqual(ann.RecordCases[i].Indexes, indexes) {
				t.Fatalf("%s case %d = %#v, want %v", name, i, ann.RecordCases[i], indexes)
			}
		}
	}
	walkExprs(reflect.ValueOf(files), func(e ast.Expr) { assertZonked(t, e) })
}

func TestCommonFieldErrors(t *testing.T) {
	for _, tt := range []struct{ source, want string }{
		{`func f(x Int) Int { x.a }`, "field a requires a record type"},
		{`func f(x Int) Int { x{a: 1} }`, "field a requires a record type"},
		{`type R = R{a: Int}; func f(x R) Int { x.b }`, "unknown field b in constructor R"},
		{`type R = R(Int); func f(x R) Int { x.a }`, "unknown field a in constructor R"},
		{`type R = First{a: Int} | Empty; func f(x R) R { x{a: 1} }`, "unknown field a in constructor Empty"},
		{`type R = First{a: Int} | Empty | Last{b: Int}; func f(x R) Int { x.a }`, "unknown field a in constructor Empty"},
		{`type R = Empty | Last{a: Int}; func f(x R) Int { x.a }`, "unknown field a in constructor Empty"},
		{`type R = First{a: Int, b: Int} | Last{a: Int}; func f(x R) R { x{a: 1, b: 2} }`, "unknown field b in constructor Last"},
		{`type R = First{a: Int} | Last{a: String}; func f(x R) Int { x.a }`, "field a has type String in constructor Last; expected Int from constructor First"},
		{`type R = First{a: Int} | Last{a: String}; func f(x R) R { x{a: 1} }`, "field a has type String in constructor Last; expected Int from constructor First"},
		{`type R[A, B] = First{a: A} | Last{a: B}; func f[A, B](x R[A, B]) A { x.a }`, "field a has type B in constructor Last; expected A from constructor First"},
		{`type R = First{a: Int} | Last{a: Int}; func f(x R) R { x{a: "bad"} }`, "type mismatch: expected Int, got String"},
		{`type R = First{a: Int} | Last{a: Int}; func f(x R) R { x{a: 1, a: 2} }`, "duplicate field a in constructor First"},
	} {
		t.Run(tt.source, func(t *testing.T) {
			p, err := types.Check([]*ast.Module{{Files: parseFiles(t, tt.source+pass5Main)}})
			var typed *types.Error
			if p != nil || !errors.As(err, &typed) || typed.Msg != tt.want {
				t.Fatalf("Check = (%v, %v), want %q", p, err, tt.want)
			}
		})
	}
}
