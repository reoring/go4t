package parser_test

import (
	"github.com/reoring/go4t/ast"
	"github.com/reoring/go4t/parser"
	"strings"
	"testing"
)

func TestModuleImportsAndQualifiedNames(t *testing.T) {
	f, err := parser.Parse(`package main
import "geometry"
import g "geometry"
func area(p g.Point) Int { match p { g.Point{x: x, y: y} => geometry.scale(x + y) } }
func origin() g.Point { g.Point{x: 0, y: 0} }
func choose(p g.Shape) Int { match p { g.Empty => 0; g.Circle(r) => r } }
instance g.Measure[g.Point] { measure(p) { g.area(p) } }
`)
	if err != nil {
		t.Fatal(err)
	}
	if len(f.Imports) != 2 || f.Imports[0].Alias != "" || f.Imports[1].Alias != "g" || f.Imports[1].Path != "geometry" {
		t.Fatalf("imports: %#v", f.Imports)
	}
	if f.Imports[0].Pos.Line != 2 || f.Imports[0].End.Col <= f.Imports[0].Pos.Col {
		t.Fatalf("import span: %#v", f.Imports[0])
	}
	area := f.Decls[0].(*ast.FuncDecl)
	if got := area.Params[0].Type.(*ast.TypeName).Name; got != "g.Point" {
		t.Fatal(got)
	}
	match := area.Body.Result.(*ast.Match)
	if got := match.Arms[0].Pat.(*ast.PRecord).Ctor; got != "g.Point" {
		t.Fatal(got)
	}
	selectExpr := match.Arms[0].Body.(*ast.Call).Fn.(*ast.Field)
	if selectExpr.Name != "scale" || selectExpr.X.(*ast.Ident).Name != "geometry" {
		t.Fatalf("qualified expression: %#v", selectExpr)
	}
	choose := f.Decls[2].(*ast.FuncDecl).Body.Result.(*ast.Match)
	if choose.Arms[0].Pat.(*ast.PCtor).Name != "g.Empty" || choose.Arms[1].Pat.(*ast.PCtor).Name != "g.Circle" {
		t.Fatal("qualified constructor pattern lost")
	}
	instance := f.Decls[3].(*ast.InstanceDecl)
	if instance.Class != "g.Measure" || instance.TypeCon != "g.Point" {
		t.Fatalf("qualified instance: %#v", instance)
	}
}
func TestModuleImportErrors(t *testing.T) {
	for _, source := range []string{`package main; import . "geometry"`, `package main; import g`, `package main; func x() Int { 1 }; import "geometry"`} {
		if _, err := parser.Parse(source); err == nil || !strings.Contains(err.Error(), "expected") {
			t.Fatalf("Parse(%s) = %v", source, err)
		}
	}
}
func TestPrivateNullaryConstructorSyntax(t *testing.T) {
	f, err := parser.Parse(`package hidden; type _Secret = _Hidden; func value(x _Secret) Int { match x { _Hidden => 1 } }`)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := f.Decls[1].(*ast.FuncDecl).Body.Result.(*ast.Match).Arms[0].Pat.(*ast.PCtor); !ok {
		t.Fatal("private constructor parsed as binding")
	}
}
