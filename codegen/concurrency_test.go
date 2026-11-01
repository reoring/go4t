package codegen

import (
	goast "go/ast"
	goparser "go/parser"
	"go/token"
	"strings"
	"testing"
)

func TestConcurrencyTupleShapes(t *testing.T) {
	p := fixture()
	list(p)
	option(p)
	source := assertSource(t, p,
		"func rt_tuple2(a, b any) any",
		"return &Tuple2{F0: a, F1: b}",
		"func rt_untuple2(p any) (any, any)",
		"t := p.(*Tuple2)",
		"return t.F0, t.F1",
	)
	if count := strings.Count(source, "type Tuple2 struct"); count != 1 {
		t.Fatalf("Tuple2 declarations = %d, want one", count)
	}
	file, err := goparser.ParseFile(token.NewFileSet(), "generated.go", source, 0)
	if err != nil {
		t.Fatal(err)
	}
	registered := map[string]string{}
	goast.Inspect(file, func(node goast.Node) bool {
		literal, ok := node.(*goast.CompositeLit)
		if !ok {
			return true
		}
		typ, ok := literal.Type.(*goast.SelectorExpr)
		if !ok || typ.Sel.Name != "ShapeFuncs" {
			return true
		}
		for _, element := range literal.Elts {
			field := element.(*goast.KeyValueExpr)
			if value, ok := field.Value.(*goast.Ident); ok {
				registered[field.Key.(*goast.Ident).Name] = value.Name
			}
		}
		return true
	})
	for field, function := range map[string]string{"Tuple2": "rt_tuple2", "Untuple2": "rt_untuple2"} {
		if got := registered[field]; got != function {
			t.Errorf("Shapes.%s = %q, want %q", field, got, function)
		}
	}
}
