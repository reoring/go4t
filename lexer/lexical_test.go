package lexer_test

import (
	"fmt"
	"testing"

	"github.com/reoring/go4t/ast"
	"github.com/reoring/go4t/lexer"
)

const leadingMinusError = `ambiguous "-" at the start of a line: end the previous line with "-" to subtract, or write (-x) for a negative value`

func TestLexicalLeadingMinus(t *testing.T) {
	for _, tc := range []struct{ source, position string }{
		{"func f(x Int) Int {\n  let y = x\n  -y\n}", "3:3"},
		{"a\n- b", "2:1"},
		{"α // trailing\r\n\n// comment\n  - b", "4:3"},
		{"f({ a\n -b })", "2:2"},
		{"[ { a\n -b } ]", "2:2"},
	} {
		_, err := lexer.New(tc.source).All()
		want := tc.position + ": " + leadingMinusError
		if err == nil || err.Error() != want {
			t.Errorf("%q: got %v, want %s", tc.source, err, want)
		}
	}
	for _, source := range []string{
		"a -\nb", "(a\n- b)", "[a\n- b]", "a;\n-b", "(-a)",
		"a +\n-b", "let x =\n-b", "_ =>\n-b", "(\n-b)", "[\n-b]", "f(a,\n-b)",
		"a\n+b", "a\n*b", "a\n== b", "a\n&& b", "a\n|> b",
	} {
		if _, err := lexer.New(source).All(); err != nil {
			t.Errorf("%q: %v", source, err)
		}
	}
}

func TestLexicalIntegerTokens(t *testing.T) {
	for _, source := range []string{
		"0", "007", "1_000_000", "0xFF_FF", "0X_FF", "0b1010", "0B_10", "0o755", "0O_755",
		"9223372036854775808", "0xFFFFFFFFFFFFFFFFF",
	} {
		got := tokens(t, source)
		if got[0].Kind != lexer.Int || got[0].Text != source || got[0].End != (ast.Pos{Line: 1, Col: len(source) + 1}) {
			t.Errorf("%q: %#v", source, got[0])
		}
	}
}

func TestLexicalNumberDiagnostics(t *testing.T) {
	for _, tc := range []struct{ source, message string }{
		{"1__0", `bad number literal 1__0: "_" must separate digits`},
		{"1_", `bad number literal 1_: "_" must separate digits`},
		{"0x__1", `bad number literal 0x__1: "_" must separate digits`},
		{"0x1_", `bad number literal 0x1_: "_" must separate digits`},
		{"0x", "bad number literal 0x: expected hexadecimal digits after 0x"},
		{"0xG", "bad number literal 0xG: expected hexadecimal digits after 0x"},
		{"0b2", "bad number literal 0b2: expected binary digits after 0b"},
		{"0b102", "bad number literal 0b102: expected binary digits after 0b"},
		{"0o8", "bad number literal 0o8: expected octal digits after 0o"},
		{"1abc", "bad number literal 1abc: a number cannot be followed by a letter"},
		{"1α", "bad number literal 1α: a number cannot be followed by a letter"},
		{"0x1g", "bad number literal 0x1g: a number cannot be followed by a letter"},
		{"1_000.5", "digit separators and base prefixes are not supported in Float literals"},
		{"1_000e2", "digit separators and base prefixes are not supported in Float literals"},
		{"1.0_0", "digit separators and base prefixes are not supported in Float literals"},
		{"1e1_0", "digit separators and base prefixes are not supported in Float literals"},
		{"0x1.2", "digit separators and base prefixes are not supported in Float literals"},
		{"0x1p2", "digit separators and base prefixes are not supported in Float literals"},
	} {
		got, err := lexer.New("\n  " + tc.source).All()
		if err == nil || err.Error() != "2:3: "+tc.message || got != nil {
			t.Errorf("%q: tokens %v, error %v; want %s", tc.source, got, err, tc.message)
		}
	}
}

func TestLexicalIntConversion(t *testing.T) {
	for _, tc := range []struct {
		text     string
		negative bool
		value    int64
	}{
		{"007", false, 7}, {"1_000_000", false, 1000000}, {"0xFF_FF", false, 65535},
		{"0b1010", false, 10}, {"0o755", false, 493}, {"0X_FF", false, 255},
		{"9223372036854775807", false, 1<<63 - 1},
		{"9223372036854775808", true, -1 << 63}, {"0x8000000000000000", true, -1 << 63},
	} {
		got, err := lexer.ParseInt(tc.text, tc.negative)
		if err != nil || got != tc.value {
			t.Errorf("ParseInt(%q, %v) = %d, %v; want %d", tc.text, tc.negative, got, err, tc.value)
		}
	}
	for _, text := range []string{"9223372036854775808", "0xFFFFFFFFFFFFFFFFF", "18446744073709551616"} {
		_, err := lexer.ParseInt(text, false)
		want := fmt.Sprintf("integer literal %s is out of range for Int", text)
		if err == nil || err.Error() != want {
			t.Errorf("%q: %v, want %s", text, err, want)
		}
	}
}
