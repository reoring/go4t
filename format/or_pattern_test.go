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

// A line cannot start with "-", so the formatter's own output must not put a
// negative literal pattern at the start of an arm's line.
func TestNegativeLiteralArmStaysParseable(t *testing.T) {
	src := "package main\nfunc f(n Int) Int { match n { 0 => 1 | -1 | -0x2 => 2 | -2.5 => 3 | _ => 0 } }\n"
	out, err := format.Source([]byte(src))
	if err != nil {
		t.Fatal(err)
	}
	again, err := format.Source(out)
	if err != nil {
		t.Fatalf("formatted source does not parse: %v\n%s", err, out)
	}
	if string(again) != string(out) {
		t.Fatalf("not idempotent:\n%s\n---\n%s", out, again)
	}
	if !strings.Contains(string(out), "\n    | -1 | -0x2 => 2\n    | -2.5 => 3\n") {
		t.Fatalf("unexpected layout:\n%s", out)
	}
}
