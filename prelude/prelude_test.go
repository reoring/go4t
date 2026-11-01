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
		"class Functor", "class Monad", "class Show", "class Eq", "type Ordering", "class Ord",
		"type Option", "type List", "type State",
		"instance Functor[Option]", "instance Monad[Option]",
		"instance Functor[List]", "instance Monad[List]",
		"instance Functor[State]", "instance Monad[State]",
		"func runState", "func evalState", "func execState", "func get", "func put", "func modify",
		"instance Show[Option]", "instance Show[List]", "instance Show[Tuple2]",
		"func ioPure", "func ioBind", "func ioFail", "func println",
		"func showInt", "func showBool", "func showString", "func unit",
		"func eqInt", "func ltInt", "func eqString", "func ltString", "func eqBool",
		"func minInt", "func maxInt",
		"instance Show[Int]", "instance Show[String]", "instance Show[Bool]",
		"instance Eq[Int]", "instance Ord[Int]", "instance Eq[String]", "instance Ord[String]",
		"instance Eq[Bool]", "instance Ord[Bool]", "instance Eq[Unit]",
		"instance Eq[Ordering]", "instance Ord[Ordering]",
		"instance Eq[Option]", "instance Ord[Option]", "instance Eq[List]", "instance Ord[List]",
		"instance Eq[Tuple2]", "instance Ord[Tuple2]",
		"instance Functor[IO]", "instance Monad[IO]",
		"func map", "func flatMap", "func forEach", "func mapM",
		"func fold", "func length", "func append", "func filter", "func reverse",
		"func fst", "func snd", "func zip",
		"func head", "func tail", "func take", "func drop", "func sum", "func concatMap",
		"func elem", "func insertBy", "func sortBy", "func sort", "func maximum", "func minimum",
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
