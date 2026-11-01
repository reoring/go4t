package types_test

import (
	"errors"
	"strings"
	"testing"

	"gofx/ast"
	"gofx/types"
)

func TestListPatternCoverage(t *testing.T) {
	for _, tt := range []struct{ source, missing string }{
		{`func value(xs List[Int]) Int { match xs { [] => 0; [_, ..] => 1 } }`, ""},
		{`func value(xs List[Int]) Int { match xs { [] => 0; [_] => 1; [_, _, ..rest] => value(rest) } }`, ""},
		{`func value(xs List[Int]) Int { match xs { [] => 0; [a, ..rest] => a + value(rest) } }`, ""},
		{`func value(xs List[Int]) Int { match xs { [_, ..] => 0 } }`, "[]"},
		{`func value(xs List[Int]) Int { match xs { [] => 0; [_, _, ..] => 2 } }`, "[_]"},
		{`func value(xs List[Int]) Int { match xs { [] => 0; [_] => 1 } }`, "[_, _, ..]"},
		{`func value(xs List[Int]) Int { match xs { [] => 0 } }`, "[_, ..]"},
		{`func value(xs List[Int]) Int { match xs { [_, _] => 2 } }`, "[], [_], [_, _, _, ..]"},
		{`func value(xs List[Int]) Int { match xs { _ if false => 0 } }`, "[], [_, ..]"},
		{`func value(xs List[Int]) Int { match xs { [] => 0; [_, ..] if true => 1 } }`, "[_, ..]"},
		{`func value(xs List[Int]) Int { match xs { [] => 0; [0, ..] => 1 } }`, "[1, ..]"},
		{`func value(xs List[Bool]) Int { match xs { [] => 0; [false, ..] => 1 } }`, "[true, ..]"},
		{`func value(xs List[List[Int]]) Int { match xs { [] => 0; [[], ..] => 1 } }`, "[[_, ..], ..]"},
		{`func value(xs Option[List[Int]]) Int { match xs { None => 0; Some([]) => 1 } }`, "Some([_, ..])"},
		{`func value(xs (List[Int], Bool)) Int { match xs { (_, false) => 0; ([], true) => 1 } }`, "([_, ..], true)"},
		{`alias Ints = List[Int]; func value(xs Ints) Int { match xs { [] => 0 } }`, "[_, ..]"},
		{`func value(xs List[Int]) Int { match xs { Cons(_, _) => 0 } }`, "[]"},
		{`func value(xs List[Int]) Int { match xs { Nil => 0; Cons(_, Nil) => 1 } }`, "[_, _, ..]"},
		{`type Chain = End | Link(Int, Chain); func value(xs Chain) Int { match xs { End => 0 } }`, "Link(_, _)"},
	} {
		t.Run(tt.source, func(t *testing.T) {
			_, err := checkFixture(t, []*ast.Module{{Files: parseFiles(t, tt.source+pass5Main)}})
			if tt.missing == "" {
				if err != nil {
					t.Fatal(err)
				}
				return
			}
			var typed *types.Error
			want := "non-exhaustive match; missing: " + tt.missing
			if !errors.As(err, &typed) || typed.Msg != want {
				t.Fatalf("error = %v, want %q", err, want)
			}
		})
	}
}

func TestListPatternBindingErrors(t *testing.T) {
	for _, tt := range []struct{ source, want string }{
		{`func value() Int { let [a, b] = [1, 2]; a + b }`, "let pattern must be irrefutable"},
		{`func value() Int { let [] = []; 0 }`, "let pattern must be irrefutable"},
		{`func value() Int { let [a, ..rest] = [1, 2]; a }`, "let pattern must be irrefutable"},
		{`func value() Int { (func([a, b]) => a + b)([1, 2]) }`, "lambda parameter pattern must be irrefutable"},
		{`func value() Int { (func([]) => 0)([]) }`, "lambda parameter pattern must be irrefutable"},
		{`func value() Int { (func([a, .._]) => a)([1, 2]) }`, "lambda parameter pattern must be irrefutable"},
		{`func value() IO[Int] { do { let [a, b] = [1, 2]; ioPure(a + b) } }`, "let pattern must be irrefutable"},
		{`func value() IO[Int] { do { [a, ..rest] <- ioPure([1, 2]); ioPure(a) } }`, "bind pattern must be irrefutable"},
		{`func value(xs List[Int]) Int { match xs { [a, a] => a; _ => 0 } }`, "duplicate pattern variable a"},
		{`func value(xs List[Int]) Int { match xs { [a, ..a] => a; _ => 0 } }`, "duplicate pattern variable a"},
		{`func value(xs List[Int]) Int { match xs { [a, ..rest] => rest; _ => 0 } }`, "type mismatch: expected Int, got List[Int]"},
		{`func value(xs Int) Int { match xs { [] => 1; _ => 0 } }`, "type mismatch: expected Int, got List["},
	} {
		t.Run(tt.source, func(t *testing.T) {
			_, err := checkFixture(t, []*ast.Module{{Files: parseFiles(t, tt.source+pass5Main)}})
			var typed *types.Error
			if !errors.As(err, &typed) || !strings.HasPrefix(typed.Msg, tt.want) {
				t.Fatalf("error = %v, want %q", err, tt.want)
			}
		})
	}
}
