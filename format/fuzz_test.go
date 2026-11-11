package format_test

import (
	"bytes"
	"os"
	"reflect"
	"testing"

	"github.com/reoring/go4t/format"
	"github.com/reoring/go4t/parser"
)

func FuzzFormat(f *testing.F) {
	for _, path := range []string{
		"../examples/tree/main.gt",
		"../std/list/list.gt",
		"../std/string/string.gt",
		"../laws/testdata/semantics.gt",
		"../laws/testdata/tactics_forms.gt",
	} {
		src, err := os.ReadFile(path)
		if err != nil {
			f.Fatal(err)
		}
		f.Add(src)
	}
	for _, src := range []string{"", "\x00\xff", "package p", "package p\nfunc f() Int { 1 }", "// header\npackage p // tail\n", "package p\nfunc f() String { \"${1}\" }", "package p\nfunc f() Int { 1; (-2) }", "package p\nfunc f() T { x |> g(_, 1) }"} {
		f.Add([]byte(src))
	}
	for _, src := range tacticsFormatSeeds {
		f.Add([]byte(src))
	}
	f.Fuzz(func(t *testing.T, src []byte) {
		before, err := parser.Parse(string(src))
		if err != nil {
			return
		}
		// Exercise the same Source entry point as go4t fmt.
		first, err := format.Source(src)
		if err != nil {
			t.Fatalf("format parsed source: %v", err)
		}
		after, err := parser.Parse(string(first))
		if err != nil {
			t.Fatalf("formatted parse: %v\n%s", err, first)
		}
		second, err := format.Source(first)
		if err != nil {
			t.Fatalf("format formatted source: %v", err)
		}
		if !bytes.Equal(first, second) {
			t.Fatalf("not idempotent:\nfirst:\n%s\nsecond:\n%s", first, second)
		}
		// Reuse the repository's position/surface-metadata-free AST comparison,
		// including alpha-renaming of synthesized placeholder bindings.
		if !reflect.DeepEqual(structure(before), structure(after)) {
			t.Fatalf("AST changed:\nbefore: %#v\nafter: %#v\nformatted:\n%s", structure(before), structure(after), first)
		}
	})
}
