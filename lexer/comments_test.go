package lexer_test

import (
	"reflect"
	"testing"

	"github.com/reoring/go4t/ast"
	"github.com/reoring/go4t/lexer"
)

func TestComments(t *testing.T) {
	l := lexer.New("// hé\npackage main // package\n// next\nfunc f() Int { 1 } // end")
	toks, err := l.All()
	if err != nil {
		t.Fatal(err)
	}
	want := []ast.Comment{
		{Pos: ast.Pos{Line: 1, Col: 1}, End: ast.Pos{Line: 1, Col: 6}, Text: "// hé"},
		{Pos: ast.Pos{Line: 2, Col: 14}, End: ast.Pos{Line: 2, Col: 24}, Text: "// package", Trailing: true},
		{Pos: ast.Pos{Line: 3, Col: 1}, End: ast.Pos{Line: 3, Col: 8}, Text: "// next"},
		{Pos: ast.Pos{Line: 4, Col: 20}, End: ast.Pos{Line: 4, Col: 26}, Text: "// end", Trailing: true},
	}
	if !reflect.DeepEqual(l.Comments, want) {
		t.Fatalf("comments=%#v; want %#v", l.Comments, want)
	}
	plain := lexer.New("\npackage main\n\nfunc f() Int { 1 }")
	without, err := plain.All()
	if err != nil {
		t.Fatal(err)
	}
	if len(toks) != len(without) {
		t.Fatalf("comments changed token count: %d != %d", len(toks), len(without))
	}
	for i, tok := range toks {
		if tok.Kind != without[i].Kind || tok.Text != without[i].Text {
			t.Errorf("token %d: %v != %v", i, tok, without[i])
		}
	}
}
