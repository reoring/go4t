package format_test

import "testing"

var tacticsFormatSeeds = []string{
	"package main; proof p(x) { absurd }",
	"package main; proof p(x) { match x }",
	"package main; proof p(x) { match (x) }",
	"package main; proof p(x) { match f(x) }",
	"package main; proof p(x) { match f(x) { A | B => refl | C => absurd } }",
	"package main; proof p(x) { match x { true => refl; false => absurd } }",
	"package main; proof p(x) { induction x { A | B => refl } }",
	"package main; proof p(x) { match f(x) { | A => match x | B => induction x } }",
	"package main; proof p(x) { match f(x) { // cases\n A | B => absurd // impossible\n C => refl } }",
}

func TestTacticSurfaceRoundTrip(t *testing.T) {
	for _, source := range tacticsFormatSeeds {
		t.Run(source, func(t *testing.T) { assertProperties(t, source) })
	}
}
