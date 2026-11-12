package lexer_test

import (
	"os"
	"testing"
	"unicode/utf8"

	"github.com/reoring/go4t/ast"
	"github.com/reoring/go4t/lexer"
)

// FuzzLexer checks byte input, including invalid UTF-8, without recovering panics.
func FuzzLexer(f *testing.F) {
	for _, path := range []string{
		"../examples/tree/main.gt",
		"../std/list/list.gt",
		"../std/string/string.gt",
		"../laws/testdata/semantics.gt",
	} {
		src, err := os.ReadFile(path)
		if err != nil {
			f.Fatal(err)
		}
		f.Add(src)
	}
	for _, src := range []string{
		"", "\x00\xff", "α\nβ", "x\r\n// end", "1e+", "\"${\"nested\"}\"", "([{}])", "\"unterminated",
		"a\n- b", "a -\nb", "(a\n- b)", "1_000_000", "0xFF_FF", "0b1010", "0o755",
		"-9223372036854775808", "-0x8000000000000000", "9223372036854775808", "0xFFFFFFFFFFFFFFFFF",
		"1__0", "1_", "0x", "0b2", "1abc", "1_000.5",
	} {
		f.Add([]byte(src))
	}
	f.Fuzz(func(t *testing.T, src []byte) {
		tokens, err := lexer.New(string(src)).All()
		if err != nil {
			return
		}
		// Each rune can produce at most a token and an inserted semicolon;
		// EOF and its optional semicolon account for the remaining two.
		if len(tokens) > 2*utf8.RuneCount(src)+2 {
			t.Fatalf("too many tokens: %d for %d bytes", len(tokens), len(src))
		}
		// Columns count runes, not bytes. Include each line's exclusive end,
		// including the empty final line after a newline and EOF itself.
		widths := []int{1}
		for _, r := range string(src) {
			if r == '\n' {
				widths = append(widths, 1)
			} else {
				widths[len(widths)-1]++
			}
		}
		inside := func(p ast.Pos) bool {
			return p.Line >= 1 && p.Line <= len(widths) && p.Col >= 1 && p.Col <= widths[p.Line-1]
		}
		less := func(a, b ast.Pos) bool {
			return a.Line < b.Line || a.Line == b.Line && a.Col < b.Col
		}
		previous := ast.Pos{Line: 1, Col: 1}
		for i, tok := range tokens {
			if !inside(tok.Pos) || !inside(tok.End) || less(tok.Pos, previous) || less(tok.End, tok.Pos) {
				t.Fatalf("token %d: invalid span %v..%v after %v", i, tok.Pos, tok.End, previous)
			}
			previous = tok.End
		}
		if len(tokens) == 0 || tokens[len(tokens)-1].Kind != lexer.EOF {
			t.Fatal("missing EOF")
		}
	})
}
