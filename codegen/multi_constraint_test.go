package codegen

import (
	"fmt"
	"testing"

	"github.com/reoring/go4t/ast"
	"github.com/reoring/go4t/parser"
	"github.com/reoring/go4t/types"
)

const multiConstraintPrelude = `package main
class Show[A] { show(a A) String }
class Eq[A] { eq(a A, b A) Bool }
type Ordering = LT | EQ | GT
class Ord[A] : Eq[A] { compare(a A, b A) Ordering }
instance Show[Int] { show(a) { "int" } }
instance Show[Bool] { show(a) { "bool" } }
instance Show[Ordering] { show(a) { "ordering" } }
instance Eq[Int] { eq(a, b) { true } }
instance Eq[Bool] { eq(a, b) { true } }
instance Ord[Int] { compare(a, b) { EQ } }
instance Ord[Bool] { compare(a, b) { EQ } }
extern func main() IO[Unit] = "github.com/reoring/go4t/rt".UnitValue
`

func multiConstraintProgram(t *testing.T, source string) *types.Program {
	t.Helper()
	file, err := parser.Parse(multiConstraintPrelude + source)
	if err != nil {
		t.Fatal(err)
	}
	p, err := types.Check([]*ast.Module{{Files: []*ast.File{file}}})
	if err != nil {
		t.Fatal(err)
	}
	return p
}

func TestMultiConstraintDictionaryParameters(t *testing.T) {
	p := multiConstraintProgram(t, `
func describe[A Show + Ord](x A) String { show(x) + show(compare(x, x)) }
func forward[A Show + Ord](x A) String { describe(x) }
func value() String { describe(1) }
func equal[A Eq + Ord](x A) Bool { eq(x, x) }
func inherited[A Eq + Ord](x A) Bool { equal(x) }
func concrete() Bool { equal(1) }
func single[A Show](x A) String { show(x) }
func captured[A Show + Ord](x A) String { let f = describe; f(x) }
`)
	assertSource(t, p,
		"func describe_(dict_A_Show Show, dict_A_Ord Ord, x_ any) any",
		"rt.AddString((dict_A_Show).Show(x_), (Show_Ordering{}).Show((dict_A_Ord).Compare(x_, x_)))",
		"return describe_(dict_A_Show, dict_A_Ord, x_)",
		"return equal_(dict_A_Ord, dict_A_Ord, x_)",
		"func single_(dict_A Show, x_ any) any",
		"return (dict_A).Show(x_)",
		"return describe_(dict_A_Show, dict_A_Ord, a0)")
}

func TestMultiConstraintMethodAndContextDictionaries(t *testing.T) {
	p := multiConstraintProgram(t, `
type Box[A] = Box(A)
class Render[X] { render[B Eq + Show](x X, a B) String }
instance Render[Box[A]] given Show[A], Ord[A] {
 render(x, a) {
  match x { Box(value) => if eq(a, a) { show(value) + show(a) + show(compare(value, value)) } else { "" } }
 }
}
func value() String { render(Box(1), true) }
instance Eq[Box[A]] given Eq[A], Show[A] { eq(x, y) { match (x, y) { (Box(a), Box(b)) => eq(a, b) } } }
instance Ord[Box[A]] given Show[A], Ord[A] { compare(x, y) { match (x, y) { (Box(a), Box(b)) => compare(a, b) } } }
func ordered() Ordering { compare(Box(1), Box(2)) }
`)
	assertSource(t, p,
		"Render(dict_B_Eq Eq, dict_B_Show Show, a0 any, a1 any) any",
		"type Render_Box struct {\n\tdict_A_Show Show\n\tdict_A_Ord  Ord\n}",
		"func (dictself Render_Box) Render(dict_B_Eq Eq, dict_B_Show Show, x_ any, a_ any) any",
		"(dict_B_Eq).Eq(a_, a_)",
		"(dictself.dict_A_Show).Show(value_)",
		"(dict_B_Show).Show(a_)",
		"(dictself.dict_A_Ord).Compare(value_, value_)",
		"(Render_Box{dict_A_Show: Show_Int{}, dict_A_Ord: Ord_Int{}}).Render(Eq_Bool{}, Show_Bool{}, &Box_Box{F0: int64(1)}, true)",
		"Ord_Box{Eq_Box: Eq_Box{dict_A_Eq: Ord_Int{}, dict_A_Show: Show_Int{}}, dict_A_Show: Show_Int{}, dict_A_Ord: Ord_Int{}}")
}

func TestMultiConstraintLetDictionaries(t *testing.T) {
	p := multiConstraintProgram(t, `
func value() String {
 let f = func(x) => show(x) + show(compare(x, x))
 f(1) + f(true)
}
func nested[A Show + Ord](a A) String {
 let outer = func(x) => {
  let inner = func(y) => show(y) + show(compare(y, y)) + show(x) + show(a)
  inner(true)
 }
 outer(1)
}
`)
	scheme := types.AnnOf(p.Funcs["value"].Decl.Body.Stmts[0].(*ast.LetStmt).Value).LetScheme
	param := scheme.TParams[0].Name
	assertSource(t, p,
		fmt.Sprintf("f_ := func(dict_%s_Show Show, dict_%s_Ord Ord) any", param, param),
		fmt.Sprintf("(dict_%s_Show).Show(x_)", param),
		fmt.Sprintf("(dict_%s_Ord).Compare(x_, x_)", param),
		"f_(Show_Int{}, Ord_Int{})",
		"f_(Show_Bool{}, Ord_Bool{})",
		"(dict_A_Show).Show(a_)")
}
