package prelude

import (
	"os"
	"reflect"
	"testing"

	"gofx/ast"
	"gofx/parser"
)

func TestPreludeParses(t *testing.T) {
	file, err := parser.Parse(Source)
	if err != nil {
		t.Fatal(err)
	}
	if file.Package != "prelude" {
		t.Errorf("package = %q, want prelude", file.Package)
	}
	want := []string{
		"class Functor", "class Monad", "class Show", "type Option", "type List",
		"instance Functor[Option]", "instance Monad[Option]",
		"instance Functor[List]", "instance Monad[List]",
		"instance Show[Option]", "instance Show[List]", "instance Show[Tuple2]",
		"func ioPure", "func ioBind", "func ioFail", "func println",
		"func showInt", "func showBool", "func showString", "func unit",
		"instance Show[Int]", "instance Show[String]", "instance Show[Bool]",
		"instance Functor[IO]", "instance Monad[IO]",
		"func map", "func flatMap",
		"func fold", "func length", "func append", "func filter", "func reverse",
		"func fst", "func snd", "func zip",
	}
	if len(file.Decls) != len(want) {
		t.Fatalf("declaration count = %d, want %d", len(file.Decls), len(want))
	}
	var names []string
	for _, decl := range file.Decls {
		switch decl := decl.(type) {
		case *ast.ClassDecl:
			names = append(names, "class "+decl.Name)
		case *ast.TypeDecl:
			names = append(names, "type "+decl.Name)
		case *ast.InstanceDecl:
			names = append(names, "instance "+decl.Class+"["+decl.TypeCon+"]")
		case *ast.FuncDecl:
			names = append(names, "func "+decl.Name)
		default:
			t.Fatalf("unexpected declaration %T", decl)
		}
	}
	if !reflect.DeepEqual(names, want) {
		t.Errorf("declarations = %v, want %v", names, want)
	}
}

func TestHTTPExampleParses(t *testing.T) {
	source, err := os.ReadFile("../examples/http/main.gfx")
	if err != nil {
		t.Fatal(err)
	}
	file, err := parser.Parse(string(source))
	if err != nil {
		t.Fatal(err)
	}
	if file.Package != "main" {
		t.Errorf("package = %q, want main", file.Package)
	}
	want := []string{"httpGet", "twice", "optionIncrement", "optionMessage", "main"}
	if len(file.Decls) != len(want) {
		t.Fatalf("declaration count = %d, want %d", len(file.Decls), len(want))
	}
	for i, decl := range file.Decls {
		function, ok := decl.(*ast.FuncDecl)
		if !ok {
			t.Fatalf("declaration %d = %T, want function", i, decl)
		}
		if function.Name != want[i] {
			t.Errorf("declaration %d = %s, want %s", i, function.Name, want[i])
		}
	}
}
