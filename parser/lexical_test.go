package parser_test

import (
	"fmt"
	"strconv"
	"strings"
	"testing"

	"github.com/reoring/go4t/ast"
	"github.com/reoring/go4t/parser"
)

func TestLexicalComparisons(t *testing.T) {
	const message = "comparison operators do not chain; parenthesize one comparison, as in (a < b) == c"
	ops := []string{"==", "!=", "<", "<=", ">", ">="}
	for _, first := range ops {
		for _, second := range ops {
			source := "a " + first + " b " + second + " c"
			_, err := parser.ParseExpr(source, ast.Pos{Line: 1, Col: 1})
			want := fmt.Sprintf("1:%d: %s", 6+len(first), message)
			if err == nil || err.Error() != want {
				t.Errorf("%q: %v; want %s", source, err, want)
			}
			for _, grouped := range []string{"(a " + first + " b) " + second + " c", "a " + first + " (b " + second + " c)"} {
				if _, err := parser.ParseExpr(grouped, ast.Pos{Line: 1, Col: 1}); err != nil {
					t.Errorf("%q: %v", grouped, err)
				}
			}
		}
	}
	for _, source := range []string{"a < b && c < d", "a < b || c == d", "a + b < c * d", "(_ < (a < b))", "((a < b) == _)"} {
		if _, err := parser.ParseExpr(source, ast.Pos{Line: 1, Col: 1}); err != nil {
			t.Errorf("%q: %v", source, err)
		}
	}
}

func TestLexicalIntegerExpressions(t *testing.T) {
	for _, tc := range []struct {
		source string
		value  int64
	}{
		{"0", 0}, {"007", 7}, {"1_000_000", 1000000}, {"0xFF_FF", 65535},
		{"0b1010", 10}, {"0o755", 493}, {"0X_FF", 255}, {"0B_10", 2}, {"0O_7", 7},
		{"9223372036854775807", 1<<63 - 1}, {"-9223372036854775808", -1 << 63},
		{"-0x8000000000000000", -1 << 63}, {"-9_223_372_036_854_775_808", -1 << 63},
	} {
		expr, err := parser.ParseExpr(tc.source, ast.Pos{Line: 3, Col: 5})
		if err != nil {
			t.Errorf("%q: %v", tc.source, err)
			continue
		}
		lit, ok := expr.(*ast.IntLit)
		text := tc.source
		if text == strconv.FormatInt(tc.value, 10) {
			text = ""
		}
		if !ok || lit.Value != tc.value || lit.Text != text || lit.Pos != (ast.Pos{Line: 3, Col: 5}) || lit.End != (ast.Pos{Line: 3, Col: 5 + len(tc.source)}) {
			t.Errorf("%q: %#v; want value %d, text %q", tc.source, expr, tc.value, text)
		}
	}
	for _, source := range []string{"-1", "-0xFF", "--9223372036854775808"} {
		expr, err := parser.ParseExpr(source, ast.Pos{Line: 1, Col: 1})
		if _, ok := expr.(*ast.Unary); err != nil || !ok {
			t.Errorf("%q: %#v, %v", source, expr, err)
		}
	}
	for _, text := range []string{"9223372036854775808", "0xFFFFFFFFFFFFFFFFF", "9223372036854775809", "0x8000000000000001"} {
		_, err := parser.ParseExpr(text, ast.Pos{Line: 1, Col: 1})
		want := "1:1: integer literal " + text + " is out of range for Int"
		if err == nil || err.Error() != want {
			t.Errorf("%q: %v, want %s", text, err, want)
		}
	}
	for _, text := range []string{"-9223372036854775809", "-0x8000000000000001", "-(9223372036854775808)"} {
		if _, err := parser.ParseExpr(text, ast.Pos{Line: 1, Col: 1}); err == nil || !strings.Contains(err.Error(), "out of range for Int") {
			t.Errorf("%q: %v", text, err)
		}
	}
}

func TestLexicalIntegerConsumers(t *testing.T) {
	for _, source := range []string{"t.0", "t.01", "t.0.1", "(_.0.1)"} {
		if _, err := parser.ParseExpr(source, ast.Pos{Line: 1, Col: 1}); err != nil {
			t.Errorf("%q: %v", source, err)
		}
	}
	for _, text := range []string{"0x1", "0b1", "0o1", "1_0", "9223372036854775808"} {
		_, err := parser.ParseExpr("t."+text, ast.Pos{Line: 1, Col: 1})
		if err == nil || err.Error() != "1:3: bad tuple index "+text {
			t.Errorf("%q: %v", text, err)
		}
	}
	for _, text := range []string{"1_000", "0x10", "0b10", "0o10"} {
		if _, err := parser.Parse("package p; proof l() { sample(" + text + ") }"); err != nil {
			t.Errorf("sample(%s): %v", text, err)
		}
	}
	for _, text := range []string{"0", "100_001", "0xFFFFFFFFFFFFFFFFF"} {
		_, err := parser.Parse("package p; proof l() { sample(" + text + ") }")
		if err == nil || !strings.Contains(err.Error(), "sample count must be an Int literal in 1..100000") {
			t.Errorf("sample(%s): %v", text, err)
		}
	}
	match := result(t, "match x { 0xFF => 1; _ => 0 }").(*ast.Match)
	if got := match.Arms[0].Pat.(*ast.PLit).X.(*ast.IntLit); got.Value != 255 || got.Text != "0xFF" {
		t.Errorf("pattern: %#v", got)
	}
}

func TestLexicalLayoutParsing(t *testing.T) {
	_, err := parser.Parse("package p\nfunc f(x Int) Int {\n  let y = x\n  -y\n}")
	if err == nil || err.Error() != `4:3: ambiguous "-" at the start of a line: end the previous line with "-" to subtract, or write (-x) for a negative value` {
		t.Errorf("error = %v", err)
	}
	for _, source := range []string{"a -\nb", "(a\n- b)", "{ let y = x; (-y) }"} {
		if _, err := parser.ParseExpr(source, ast.Pos{Line: 1, Col: 1}); err != nil {
			t.Errorf("%q: %v", source, err)
		}
	}
}
