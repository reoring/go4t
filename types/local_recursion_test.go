package types_test

import (
	"strings"
	"testing"

	"github.com/reoring/go4t/ast"
	"github.com/reoring/go4t/types"
)

func TestLocalRecursion(t *testing.T) {
	for _, tc := range []struct{ name, source, wantError string }{
		{"recursive let", `func f() Int { let loop = func(n, acc) => if n == 0 { acc } else { loop(n - 1, acc + n) }; loop(5, 0) }`, ""},
		{"generalized recursion", `func f() (Int, Bool) { let loop = func(n, x) => if n == 0 { x } else { loop(n - 1, x) }; (loop(2, 7), loop(3, true)) }`, ""},
		{"constrained recursion", `func f() String { let loop = func(n, x) => if n == 0 { show(x) } else { loop(n - 1, x) }; loop(2, 7) }`, ""},
		{"monomorphic recursive body", `func f() Int { let loop = func(x) => if true { loop(1) } else { loop(false) }; loop(2) }`, "type mismatch: expected Int, got Bool"},
		{"annotated recursive let", `func f() Int { let loop: func(Int) Int = func(n) => if n == 0 { 0 } else { loop(n - 1) }; loop(3) }`, ""},
		{"nonlambda outer binding", `func f(x Int) Int { let x = x + 1; x }`, ""},
		{"nonlambda undefined binding", `func f() Int { let x = x + 1; x }`, "unknown identifier x"},
		{"pattern let stays nonrecursive", `func f() Int { let (loop, x) = (func(n) => loop(n), 1); x }`, "unknown identifier loop"},
		{"mutual group", `func f() Bool { func isEven(n Int) Bool { if n == 0 { true } else { isOdd(n - 1) } }; func isOdd(n Int) Bool { if n == 0 { false } else { isEven(n - 1) } }; isEven(4) }`, ""},
		{"group outer type parameters", `func f[A Show](x A) String { func first(n Int, a A) String { if n == 0 { show(a) } else { second(n - 1, a) } }; func second(n Int, a A) String { first(n, a) }; first(2, x) }`, ""},
		{"group outer locals", `func f() Int { let x = 7; func get() Int { x }; get() }`, ""},
		{"group later let invisible", `func f() Int { func get() Int { later }; let later = 7; get() }`, "unknown identifier later"},
		{"let separates groups", `func f() Int { func first() Int { second() }; let x = 1; func second() Int { x }; first() }`, "unknown identifier second"},
		{"expression separates groups", `func f() Int { func first() Int { second() }; 1; func second() Int { 2 }; first() }`, "unknown identifier second"},
		{"duplicate group name", `func f() Int { func get() Int { 1 }; func get() Int { 2 }; get() }`, "duplicate local function get"},
		{"duplicate parameter", `func f() Int { func get(x Int, x Int) Int { x }; get(1, 2) }`, "duplicate parameter x"},
		{"local return checked", `func f() Int { func get() Int { true }; get() }`, "type mismatch: expected Int, got Bool"},
		{"local arity checked", `func f() Int { func get(x Int) Int { x }; get() }`, "arity mismatch: want 1 arguments, got 0"},
		{"recursive occurs check", `func f() Unit { let loop = func(x) => loop; unit() }`, "occurs check"},
		{"recursive captures monomorphic parameter", `func f() Int { let outer = func(x) => { let loop = func(done, y) => if done { x(y) } else { loop(true, y) }; loop(false, 1); loop(false, true) }; outer(func(x) => x) }`, "type mismatch: expected Int, got Bool"},
		{"do recursive let", `func f() Option[Int] { do { let loop = func(n) => if n == 0 { 7 } else { loop(n - 1) }; Some(loop(3)) } }`, ""},
		{"do mutual group", `func f() Option[Bool] { do { x <- Some(4); func isEven(n Int) Bool { if n == 0 { true } else { isOdd(n - 1) } }; func isOdd(n Int) Bool { if n == 0 { false } else { isEven(n - 1) } }; Some(isEven(x)) } }`, ""},
		{"do later let invisible", `func f() Option[Int] { do { func get() Int { later }; let later = 7; Some(get()) } }`, "unknown identifier later"},
		{"do bind separates groups", `func f() Option[Int] { do { func first() Int { second() }; x <- Some(1); func second() Int { x }; Some(first()) } }`, "unknown identifier second"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := checkFixture(t, []*ast.Module{{Files: parseFiles(t, tc.source+pass5Main)}})
			if tc.wantError == "" {
				if err != nil {
					t.Fatal(err)
				}
			} else if err == nil || !strings.Contains(err.Error(), tc.wantError) {
				t.Fatalf("error = %v, want %q", err, tc.wantError)
			}
		})
	}
}

func TestRecursiveLetFreeOccurrences(t *testing.T) {
	for _, tc := range []struct {
		value     string
		recursive bool
	}{
		{`func(x) => if true { x } else { loop(x) }`, true},
		{`func(loop) => loop`, false},
		{`func((loop, x)) => loop(x)`, false},
		{`func(x) => { let loop = x; loop }`, false},
		{`func(x) => match x { Some(loop) => loop; None => 0 }`, false},
		{`func(x Int) => { func loop(y Int) Int { y }; loop(x) }`, false},
		{`func(x Int) => { func helper(y Int) Int { loop(y) }; if true { x } else { helper(x) } }`, true},
	} {
		t.Run(tc.value, func(t *testing.T) {
			files := parseFiles(t, "func test() Unit { let loop = "+tc.value+"; unit() }"+pass5Main)
			if _, err := checkFixture(t, []*ast.Module{{Files: files}}); err != nil {
				t.Fatal(err)
			}
			value := files[1].Decls[0].(*ast.FuncDecl).Body.Stmts[0].(*ast.LetStmt).Value
			if got := types.AnnOf(value).Recursive; got != tc.recursive {
				t.Fatalf("Recursive = %t, want %t", got, tc.recursive)
			}
		})
	}
}
