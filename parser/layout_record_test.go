package parser_test

import (
	"testing"

	"github.com/reoring/go4t/ast"
	"github.com/reoring/go4t/parser"
)

// Record braces are not blocks: newlines inside them must not break parsing
// (the lexer still inserts ';' there; the parser skips it).
func TestMultilineRecords(t *testing.T) {
	src := `package main
type Road = Road{
  from: String,
  to: String,
  minutes: Int
}
type Point = Point{
  x: Int,
  y: Int,
}
func f(r Road) Road {
  let r2 = Road{
    from: r.from,
    to: r.to,
    minutes: r.minutes + 10
  }
  let r3 = r2{
    minutes: 0
  }
  match r3 {
    Road{
      minutes: 0,
      ..
    } => r2
    _ => r3
  }
}
func main() IO[Unit] { println("x") }
`
	f, err := parser.Parse(src)
	if err != nil {
		t.Fatal(err)
	}
	road := f.Decls[0].(*ast.TypeDecl)
	if len(road.Ctors[0].FieldNames) != 3 {
		t.Fatalf("road fields = %v", road.Ctors[0].FieldNames)
	}
	point := f.Decls[1].(*ast.TypeDecl)
	if len(point.Ctors[0].FieldNames) != 2 {
		t.Fatalf("point fields = %v", point.Ctors[0].FieldNames)
	}
	body := f.Decls[2].(*ast.FuncDecl).Body
	if _, ok := body.Stmts[0].(*ast.LetStmt).Value.(*ast.RecordLit); !ok {
		t.Fatalf("r2 is %T, want RecordLit", body.Stmts[0].(*ast.LetStmt).Value)
	}
	if _, ok := body.Stmts[1].(*ast.LetStmt).Value.(*ast.RecordUpdate); !ok {
		t.Fatalf("r3 is %T, want RecordUpdate", body.Stmts[1].(*ast.LetStmt).Value)
	}
	m := body.Result.(*ast.Match)
	if rp, ok := m.Arms[0].Pat.(*ast.PRecord); !ok || !rp.Rest || len(rp.Names) != 1 {
		t.Fatalf("arm pattern = %#v", m.Arms[0].Pat)
	}
}
