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
		"class Functor", "class Monad", "class Show", "class Debug", "class Eq", "type Ordering", "class Ord",
		"type Option", "type List", "type State",
		"instance Functor[Option]", "instance Monad[Option]",
		"instance Functor[List]", "instance Monad[List]",
		"instance Functor[State]", "instance Monad[State]",
		"func runState", "func evalState", "func execState", "func get", "func put", "func modify",
		"func fromOption", "func maybe", "func isSome", "func isNone",
		"instance Show[List]", "instance Show[Tuple2]",
		"func ioPure", "func ioBind", "func ioFail", "func println",
		"func showInt", "func showBool", "func showString", "func unit",
		"func eqInt", "func ltInt", "func eqString", "func ltString", "func eqBool",
		"func minInt", "func maxInt", "func abs",
		"func showFloat", "func eqFloat", "func ltFloat", "func toFloat",
		"func truncate", "func floor", "func ceil", "func round", "func sqrt", "func pow", "func powInt",
		"func parseFloat", "func showFloatFixed", "func absFloat", "func nan", "func isNaN",
		"func min", "func max", "instance Show[Float]", "instance Debug[Float]",
		"instance Eq[Float]", "instance Ord[Float]",
		"func strLength", "func strConcat", "func join", "func split", "func lines", "func words",
		"func trim", "func toUpper", "func toLower", "func startsWith", "func endsWith", "func contains",
		"func replace", "func substring", "func chars", "func repeat", "func parseInt", "func showPadLeft",
		"func readLine", "func readAll", "func args", "func exit", "func getEnv",
		"instance Show[Int]", "instance Show[String]", "instance Show[Bool]",
		"instance Eq[Int]", "instance Ord[Int]", "instance Eq[String]", "instance Ord[String]",
		"instance Eq[Bool]", "instance Ord[Bool]", "instance Eq[Unit]",
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
	wantDeriving := map[string][]string{
		"Ordering": {"Eq", "Ord", "Show"},
		"Option":   {"Eq", "Ord", "Show"},
		"List":     {"Eq", "Ord"},
	}
	var names []string
	for _, decl := range file.Decls {
		switch decl := decl.(type) {
		case *ast.ClassDecl:
			names = append(names, "class "+decl.Name)
		case *ast.TypeDecl:
			names = append(names, "type "+decl.Name)
			if want := wantDeriving[decl.Name]; !reflect.DeepEqual(decl.Deriving, want) {
				t.Errorf("%s deriving = %v, want %v", decl.Name, decl.Deriving, want)
			}
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

func TestTextExamplesParse(t *testing.T) {
	for _, name := range []string{"wc", "strings"} {
		t.Run(name, func(t *testing.T) {
			source, err := os.ReadFile("../examples/" + name + "/main.gfx")
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
			if len(file.Decls) == 0 {
				t.Fatal("example has no declarations")
			}
			main, ok := file.Decls[len(file.Decls)-1].(*ast.FuncDecl)
			if !ok || main.Name != "main" {
				t.Fatalf("last declaration = %#v, want main function", file.Decls[len(file.Decls)-1])
			}
		})
	}
}
