package format_test

import (
	"strings"
	"testing"

	"github.com/reoring/go4t/format"
)

const lawProofFixture = `// Equational laws.
package main

// A law is not an Eq call.
law identity(x Int, xs List[Int], pair (Int, Int), box Box,) {
  // The two sides.
  x + 0 == x // right side
}

// All tactics and structural patterns.
proof identity(value, items, pair, box,) {
  if value > 0 {
    // Rewriting keeps a continuation.
    rewrite identity(value, items, pair, box,) {
      match items {
        // Empty list.
        Nil => refl // base case
        Cons(head, tail) => match tail {
          Nil => refl
          Cons(_, rest) => rewrite other.identity(head, rest, pair, box) {
            refl
          }
        }
      }
    }
  } else {
    match pair {
      (left, right) => match box {
        Box{value: Some(inner), ..} => refl
        Box{value: None, ..} => refl
      }
    }
  }
}

law constant() { 1 == 1 }
proof constant() { rewrite constant() { refl } }
`

func TestLawProofRoundTrip(t *testing.T) {
	assertProperties(t, lawProofFixture)
}

func TestLawProofGolden(t *testing.T) {
	src := "package main;law f(x Int){x+1==1+x};proof f(y){if y>0{rewrite f(y){match y{Some(z)=>refl;None=>refl}}}else{refl}}"
	want := `package main

law f(x Int) {
  x + 1 == 1 + x
}

proof f(y) {
  if y > 0 {
    rewrite f(y) {
      match y {
        Some(z) => refl
        None => refl
      }
    }
  } else {
    refl
  }
}
`
	got, err := format.Source([]byte(src))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != want {
		t.Fatalf("got:\n%s\nwant:\n%s", got, want)
	}
	assertProperties(t, src)
}

func TestLawProofExpressionRoundTrip(t *testing.T) {
	for _, src := range []string{
		"law f() { a == b == c }",
		"law f() { a == (b == c) }",
		"law f() { (a || b) == (c && d) }",
		"law f() { (if true { a } else { b }) == c }",
		"law f() { (x |> f) == g(x) }",
		"law f() { f(_, 1) == g(_, 1) }",
		"proof f(x) { if test(x |> f) { refl } else { rewrite f(x |> g) { refl } } }",
		"proof f(x) { match x {} }",
		"proof f(x) { induction x }",
		"proof f(x) { induction x { Nil => refl; Cons(head, tail) => refl } }",
		"proof f() { sample(1) }",
		"proof f(x) { sample(100000) }",
	} {
		t.Run(src, func(t *testing.T) { assertProperties(t, "package main\n"+src) })
	}
}

func TestLawProofComments(t *testing.T) {
	for _, src := range []string{
		`law f() { // law open
  1 == 1
  // law close
}`,
		`proof f() { // proof open
  refl // refl
  // proof close
}`,
		`proof f(x) {
  if true { // then open
    refl // then
    // then close
  } else { // else open
    refl // else
    // else close
  }
}`,
		`proof f(x) {
  rewrite f(x) { // rewrite open
    refl // continuation
    // rewrite close
  }
}`,
		`proof f(x) {
  match x { // match open
    Some(y) => rewrite f(y) {
      refl
    } // arm end
    None => refl
    // match close
  }
}`,
		`proof f(α) {
  rewrite f("https://example.test/{", α) { // real brace, not the string
    refl
  }
}`,
		`proof f(x) {
  if true {
    refl
    // } else { is a comment, not a brace
  } else { // actual else
    refl
  }
}`,
	} {
		t.Run(src, func(t *testing.T) { assertProperties(t, "package main\n"+src) })
	}
}

func TestMultilineTacticPattern(t *testing.T) {
	src := "package main\nproof f(x) { match x { R{" + strings.Repeat("a", 72) + ": Some(y), b: z} => refl } }"
	got, err := format.Source([]byte(src))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(got), "\n    | R{\n") {
		t.Fatalf("multiline tactic pattern needs a leading bar:\n%s", got)
	}
	assertProperties(t, src)
}

func TestSampleInductionComments(t *testing.T) {
	for _, src := range []string{
		"proof f(x) { induction x // every constructor\n}",
		"proof f(x) { induction x { // split\nNil => refl; Cons(head, tail) => refl // IH\n} }",
		"proof f() { sample(30) // tested, not proved\n}",
	} {
		assertProperties(t, "package main\n"+src)
	}
}
