package parser_test

import (
	"strings"
	"testing"

	"gofx/ast"
	"gofx/parser"
)

func TestFloatExpressions(t *testing.T) {
	for _, tt := range []struct {
		source string
		want   float64
	}{{"1.5", 1.5}, {"2e3", 2000}, {"1.0e-3", 0.001}, {"2E+3", 2000}, {"5e-324", 5e-324}} {
		expr, err := parser.ParseExpr(tt.source, ast.Pos{Line: 3, Col: 5})
		if err != nil {
			t.Fatal(err)
		}
		literal, ok := expr.(*ast.FloatLit)
		if !ok || literal.Value != tt.want || literal.GetPos() != (ast.Pos{Line: 3, Col: 5}) {
			t.Fatalf("%q = %#v", tt.source, expr)
		}
	}
	for _, source := range []string{"1.", ".5", "1e309"} {
		_, err := parser.ParseExpr(source, ast.Pos{Line: 3, Col: 5})
		if err == nil || !strings.HasPrefix(err.Error(), "3:") {
			t.Errorf("invalid %q: error = %v", source, err)
		}
	}
}

func TestFloatAndTupleParsing(t *testing.T) {
	expr, err := parser.ParseExpr("t.0.1 + 1.5", ast.Pos{Line: 1, Col: 1})
	if err != nil {
		t.Fatal(err)
	}
	binary := expr.(*ast.Binary)
	outer := binary.L.(*ast.Field)
	inner := outer.X.(*ast.Field)
	if outer.Index != 1 || inner.Index != 0 || binary.R.(*ast.FloatLit).Value != 1.5 {
		t.Fatalf("tuple access lost its indexes: %#v", expr)
	}
	match := result(t, "match x { 1.5 => true; _ => false }").(*ast.Match)
	if match.Arms[0].Pat.(*ast.PLit).X.(*ast.FloatLit).Value != 1.5 {
		t.Fatal("float pattern lost its value")
	}
}
