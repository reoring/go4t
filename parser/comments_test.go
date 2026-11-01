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

func TestFileCommentsAndSurfaceSyntax(t *testing.T) {
	file, err := parser.Parse("// header\npackage main // package\nfunc f() Int { 1 |> add(_, 2) } // function\n")
	if err != nil {
		t.Fatal(err)
	}
	if len(file.Comments) != 3 || file.Comments[0].Text != "// header" || file.Comments[0].Trailing || !file.Comments[1].Trailing || !file.Comments[2].Trailing {
		t.Fatalf("comments=%#v", file.Comments)
	}
	if file.PackagePos != (ast.Pos{Line: 2, Col: 1}) || file.PackageEnd != (ast.Pos{Line: 2, Col: 13}) {
		t.Fatalf("package span=%v..%v", file.PackagePos, file.PackageEnd)
	}
	call := file.Decls[0].(*ast.FuncDecl).Body.Result.(*ast.Call)
	if call.Pipe == nil || call.Pipe.Op != "|>" {
		t.Fatal("pipe surface not retained")
	}
	partial := call.Pipe.R.(*ast.Lambda)
	if !partial.Partial || partial.Section {
		t.Fatal("partial-call surface not retained")
	}
	hole := partial.Body.(*ast.Call).Args[0].(*ast.Ident)
	if !hole.Placeholder || hole.Name != partial.Params[0].Name {
		t.Fatal("lowered placeholder binding changed")
	}
}

func TestInterpolationComments(t *testing.T) {
	file, err := parser.Parse("package main\nfunc f() String { \"${ // inside\n1}\" }\n")
	if err != nil {
		t.Fatal(err)
	}
	if len(file.Comments) != 1 || file.Comments[0].Text != "// inside" || !file.Comments[0].Trailing {
		t.Fatalf("comments=%#v", file.Comments)
	}
	interp := file.Decls[0].(*ast.FuncDecl).Body.Result.(*ast.Interp)
	if len(interp.Starts) != 1 || interp.Starts[0] != (ast.Pos{Line: 2, Col: 22}) {
		t.Fatalf("starts=%v", interp.Starts)
	}
}
