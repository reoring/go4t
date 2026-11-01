package types_test

import (
	"errors"
	"testing"

	"go4t/ast"
	"go4t/types"
)

const expectedSale = `
type Sale = Sale{product: String, qty: Int, jpy: Int}
type Key[A] = Key{key: A}
func identity[A](x A) A { x }
func map[A, B](xs List[A], f func(A) B) List[B] {
  match xs { Nil => Nil; Cons(x, rest) => Cons(f(x), map(rest, f)) }
}
`

func TestExpectedTypes(t *testing.T) {
	for _, tc := range []struct{ name, result, body string }{
		{"constructor return", "Option[func(Sale) String]", `Some(_.product)`},
		{"call return", "func(Sale) String", `identity(_.product)`},
		{"return projection", "func(Sale) String", `_.product`},
		{"annotated let", "String", `let f: func(Sale) String = _.product; f(Sale{product: "tea", qty: 2, jpy: 300})`},
		{"annotated lambda let", "String", `let key: func(Sale) String = func(s) => s.product; key(Sale{product: "tea", qty: 2, jpy: 300})`},
		{"match return", "func(Sale) Int", `match true { true => _.qty; false => _.jpy }`},
		{"match first arm", "Int", `let key = match true { true => func(s Sale) => s.qty; false => _.jpy }; key(Sale{product: "tea", qty: 2, jpy: 300})`},
		{"if return", "func(Sale) Int", `if true { _.qty } else { _.jpy }`},
		{"nested if match", "Option[func(Sale) String]", `if true { match true { true => Some(_.product); false => None } } else { Some(func(s) => s.product) }`},
		{"record return", "Key[func(Sale) String]", `Key{key: _.product}`},
		{"record field", "Key[func(Sale) String]", `let k: Key[func(Sale) String] = Key{key: func(s) => s.product}; k`},
		{"record update", "Key[func(Sale) String]", `let k: Key[func(Sale) String] = Key{key: _.product}; k{key: func(s) => s.product}`},
		{"lambda result", "func(Int) func(Sale) String", `func(_) => _.product`},
		{"annotated lambda result", "func(Int) func(Sale) String", `func(n Int) => if n == 0 { _.product } else { func(s) => s.product }`},
		{"mixed lambda parameters", "func(Int, Sale) String", `func(n Int, s) => if n == 0 { s.product } else { "" }`},
		{"do result", "Option[func(Sale) String]", `do { n <- Some(1); Some(func(s) => if n == 1 { s.product } else { "" }) }`},
		{"local function return", "Option[func(Sale) String]", `func key() Option[func(Sale) String] { Some(_.product) }; key()`},
		{"map unchanged", "List[Int]", `map([Sale{product: "tea", qty: 2, jpy: 300}], _.qty)`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p, err := checkFixture(t, []*ast.Module{{Files: parseFiles(t, expectedSale+"\nfunc value() "+tc.result+" { "+tc.body+" }"+pass5Main)}})
			if err != nil {
				t.Fatal(err)
			}
			if got := types.TypeString(types.AnnOf(p.Funcs["value"].Decl.Body.Result).Type); got != tc.result {
				t.Fatalf("type = %s, want %s", got, tc.result)
			}
		})
	}
}

func TestExpectedTypeBoundaries(t *testing.T) {
	for _, tc := range []struct{ name, source, want string }{
		{"no expectation", `func value() Unit { let key = Some(_.product); unit() }`, "cannot infer record type of _.product; annotate"},
		{"no lookahead", `func value() Unit { let key = Some(_.product); let expected: Option[func(Sale) String] = key; unit() }`, "cannot infer record type of _.product; annotate"},
		{"no lambda context", `func value() Unit { let key = func(s) => s.product; unit() }`, "cannot infer record type of s.product; annotate"},
		{"no list propagation", `func value() List[func(Sale) String] { [(_.product)] }`, "cannot infer record type of _.product; annotate"},
		{"no tuple propagation", `func value() (func(Sale) String, Int) { ((_.product), 1) }`, "cannot infer record type of _.product; annotate"},
		{"no if sibling expectation", `func value() Unit { let key = if true { func(s Sale) => s.product } else { (_.product) }; unit() }`, "cannot infer record type of _.product; annotate"},
		{"integer stays integer", `func value() Float { 1 }`, "type mismatch: expected Float, got Int"},
		{"constructor integer stays integer", `func value() Option[Float] { Some(1) }`, "type mismatch: expected Float, got Int"},
		{"bad field", `func value() Option[func(Sale) String] { Some(_.missing) }`, "unknown field missing in constructor Sale"},
		{"wrong projection parameter", `func value() Option[func(Int) String] { Some(_.product) }`, "field product requires a record type"},
		{"wrong lambda annotation", `func value() func(Sale) String { func(s Int) => "" }`, "type mismatch: expected func(Sale) String, got func(Int) String"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p, err := checkFixture(t, []*ast.Module{{Files: parseFiles(t, expectedSale+tc.source+pass5Main)}})
			var diagnostic *types.Error
			if p != nil || !errors.As(err, &diagnostic) || diagnostic.Msg != tc.want {
				t.Fatalf("Check = (%v, %v), want %q", p, err, tc.want)
			}
		})
	}
}
