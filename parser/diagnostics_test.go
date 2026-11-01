package parser_test

import (
	"strings"
	"testing"
	"unicode/utf8"

	"gofx/ast"
	"gofx/parser"
)

func TestEndPositions(t *testing.T) {
	for _, source := range []string{
		`answer`, `"hé\nllo"`, `"hi ${name}!"`, `f(1)(2).name`, `record{field: 1}`,
		`1 + 2 * 3`, `-value`, `if true { 1 } else { 2 }`, `[1, 2]`, `(1, "x")`,
		`func((a, b), named Int) => a + b`, `match x { Some((true, _)) => 1; None => 0 }`,
		`do { value <- action(); let other = 1; result(value) }`,
		`{ let Pair{left: x, right: y} = pair; x }`,
	} {
		t.Run(source, func(t *testing.T) {
			e, err := parser.ParseExpr(source+"  // not part of span", ast.Pos{Line: 3, Col: 4})
			if err != nil {
				t.Fatal(err)
			}
			want := ast.Pos{Line: 3, Col: 4 + utf8.RuneCountInString(source)}
			if e.GetEnd() != want {
				t.Fatalf("end = %s, want %s", e.GetEnd(), want)
			}
			ast.Walk(e, func(n ast.Node) {
				if n.GetEnd() == (ast.Pos{}) {
					t.Errorf("%T has no End", n)
				}
			})
		})
	}
}

func TestDeclarationAndPatternEndPositions(t *testing.T) {
	source := `package main
class Display[A] { display(a A) String }
type Box[A] = Box{value: A}
instance Display[Box[A]] given Display[A] { display(box) { "box" } }
extern func foreign(x String) Int = "native".Read
func main() IO[Unit] {
 let whole @ (first, second) = (1, 2)
 match whole { (left, _) => ioPure(unit()) }
}`
	f, err := parser.Parse(source)
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(source, "\n")
	for i, d := range f.Decls {
		line := i + 2
		if i == len(f.Decls)-1 {
			line = len(lines)
		}
		want := ast.Pos{Line: line, Col: utf8.RuneCountInString(lines[line-1]) + 1}
		if d.GetEnd() != want {
			t.Errorf("%T end = %s, want %s", d, d.GetEnd(), want)
		}
		ast.Walk(d, func(n ast.Node) {
			if n.GetEnd() == (ast.Pos{}) {
				t.Errorf("%T has no End", n)
			}
		})
	}
	block := f.Decls[len(f.Decls)-1].(*ast.FuncDecl).Body
	p := block.Stmts[0].(*ast.LetStmt).Pat.(*ast.PAs)
	if p.End != (ast.Pos{Line: 7, Col: 29}) {
		t.Errorf("as-pattern end = %s", p.End)
	}
	tuple := p.Pat.(*ast.PTuple)
	if tuple.Elems[0].GetEnd() != (ast.Pos{Line: 7, Col: 20}) {
		t.Errorf("variable end = %s", tuple.Elems[0].GetEnd())
	}
}
