package format_test

import (
	"strings"
	"testing"

	"github.com/reoring/go4t/format"
)

func TestOrPatternRoundTrip(t *testing.T) {
	for _, pattern := range []string{
		"A|B", "Some(A|B)", "(A|B,C|D,)", "R{value:A|B}",
		"[A|B,..rest]", "whole@(A|B)", "whole@A|whole@B",
		"(A|B)|C", "A|(B|C)", "-1|-2", "-2.5|3.5", `"a"|"b"`,
		"A | // alternative\n B", "Some(A // first\n | B)",
	} {
		t.Run(pattern, func(t *testing.T) {
			assertProperties(t, "package main\nfunc f(x T) Int { match x { | "+pattern+" => 1 | _ => 0 } }")
		})
	}
	source := "package main\nfunc f(x T) Int { match x { A|B if true => 1; C => 0 } }"
	got, err := format.Source([]byte(source))
	if err != nil || !strings.Contains(string(got), "A | B if true => 1") {
		t.Fatalf("format = %s, error = %v", got, err)
	}
}
