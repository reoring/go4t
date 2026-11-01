package types_test

import (
	"errors"
	"testing"

	"go4t/ast"
	"go4t/types"
)

const projectionTypes = `
type Row = Row{qty: Int}
type Box = Box{row: Row}
type Common[A] = First{value: A} | Second{count: Int, value: A}
func map[A, B](xs List[A], f func(A) B) List[B] {
  match xs { Nil => Nil; Cons(x, rest) => Cons(f(x), map(rest, f)) }
}
`

func TestProjectionTypes(t *testing.T) {
	for _, tc := range []struct{ body, want string }{
		{`fmap(Some(Row{qty: 3}), (_.qty))`, "Option[Int]"},
		{`map([Row{qty: 3}], _.qty)`, "List[Int]"},
		{`[Row{qty: 3}] |> map(_.qty)`, "List[Int]"},
		{`fmap(Some((3, "ok")), (_.0))`, "Option[Int]"},
		{`fmap(Some((3, "ok")), _.1)`, "Option[String]"},
		{`fmap(Some(((3, "ok"), true)), _.0.1)`, "Option[String]"},
		{`fmap(Some(Box{row: Row{qty: 4}}), (_.row.qty))`, "Option[Int]"},
		{`fmap(Some(Box{row: Row{qty: 4}}), _.row.qty)`, "Option[Int]"},
		{`map([First{value: 3}, Second{count: 1, value: 4}], _.value)`, "List[Int]"},
		{`let get: func(Row) Int = (_.qty); get(Row{qty: 3})`, "Int"},
		{`let get: func((Int, String)) String = (_.1); get((3, "ok"))`, "String"},
		{`let values: func(List[Row]) List[Int] = map(_, _.qty); values([Row{qty: 3}])`, "List[Int]"},
	} {
		t.Run(tc.body, func(t *testing.T) {
			program, err := checkFixture(t, []*ast.Module{{Files: parseFiles(t, projectionTypes+`
func value() `+tc.want+` { `+tc.body+` }
`+pass5Main)}})
			if err != nil {
				t.Fatal(err)
			}
			value := program.Funcs["value"].Decl.Body.Result
			if got := types.TypeString(types.AnnOf(value).Type); got != tc.want {
				t.Fatalf("type = %s, want %s", got, tc.want)
			}
		})
	}
}

func TestProjectionTypeErrors(t *testing.T) {
	for _, tc := range []struct{ extra, body, want string }{
		{"", `fmap(Some(Row{qty: 1}), _.missing)`, "unknown field missing in constructor Row"},
		{"", `fmap(Some(1), _.qty)`, "field qty requires a record type"},
		{"", `fmap(Some((1, true)), _.2)`, "tuple index 2 out of range for (Int, Bool)"},
		{"", `fmap(Some(1), _.0)`, "tuple index requires a tuple type, got Int"},
		{`type Missing = Has{qty: Int} | Empty`, `fmap(Some(Has{qty: 1}), _.qty)`, "unknown field qty in constructor Empty"},
		{`type Different = Number{qty: Int} | Text{qty: String}`, `fmap(Some(Number{qty: 1}), _.qty)`, "field qty has type String in constructor Text; expected Int from constructor Number"},
	} {
		t.Run(tc.body+tc.extra, func(t *testing.T) {
			files := parseFiles(t, projectionTypes+tc.extra+`
func value() Option[Int] { `+tc.body+` }
`+pass5Main)
			files[1].Name = "section-field.gt"
			_, err := checkFixture(t, []*ast.Module{{Files: files}})
			var typed *types.Error
			if !errors.As(err, &typed) || typed.Msg != tc.want || typed.File != "section-field.gt" {
				t.Fatalf("error = %v, want %q", err, tc.want)
			}
		})
	}
}

func TestProjectionWithoutExpectedRecordRejected(t *testing.T) {
	for _, expr := range []string{"_.qty", "_.row.qty"} {
		_, err := checkFixture(t, []*ast.Module{{Files: parseFiles(t, "func value() Unit { let key = "+expr+"; unit() }"+pass5Main)}})
		var diagnostic *types.Error
		want := "cannot infer record type of _.qty; annotate"
		if expr == "_.row.qty" {
			want = "cannot infer record type of _.row; annotate"
		}
		if !errors.As(err, &diagnostic) || diagnostic.Msg != want {
			t.Fatalf("%s: error = %v, want %q", expr, err, want)
		}
	}
}
