package parser_test

import (
	"strings"
	"testing"

	"github.com/reoring/go4t/ast"
	"github.com/reoring/go4t/parser"
)

func TestOrPatterns(t *testing.T) {
	for _, source := range []string{
		"A | B", "Some(A | B)", "(A | B, C | D,)",
		"R{value: A | B}", "[A | B, ..rest]", "whole @ (A | B)",
		"whole @ A | whole @ B", "(A | B) | C", "A | (B | C)",
		"-1 | -2", "-2.5 | 3.5", `"a" | "b"`,
	} {
		t.Run(source, func(t *testing.T) {
			m := result(t, "match x { | "+source+" => 1 | _ => 2 }").(*ast.Match)
			if len(m.Arms) != 2 {
				t.Fatalf("arms = %d, want 2", len(m.Arms))
			}
			found := false
			ast.Walk(m.Arms[0].Pat, func(n ast.Node) {
				if _, ok := n.(*ast.POr); ok {
					found = true
				}
			})
			if !found {
				t.Fatal("missing POr")
			}
		})
	}
	m := result(t, "match x { -1 => 0; -2.5 => 1 }").(*ast.Match)
	if m.Arms[0].Pat.(*ast.PLit).X.(*ast.IntLit).Value != -1 || m.Arms[1].Pat.(*ast.PLit).X.(*ast.FloatLit).Value != -2.5 {
		t.Fatal("negative patterns are not negative literal values")
	}
	m = result(t, "match x { (a, b,) => a }").(*ast.Match)
	if len(m.Arms[0].Pat.(*ast.PTuple).Elems) != 2 {
		t.Fatal("trailing comma added a tuple element")
	}
}

func TestOrPatternErrors(t *testing.T) {
	for _, tt := range []struct{ pattern, want string }{
		{"-x", `expected a number after "-" in a pattern`},
		{"-true", `expected a number after "-" in a pattern`},
		{"-\"x\"", `expected a number after "-" in a pattern`},
		{"(x,)", "a tuple needs at least two elements"},
	} {
		_, err := parser.ParseExpr("match x { "+tt.pattern+" => 1 }", ast.Pos{Line: 1, Col: 1})
		if err == nil || !strings.HasSuffix(err.Error(), ": "+tt.want) {
			t.Errorf("%s: error = %v, want %q", tt.pattern, err, tt.want)
		}
	}
}
