package types_test

import (
	"errors"
	"reflect"
	"testing"

	"gofx/ast"
	"gofx/types"
)

func TestMultiConstraintSchemeOrder(t *testing.T) {
	p, err := types.Check([]*ast.Module{{Files: parseFiles(t, `
func ordered[F[_] Monad, A Show + Ord, B Eq + Show](fa F[A], b B) String { show(b) }
class Render[X] { render[F[_] Monad, A Eq + Show](x X, fa F[A]) F[String] }
instance Render[Int] { render(x, fa) { fmap(fa, func(a) => if eq(a, a) { show(a) } else { "" }) } }
`+pass5Main)}})
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		scheme *types.Scheme
		want   []string
	}{
		{p.Funcs["ordered"].Scheme, []string{"Monad[F]", "Show[A]", "Ord[A]", "Eq[B]", "Show[B]"}},
		{p.Classes["Render"].Methods[0].Scheme, []string{"Monad[F]", "Eq[A]", "Show[A]"}},
	} {
		var got []string
		for _, c := range tc.scheme.Constraints {
			got = append(got, c.Class+"["+types.TypeString(c.Arg)+"]")
		}
		if !reflect.DeepEqual(got, tc.want) {
			t.Fatalf("constraints = %v, want %v", got, tc.want)
		}
	}
}

func TestMultiConstraintDiagnostics(t *testing.T) {
	for _, tc := range []struct{ source, want string }{
		{`func bad[A Show + Eq + Show](x A) A { x }`, "duplicate constraint Show for type parameter A"},
		{`class Bad[X] { bad[A Eq + Eq](x X, a A) A }`, "duplicate constraint Eq for type parameter A"},
		{`instance Show[Option[A]] given Show[A], Eq[A], Show[A] { show(x) { "" } }`, "duplicate constraint Show for type parameter A"},
		{`func bad[A Show + Missing](x A) A { x }`, "unknown class Missing"},
		{`func bad[A Show + Monad](x A) A { x }`, "kind mismatch: got *, want (*) -> *"},
		{`func bad[A Eq + Show](x A) Ordering { compare(x, x) }`, "no instance Ord[A] (type parameter A is constrained only by Eq + Show)"},
	} {
		t.Run(tc.source, func(t *testing.T) {
			files := parseFiles(t, tc.source+pass5Main)
			files[1].Name = "multi.gfx"
			_, err := types.Check([]*ast.Module{{Files: files}})
			var typed *types.Error
			if !errors.As(err, &typed) || typed.Msg != tc.want || typed.Pos.Line == 0 || typed.Pos.Col == 0 {
				t.Fatalf("Check error = %v, want positioned %q", err, tc.want)
			}
		})
	}
}

func TestMultiConstraintSubclassResolution(t *testing.T) {
	for _, constraints := range []string{"Eq + Ord", "Ord + Eq", "Show + Eq + Ord", "Eq + Show + Ord"} {
		t.Run(constraints, func(t *testing.T) {
			p, err := types.Check([]*ast.Module{{Files: parseFiles(t, `
func both[A `+constraints+`](x A) Bool { eq(x, x) }
func forward[A Show + Ord](x A) Bool { both(x) }
func concrete() Bool { both(1) }
`+pass5Main)}})
			if err != nil {
				t.Fatal(err)
			}
			for _, name := range []string{"both", "forward", "concrete"} {
				ann := types.AnnOf(p.Funcs[name].Decl.Body.Result.(*ast.Call).Fn)
				for _, dict := range ann.Dicts {
					if dict.Class == "Show" {
						continue
					}
					for dict.Kind == types.DictSuper {
						dict = *dict.Super
					}
					if name == "concrete" {
						assertInstanceDict(t, dict, "Ord", "Int")
					} else {
						assertParamDict(t, dict, "Ord", "A")
					}
				}
			}
		})
	}
}

func TestMultiConstraintLetGeneralization(t *testing.T) {
	files := parseFiles(t, pass5Show+`
instance Show[Ordering] { show(o) { match o { LT => "LT"; EQ => "EQ"; GT => "GT" } } }
func value() String {
 let f = func(x) => show(x) + show(compare(x, x))
 f(1) + f(true)
}
`+pass5Main)
	for range 2 {
		p, err := types.Check([]*ast.Module{{Files: files}})
		if err != nil {
			t.Fatal(err)
		}
		body := p.Funcs["value"].Decl.Body
		value := body.Stmts[0].(*ast.LetStmt).Value
		scheme := types.AnnOf(value).LetScheme
		if scheme == nil || len(scheme.TParams) != 1 || len(scheme.Constraints) != 2 {
			t.Fatalf("let scheme = %#v", scheme)
		}
		for i, want := range []string{"Show", "Ord"} {
			if c := scheme.Constraints[i]; c.Class != want || c.Arg != scheme.TParams[0] {
				t.Fatalf("let constraint %d = %#v", i, c)
			}
		}
		binary := body.Result.(*ast.Binary)
		for i, call := range []ast.Expr{binary.L, binary.R} {
			dicts := types.AnnOf(call.(*ast.Call).Fn).Dicts
			if len(dicts) != 2 {
				t.Fatalf("let call dictionaries = %#v", dicts)
			}
			for j, class := range []string{"Show", "Ord"} {
				assertInstanceDict(t, dicts[j], class, []string{"Int", "Bool"}[i])
			}
		}
		walkExprs(reflect.ValueOf(value), func(e ast.Expr) { assertZonked(t, e) })
	}
}

func TestMultiConstraintInstanceContext(t *testing.T) {
	p, err := types.Check([]*ast.Module{{Files: parseFiles(t, `
instance Show[Option[A]] given Show[A], Eq[A], Ord[A] {
 show(x) { match x { None => ""; Some(a) => if eq(a, a) { show(a) } else { "" } } }
}
func value() String { show(Some(1)) }
`+pass5Main)}})
	if err != nil {
		t.Fatal(err)
	}
	dict := types.AnnOf(p.Funcs["value"].Decl.Body.Result.(*ast.Call).Fn).Dicts[0]
	if len(dict.Args) != 3 {
		t.Fatalf("context dictionaries = %#v", dict)
	}
	assertInstanceDict(t, dict.Args[0], "Show", "Int")
	if dict.Args[1].Kind != types.DictSuper || dict.Args[1].Class != "Eq" {
		t.Fatalf("superclass dictionary = %#v", dict.Args[1])
	}
	assertInstanceDict(t, *dict.Args[1].Super, "Ord", "Int")
	assertInstanceDict(t, dict.Args[2], "Ord", "Int")
}
