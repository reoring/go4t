package parser_test

import (
	"gofx/ast"
	"gofx/parser"
	"testing"
)

func TestSourceCommentsForTooling(t *testing.T) {
	source := "package main // trailing\r\n// first\r\n// second\r\nextern func text() String = \"//not-comment\".Text\r\n"
	file, err := parser.Parse(source)
	if err != nil {
		t.Fatal(err)
	}
	if file.Source != source {
		t.Fatal("original source not retained")
	}
	if len(file.Comments) != 3 {
		t.Fatalf("comments: %+v", file.Comments)
	}
	for i, c := range file.Comments {
		if c.Trailing != (i == 0) || c.Pos.Line != i+1 || c.End.Line != i+1 {
			t.Errorf("comment %d: %+v", i, c)
		}
	}
	if file.Comments[1].Pos != (ast.Pos{Line: 2, Col: 1}) {
		t.Fatal(file.Comments[1])
	}
}
