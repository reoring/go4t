package parser_test

import (
	"strings"
	"testing"

	"gofx/ast"
	"gofx/parser"
)

func TestListPatterns(t *testing.T) {
	for _, tt := range []struct{ source, want string }{
		{"[]", "Nil()"},
		{"[a]", "Cons(a, Nil())"},
		{"[a, b, c]", "Cons(a, Cons(b, Cons(c, Nil())))"},
		{"[a, b, ..rest]", "Cons(a, Cons(b, rest))"},
		{"[a, ..]", "Cons(a, _)"},
		{"[a, .._]", "Cons(a, _)"},
		{"[a, .._rest,]", "Cons(a, _rest)"},
		{"[a, b,]", "Cons(a, Cons(b, Nil()))"},
		{"[\na,\nb,\n..rest,\n]", "Cons(a, Cons(b, rest))"},
		{"[[], [a, ..rest]]", "Cons(Nil(), Cons(Cons(a, rest), Nil()))"},
		{"[Some(a), (b, _), whole @ Cons(c, tail)]", "Cons(Some(a), Cons((b, _), Cons(whole @ Cons(c, tail), Nil())))"},
	} {
		t.Run(tt.source, func(t *testing.T) {
			m := result(t, "match xs { "+tt.source+" => 0 }").(*ast.Match)
			if got := patternShape(m.Arms[0].Pat); got != tt.want {
				t.Fatalf("pattern = %s, want %s", got, tt.want)
			}
		})
	}
}

func TestListPatternBindings(t *testing.T) {
	lambda := result(t, "func([a, b]) => a + b").(*ast.Lambda)
	if got := patternShape(lambda.Params[0].Pat); got != "Cons(a, Cons(b, Nil()))" {
		t.Fatalf("lambda pattern = %s", got)
	}
	block := result(t, "{ let [a, b] = xs; a + b }").(*ast.BlockExpr).Block
	if got := patternShape(block.Stmts[0].(*ast.LetStmt).Pat); got != "Cons(a, Cons(b, Nil()))" {
		t.Fatalf("let pattern = %s", got)
	}
	do := result(t, "do { [a, ..rest] <- xs; pure(a) }").(*ast.Do)
	if got := patternShape(do.Stmts[0].(*ast.DoBind).Pat); got != "Cons(a, rest)" {
		t.Fatalf("bind pattern = %s", got)
	}
}

func TestListPatternErrors(t *testing.T) {
	for _, tt := range []struct{ source, want string }{
		{"[a, ..rest, b]", ".. must be last in list pattern"},
		{"[a, .., b]", ".. must be last in list pattern"},
		{"[a, .._, b]", ".. must be last in list pattern"},
		{"[a, ..rest, ..other]", ".. must be last in list pattern"},
		{"[a, ..rest b]", ".. must be last in list pattern"},
		{"[..rest]", "list rest pattern needs a preceding element"},
		{"[..]", "list rest pattern needs a preceding element"},
		{"[a, ..Some]", "list rest must be a lower-case name or _"},
		{"[a, ..[b]]", "list rest must be a lower-case name or _"},
		{"[a, ..1]", "list rest must be a lower-case name or _"},
		{"[a b]", `expected "]", found "b"`},
	} {
		t.Run(tt.source, func(t *testing.T) {
			_, err := parser.ParseExpr("match xs { "+tt.source+" => 0 }", ast.Pos{Line: 1, Col: 1})
			if err == nil || !strings.HasSuffix(err.Error(), ": "+tt.want) {
				t.Fatalf("error = %v, want %q", err, tt.want)
			}
		})
	}
}
