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
	f.Fuzz(func(t *testing.T, src []byte) {
		// Syntax errors are expected; unexpected panics must reach the fuzzer.
		_, _ = parser.Parse(string(src))
	})
}
