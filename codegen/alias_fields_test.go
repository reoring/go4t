package codegen

import (
	"strings"
	"testing"

	"gofx/ast"
	"gofx/parser"
	"gofx/types"
)

func TestAliasCommonFieldsCompile(t *testing.T) {
	file, err := parser.Parse(`package main
alias Name = String
alias Pair[A] = (A, A)
alias Handler = func(Request) Response
type Request = Request{name: Name}
type Response = Response{message: Name}
type Shape[A] = Circle{id: Int, value: A} | Rect{value: A, w: Int, id: Int}
func handler(f Handler, r Request) Response { f(r) }
func pair[A](x A) Pair[A] { (x, x) }
func id[A](s Shape[A]) Int { s.id }
func value[A](s Shape[A]) A { s.value }
func reset[A](s Shape[A]) Shape[A] { s{id: 0} }
func both[A](s Shape[A], v A) Shape[A] { s{value: v, id: 0} }
func base() Shape[Int] { Circle{id: 7, value: 8} }
func replacement(n Int) Int { n }
func evaluated() Shape[Int] { base(){value: replacement(1), id: replacement(2)} }
func fromLiteral() Int { Rect{value: 3, w: 4, id: 5}.id }
func fromUpdated() Int { reset(Rect{value: 3, w: 4, id: 5}).id }
func main() IO[Unit] { main() }
`)
	if err != nil {
		t.Fatal(err)
	}
	// This fixture only compiles generated Go; the runnable slice example
	// exercises actual IO and both constructors end to end.
	p, err := types.Check([]*ast.Module{{Files: []*ast.File{file}}})
	if err != nil {
		t.Fatal(err)
	}
	out := assertSource(t, p,
		"switch ", ".(type)", "case *Shape_Circle:", "case *Shape_Rect:",
		"type Tuple2 struct", "func handler_(f_ any, r_ any) any")
	for _, name := range []string{"Name", "Pair", "Handler"} {
		if strings.Contains(out, "type "+name+" ") {
			t.Errorf("transparent alias %s emitted a Go type", name)
		}
	}
	for _, call := range []string{"= base_()", "= replacement_(int64(1))", "= replacement_(int64(2))"} {
		if strings.Count(out, call) != 1 {
			t.Errorf("%s must be evaluated once:\n%s", call, out)
		}
	}
	base, first, second := strings.Index(out, "= base_()"), strings.Index(out, "= replacement_(int64(1))"), strings.Index(out, "= replacement_(int64(2))")
	if !(base < first && first < second) {
		t.Fatal("receiver and replacement evaluations lost source order")
	}
	for _, name := range []string{"id", "value", "reset", "both"} {
		ann := types.AnnOf(p.Funcs[name].Decl.Body.Result)
		if len(ann.RecordCases) != 2 {
			t.Fatalf("%s cases = %#v", name, ann.RecordCases)
		}
	}
}

func TestAliasInstanceCompile(t *testing.T) {
	file, err := parser.Parse(`package main
class Label[A] { label(x A) String }
alias Name = String
instance Label[Name] { label(x) { x } }
type Box[A, B] = Box{first: A, second: B}
alias Flipped[A, B] = Box[B, A]
instance Label[Flipped[A, B]] given Label[A], Label[B] {
 label(x) { label(x.first) + label(x.second) }
}
func display() String { label(Box("a", "b")) }
func main() IO[Unit] { main() }
`)
	if err != nil {
		t.Fatal(err)
	}
	p, err := types.Check([]*ast.Module{{Files: []*ast.File{file}}})
	if err != nil {
		t.Fatal(err)
	}
	assertSource(t, p, "type Label_String struct", "type Label_Box struct", "dict_A Label", "dict_B Label")
}
