package format_test

import "testing"

func TestLawRequiresRoundTrip(t *testing.T) {
	for _, source := range []string{
		"law bounded(x Int) requires x < 10 && x >= 0 { x + 1 == 1 + x }",
		"law bounded(x Int)\n requires x < 10 { x == x }",
		"law bounded(x Int) requires (if x < 10 { true } else { false }) { x == x }",
		"law bounded(x Int) requires { let bound = 10; x < bound } { x == x }",
	} {
		assertProperties(t, "package main\n"+source+"\nproof bounded(x) { refl }\n")
	}
}
