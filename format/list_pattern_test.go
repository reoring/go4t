package format_test

import (
	"testing"

	"github.com/reoring/go4t/format"
)

func TestListPatternGolden(t *testing.T) {
	for _, tt := range []struct{ source, want string }{
		{
			"package main;func f(xs List[Int]) Int{match xs{[]=>0|[a,b,c,]=>a+b+c|[a,b,..rest,]=>a|[a,.._]=>a}}",
			"package main\n\nfunc f(xs List[Int]) Int {\n  match xs {\n    [] => 0\n    [a, b, c] => a + b + c\n    [a, b, ..rest] => a\n    [a, ..] => a\n  }\n}\n",
		},
		{
			"package main;func f(xs List[Int]) Int{match xs{Cons(a,Cons(b,Nil))=>a+b|Nil=>0|[a,..rest]=>a}}",
			"package main\n\nfunc f(xs List[Int]) Int {\n  match xs {\n    Cons(a, Cons(b, Nil)) => a + b\n    Nil => 0\n    [a, ..rest] => a\n  }\n}\n",
		},
		{
			"package main;func f() Int{let [a,b]=xs;let g=func([x,y])=>x+y;g([a,b])}",
			"package main\n\nfunc f() Int {\n  let [a, b] = xs\n  let g = func([x, y]) => x + y\n  g([a, b])\n}\n",
		},
		{
			"package main;func f(xs T) Int{match xs{whole@[[],[a,..rest],Cons(b,Nil)]=>a|_=>0}}",
			"package main\n\nfunc f(xs T) Int {\n  match xs {\n    whole @ [[], [a, ..rest], Cons(b, Nil)] => a\n    _ => 0\n  }\n}\n",
		},
	} {
		t.Run(tt.source, func(t *testing.T) {
			got, err := format.Source([]byte(tt.source))
			if err != nil || string(got) != tt.want {
				t.Fatalf("format = %s, error = %v, want:\n%s", got, err, tt.want)
			}
			assertProperties(t, tt.source)
		})
	}
}

func TestListPatternRoundTrip(t *testing.T) {
	for _, pattern := range []string{
		"[]", "[a]", "[a, b, c,]", "[a, ..rest]", "[a, .._]", "[a, ..]",
		"[a, // first\n b, // second\n ..rest]", "[ // opening\n a, ..rest]",
		"[a, .. // ignored\n]", "[[], [a, ..rest]]",
		"[Some(a), (b, _), whole @ Cons(c, tail)]",
	} {
		t.Run(pattern, func(t *testing.T) {
			assertProperties(t, "package main\nfunc f(xs T) Int { match xs { "+pattern+" => 0; _ => 1 } }")
		})
	}
}
