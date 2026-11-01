package parser_test

import (
	"reflect"
	"testing"

	"gofx/ast"
	"gofx/lexer"
)

func TestAliasDeclarations(t *testing.T) {
	f := parse(t, `package main
alias Name = String
alias Pair[A] = (A, A)
alias Handler = func(Request) Response
alias Names = List[Name]
`)
	if len(f.Decls) != 4 {
		t.Fatalf("declarations = %d, want 4", len(f.Decls))
	}
	name := f.Decls[0].(*ast.AliasDecl)
	if name.Name != "Name" || len(name.TParams) != 0 || name.Pos != (ast.Pos{Line: 2, Col: 1}) {
		t.Fatalf("Name = %#v", name)
	}
	typeName(t, name.Type, "String")
	pair := f.Decls[1].(*ast.AliasDecl)
	if pair.Name != "Pair" || !reflect.DeepEqual(pair.TParams, []string{"A"}) {
		t.Fatalf("Pair = %#v", pair)
	}
	tuple := pair.Type.(*ast.TupleType)
	if len(tuple.Elems) != 2 {
		t.Fatalf("Pair body = %#v", tuple)
	}
	for _, elem := range tuple.Elems {
		typeName(t, elem, "A")
	}
	handler := f.Decls[2].(*ast.AliasDecl).Type.(*ast.FuncType)
	if len(handler.Params) != 1 {
		t.Fatalf("Handler = %#v", handler)
	}
	typeName(t, handler.Params[0], "Request")
	typeName(t, handler.Ret, "Response")
	typeName(t, f.Decls[3].(*ast.AliasDecl).Type, "List", "Name")

	tokens, err := lexer.New("alias").All()
	if err != nil || len(tokens) == 0 || tokens[0].Kind != lexer.Keyword {
		t.Fatalf("alias keyword = %v, %v", tokens, err)
	}
}
