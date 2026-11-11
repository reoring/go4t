package format_test

import (
	"strings"
	"testing"

	"github.com/reoring/go4t/format"
)

func TestDeclsFormatting(t *testing.T) {
	for _, source := range []string{
		`type Row[A,] = {qty: A} deriving Eq`,
		`type Row = Row{qty: Int}`,
		`let limit = 10`,
		`let limit: Int = base + 1`,
		`alias Pair[A, B,] = (A, B,)`,
		`class C[A,] { method(x A) Int }`,
		`func f[A,](x A) (A, A,) { (x, x,) }`,
		`func f() Int { map(xs, (_ + 1)) }`,
		`func f() Int { xs |> map((_ + 1)) }`,
		`func f() Int { fold(xs, 0, _ + _) }`,
		`func f() Int { f(_, _.x, (0 < _)) }`,
	} {
		assertProperties(t, "package main\n"+source+"\n")
	}
	got, err := format.Source([]byte("package main\nfunc f() Int { map(xs, (_ + 1)) }"))
	if err != nil || !strings.Contains(string(got), "map(xs, _ + 1)") {
		t.Fatalf("formatted = %s, %v", got, err)
	}
}
