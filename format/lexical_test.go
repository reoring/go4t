package format_test

import (
	"strings"
	"testing"

	"github.com/reoring/go4t/format"
	"github.com/reoring/go4t/parser"
)

func TestLexicalLiteralRoundTrip(t *testing.T) {
	for _, literal := range []string{
		"0", "007", "1_000_000", "0xFF_FF", "0b1010", "0o755", "0X_FF", "0B_10", "0O_755",
		"9223372036854775807", "-9223372036854775808", "-0x8000000000000000", "-9_223_372_036_854_775_808",
	} {
		source := "package p; func f() Int { let x = " + literal + "; x }"
		assertProperties(t, source)
		out, err := format.Source([]byte(source))
		if err != nil || !strings.Contains(string(out), "let x = "+literal+"\n") {
			t.Errorf("%q: %s, %v", literal, out, err)
		}
	}
	for _, expression := range []string{"-1", "-0xFF", "-9223372036854775808", "-0x8000000000000000", "-9223372036854775808 + 1", "-9223372036854775808 |> f"} {
		source := "package p; func f() Int { 1; " + expression + " }"
		assertProperties(t, source)
		out, err := format.Source([]byte(source))
		if err != nil || !strings.Contains(string(out), "\n  ("+expression+")\n") {
			t.Errorf("%q: %s, %v", expression, out, err)
		}
	}
	for _, expression := range []string{"(-9223372036854775808)(1)", "(-0x8000000000000000).0", "(-9223372036854775808){x: 1}"} {
		assertProperties(t, "package p; func f() T { "+expression+" }")
	}
}

func TestLexicalComparisonRoundTrip(t *testing.T) {
	for _, expression := range []string{"(a < b) == c", "a == (b < c)", "(a < b) == (c < d)", "((a < b) == _)"} {
		source := "package p; func f() Bool { " + expression + " }"
		assertProperties(t, source)
		out, err := format.Source([]byte(source))
		if err != nil || !strings.Contains(string(out), "\n  "+expression+"\n") {
			t.Errorf("%q: %s, %v", expression, out, err)
		}
	}
	assertProperties(t, "package p; law l() { (a < b) == (c < d) }")
}

func TestLexicalFormatDiagnostics(t *testing.T) {
	for _, expression := range []string{
		"x == y == z", "a < b == c", "a\n- b", "9223372036854775808", "0xFFFFFFFFFFFFFFFFF",
		"1__0", "1_", "0x", "0b2", "1abc", "1_000.5",
	} {
		source := "package p; func f() T { " + expression + " }"
		_, parseErr := parser.Parse(source)
		_, formatErr := format.Source([]byte(source))
		if parseErr == nil || formatErr == nil || parseErr.Error() != formatErr.Error() {
			t.Errorf("%q: parse %v, format %v", expression, parseErr, formatErr)
		}
	}
	_, err := format.Source([]byte("package p\nfunc f(x Int) Int {\n  let y = x\n  -y\n}"))
	if err == nil || err.Error() != `4:3: ambiguous "-" at the start of a line: end the previous line with "-" to subtract, or write (-x) for a negative value` {
		t.Errorf("error = %v", err)
	}
}
