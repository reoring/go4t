package parser_test

import (
	"os"
	"testing"

	"github.com/reoring/go4t/parser"
)

func FuzzParser(f *testing.F) {
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
	for _, src := range []string{"", "\x00\xff", "package p", "package p\nfunc f() Int {", "package p\nfunc f() Int { (((1))) }", "package p\nfunc f() String { \"${1}\" }"} {
		f.Add([]byte(src))
	}
	for _, expr := range []string{
		"a\n- b", "a -\nb", "(a\n- b)", "x == y == z", "a < b == c", "(a < b) == c",
		"1_000_000", "0xFF_FF", "0b1010", "0o755", "-9223372036854775808", "-0x8000000000000000",
		"9223372036854775808", "0xFFFFFFFFFFFFFFFFF", "1__0", "1_", "0x", "0b2", "1abc", "1_000.5",
	} {
		f.Add([]byte("package p\nfunc f() T { " + expr + " }"))
	}
	f.Fuzz(func(t *testing.T, src []byte) {
		// Syntax errors are expected; unexpected panics must reach the fuzzer.
		_, _ = parser.Parse(string(src))
	})
}
