package types_test

import (
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"gofx/ast"
	"gofx/types"
)

func TestJSONDerivingExpansions(t *testing.T) {
	files := derivingFiles(t, `
type Box[A] = Box{value: A, nested: List[Option[A]]} deriving Encode, Decode
type Choice[A] = Empty | Pos(A, Int) | Named{value: A} deriving Encode, Decode
type Tree[A] = Tip | Fork(A, List[Tree[A]]) deriving Encode, Decode
func encoded() Json { encode(Box(1, [Some(2), None])) }
func decoded(j Json) Result[String, Choice[String]] { decode(j) }
func triple(j Json) Result[String, (Int, String, Bool)] { decode(j) }
func five() Json { encode((1, true, "x", 2.5, unit())) }
func mapped(j Json) Result[String, Map[String, Box[Int]]] { decode(j) }
`)
	for range 2 {
		program, err := types.Check(files)
		if err != nil {
			t.Fatal(err)
		}
		seen := make(map[string]bool)
		for _, inst := range program.Instances {
			if inst.Class.Name != "Encode" && inst.Class.Name != "Decode" {
				continue
			}
			key := inst.Class.Name + "/" + inst.TypeCon.Name
			if seen[key] {
				t.Fatalf("duplicate expansion %s", key)
			}
			seen[key] = true
			switch inst.TypeCon.Name {
			case "Box", "Choice", "Tree":
				if len(inst.Context) != 1 || inst.Context[0].Class != inst.Class.Name || inst.Context[0].Arg != inst.Params[0] {
					t.Fatalf("missing derived context: %#v", inst)
				}
				walkExprs(reflect.ValueOf(inst.Decl), func(e ast.Expr) { assertZonked(t, e) })
			case "Map":
				if len(inst.HeadArgs) != 2 || inst.HeadArgs[0] != types.String || len(inst.Params) != 1 || len(inst.Context) != 1 {
					t.Fatalf("Map fixed head = %#v", inst)
				}
			}
		}
		for _, name := range []string{"Encode/Box", "Decode/Box", "Encode/Choice", "Decode/Choice", "Encode/Tree", "Decode/Tree", "Encode/Tuple3", "Decode/Tuple3", "Encode/Tuple5", "Decode/Tuple5"} {
			if !seen[name] {
				t.Errorf("missing %s", name)
			}
		}
	}
}

func TestJSONDerivingErrors(t *testing.T) {
	cases := []struct{ source, want string }{
		{`type T = T deriving Encode, Encode`, "duplicate deriving class Encode for T"},
		{`type T = T deriving Decode; instance Decode[T] { decode(_j) { Ok(T) } }`, "duplicate instance Decode[T]"},
		{`func value() Json { encode(mapFromList([(1, "x")])) }`, "no instance Encode[Map[Int, String]]"},
		{`func value(j Json) Result[String, Map[Int, String]] { decode(j) }`, "no instance Decode[Map[Int, String]]"},
		{`type Hidden = Hidden; type T = T(Hidden) deriving Encode`, "no instance Encode[Hidden]"},
		{`type Hidden = Hidden; type T = T(Hidden) deriving Decode`, "no instance Decode[Hidden]"},
		{`instance Encode[Map[Int, V]] given Encode[V] { encode(_m) { JNull } }`, "duplicate instance Encode[Map]"},
	}
	for _, class := range []string{"Encode", "Decode"} {
		for _, field := range []string{"func(Int) Int", "List[func(Int) Int]", "(String, func(Int) Int)"} {
			cases = append(cases, struct{ source, want string }{fmt.Sprintf("type T = T{value: %s} deriving %s", field, class), fmt.Sprintf("cannot derive %s for T: field of function type", class)})
		}
	}
	for _, tc := range cases {
		t.Run(tc.source, func(t *testing.T) {
			_, err := types.Check(derivingFiles(t, tc.source))
			var diagnostic *types.Error
			if !errors.As(err, &diagnostic) || diagnostic.File != "deriving.gfx" || diagnostic.Pos.Line == 0 || !strings.Contains(diagnostic.Msg, tc.want) {
				t.Fatalf("error = %v; want positioned %q", err, tc.want)
			}
		})
	}
}

func TestJSONConcreteInstanceHead(t *testing.T) {
	files := derivingFiles(t, `
type Pair[A, B] = Pair(A, B)
class Pick[A] { pick(a A) Int }
class PickMore[A] : Pick[A] { pickMore(a A) Int }
instance Pick[Pair[String, V]] { pick(_a) { 1 } }
instance PickMore[Pair[String, V]] { pickMore(a) { pick(a) } }
func value() Int { pickMore(Pair("key", true)) }
`)
	program, err := types.Check(files)
	if err != nil {
		t.Fatal(err)
	}
	dicts := types.AnnOf(program.Funcs["value"].Decl.Body.Result.(*ast.Call).Fn).Dicts
	assertInstanceDict(t, dicts[0], "PickMore", "Pair")
}

func TestJSONHeadVariablesDoNotResolveUserTypeNames(t *testing.T) {
	files := derivingFiles(t, `
type A = A
type V = V
type JsonT0 = JsonT0
type Box[A] = Box(A) deriving Encode, Decode
func value() Json { encode((Box(1), mapFromList([("x", true)]), ["a"])) }
`)
	if _, err := types.Check(files); err != nil {
		t.Fatal(err)
	}
}
