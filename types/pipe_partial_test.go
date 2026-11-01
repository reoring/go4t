package types_test

import (
	"errors"
	"fmt"
	"testing"

	"go4t/ast"
	"go4t/parser"
	"go4t/types"
)

func TestPipePartialTypes(t *testing.T) {
	for _, tc := range []struct{ expression, want string }{
		{"add(1, _)", "func(Int) Int"},
		{"pair(_, _)", "func(Int, String) Tuple2[Int, String]"},
		{"(_ + 1)", "func(Int) Int"},
		{"(2 * _)", "func(Int) Int"},
		{"(_ == 2)", "func(Int) Bool"},
		{"(_ < _)", "func(Int, Int) Bool"},
		{"fmap(_, (_ + 1))", "func(Option[Int]) Option[Int]"},
	} {
		t.Run(tc.expression, func(t *testing.T) {
			source := `func add(a Int, b Int) Int { a + b }
func pair(a Int, b String) (Int, String) { (a, b) }
func value() ` + tc.want + ` { ` + tc.expression + ` }
` + pass5Main
			// Tuple2 is an internal type rendering; the surface writes tuples.
			if tc.expression == "pair(_, _)" {
				source = `func pair(a Int, b String) (Int, String) { (a, b) }
func value() func(Int, String) (Int, String) { pair(_, _) }
` + pass5Main
			}
			p, err := checkFixture(t, []*ast.Module{{Files: parseFiles(t, source)}})
			if err != nil {
				t.Fatal(err)
			}
			value := p.Funcs["value"].Decl.Body.Result
			if _, ok := value.(*ast.Lambda); !ok {
				t.Fatalf("checked expression = %T, want ordinary lambda", value)
			}
			if got := types.TypeString(types.AnnOf(value).Type); got != tc.want {
				t.Fatalf("type = %s, want %s", got, tc.want)
			}
		})
	}
}

func TestPipePartialInference(t *testing.T) {
	for _, source := range []string{
		`func value() Int { 1 |> add(2) |> (func(x) => x * 2) }`,
		`func value() Int { 1 |> add(_, 2) |> (2 * _) }`,
		`func value() String { "x" |> pair(1, _) |> (func(p (Int, String)) => p.1) }`,
		`func value() Option[Int] { Some(1) |> fmap(_, (_ + 1)) }`,
		`func value() (Option[Int], Option[Bool]) { let wrap = Some(_); (wrap(1), wrap(true)) }`,
		`func value() (Bool, Bool) { let less = (_ < _); (less(1, 2), less("a", "b")) }`,
		`func value() Int { let plus = add(_, _); plus(1, 2) }`,
		`func value() Int { let apply = func(f, x) => f(x); apply(add(1, _), 2) }`,
		`func value() Bool { false || true |> (func(x) => !x) }`,
		`func value() Int { let _ = 1; (func(_) => 2)(true) }`,
	} {
		t.Run(source, func(t *testing.T) {
			_, err := checkFixture(t, []*ast.Module{{Files: parseFiles(t, `func add(a Int, b Int) Int { a + b }
func pair(a Int, b String) (Int, String) { (a, b) }
`+source+pass5Main)}})
			if err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestPipePartialMisuseRejectedBeforeChecking(t *testing.T) {
	for _, expression := range []string{"_", "(_)", "add(1 + _, 2)"} {
		source := "package main\nfunc value() Int {\n  " + expression + "\n}"
		_, err := parser.Parse(source)
		column := 3
		if expression == "(_)" {
			column++
		} else if expression == "add(1 + _, 2)" {
			column += 8
		}
		want := fmt.Sprintf("3:%d: _ is only valid as a placeholder in a call or operator section", column)
		if err == nil || err.Error() != want {
			t.Fatalf("%s: error = %v, want %q", expression, err, want)
		}
	}
}

func TestPipePartialTypeErrors(t *testing.T) {
	for _, tc := range []struct{ body, want string }{
		{`let f = add(_, _); f(1)`, "arity mismatch: want 2 arguments, got 1"},
		{`1 |> add(_, _)`, "arity mismatch: want 2 arguments, got 1"},
		{`"bad" |> add(1)`, "type mismatch: expected Int, got String"},
		{`let f = add(1, _); f(true)`, "type mismatch: expected Int, got Bool"},
	} {
		files := parseFiles(t, `func add(a Int, b Int) Int { a + b }
func value() Int { `+tc.body+` }
`+pass5Main)
		files[1].Name = "pipe-partial.gt"
		_, err := checkFixture(t, []*ast.Module{{Files: files}})
		var typed *types.Error
		if !errors.As(err, &typed) || typed.Msg != tc.want || typed.File != "pipe-partial.gt" {
			t.Fatalf("%s: error = %v, want %q", tc.body, err, tc.want)
		}
	}
}
