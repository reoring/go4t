package types_test

import (
	"strings"
	"testing"

	"github.com/reoring/go4t/types"
)

func TestTacticExpressionTypesAndIndex(t *testing.T) {
	const source = `
func identity(o Option[Int]) Option[Int] { o }
law p(o Option[Int]) { o == o }
proof p(o) { match identity(o) { None => absurd; Some(n) => if n > 0 { refl } else { refl } } }
`
	p, err := checkLawSource(t, source, false)
	if err != nil {
		t.Fatal(err)
	}
	index := p.PositionIndex()
	fullSource := "package main\n" + source + lawMain
	binding := index.At("laws.gt", indexPos(fullSource, "if n >", 3))
	if binding == nil || binding.Definition == nil || types.TypeString(binding.Definition.Type) != "Int" {
		t.Fatalf("expression match binder hover: %+v", binding)
	}
	for _, tc := range []struct{ body, want string }{
		{"match identity(o) { Some(_) => refl }", "non-exhaustive match; missing: None"},
		{"match identity(o) { Some(n) | None => refl }", "or-pattern alternatives must bind the same variables"},
		{"match identity(o) { Some(n) | Some(m) => refl; None => refl }", "or-pattern alternatives must bind the same variables"},
	} {
		_, err := checkLawSource(t, "func identity(o Option[Int]) Option[Int] { o }; law p(o Option[Int]) { o == o }; proof p(o) { "+tc.body+" }", true)
		if err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Fatalf("%s: %v, want %s", tc.body, err, tc.want)
		}
	}
	for _, body := range []string{"match c", "match c { true | false => refl }", "match (c) { true => refl; false => absurd }"} {
		if _, err := checkLawSource(t, "law p(c Bool) { c == c }; proof p(c) { "+body+" }", true); err != nil {
			t.Fatalf("%s: %v", body, err)
		}
	}
}
