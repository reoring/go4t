package format_test

import (
	"strings"
	"testing"

	"github.com/reoring/go4t/format"
)

const lawSurfaceFixture = `package main
// implicit
law implicit(a Int) { a + 0 == a }
// inline
law inline(a Int) requires a > 0 { a > 0 } proof { refl }
// separate
law separate(a Bool) { (a) == true }
proof separate(a) { refl }
law several(a Int) requires a > 0 requires a < 10 { a > 0 }
law long(argumentWithALongName Int) requires argumentWithALongName > 0 && argumentWithALongName < 100 { argumentWithALongName > 0 }
law predicate(a Bool, b Bool) { a && (b == b) && a }
`

func TestLawSurfaceRoundTrip(t *testing.T) {
	assertProperties(t, lawSurfaceFixture)
	for _, src := range []string{
		"law l(a Int) { a == a }\n// between\nproof { refl }",
		"// before\nlaw l(a Int) { a == a } // closing\nproof { refl }",
		"law l(a Int) { a == a } proof // proof keyword\n{ refl }",
		"law l(a Int) requires a > 0 // first\nrequires a < 10 // second\n{ a > 0 } proof { refl }",
		"law l(a Bool) { (a) == true }",
		"law l(a Bool) { a }",
		"law l(a Bool, b Bool) { (a == b) |> f } proof { sample(10) }",
		"law l(a Bool, b Bool) { a || (b && a == b) }",
	} {
		t.Run(src, func(t *testing.T) { assertProperties(t, "package main\n"+src) })
	}
	for _, tactic := range []string{"refl", "match a { None => refl; Some(x) => refl }", "if a { refl } else { refl }", "rewrite other(a) { refl }", "induction a", "induction a { Zero => refl; Succ(x) => refl }", "sample(10)"} {
		assertProperties(t, "package main; law l(a Int) { a == a } proof { "+tactic+" }")
	}
}

func TestLawSurfaceLayout(t *testing.T) {
	src := "package main; law l(a Int) requires a > 0 requires a < 10 { a > 0 } proof { refl }; law s(a Int) requires a > 0 { a > 0 }"
	want := "package main\n\nlaw l(a Int)\n  requires a > 0\n  requires a < 10\n{\n  a > 0\n} proof {\n  refl\n}\n\nlaw s(a Int) requires a > 0 {\n  a > 0\n}\n"
	got, err := format.Source([]byte(src))
	if err != nil || string(got) != want {
		t.Fatalf("format = %s, %v; want %s", got, err, want)
	}
	long := "law l(a Int) requires " + strings.Repeat("a > 0 && ", 10) + "a < 100 { a > 0 }"
	got, err = format.Source([]byte("package main; " + long))
	if err != nil || !strings.Contains(string(got), "law l(a Int)\n  requires ") || !strings.Contains(string(got), "\n{\n") {
		t.Fatalf("long clause = %s, %v", got, err)
	}
	assertProperties(t, "package main; "+long)
}
