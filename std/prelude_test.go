package std_test

import (
	"io/fs"
	"os"
	"strings"
	"testing"

	"gofx/ast"
	"gofx/parser"
	"gofx/std"
)

func TestStdInventoryParses(t *testing.T) {
	var files []string
	if err := fs.WalkDir(std.FS, ".", func(name string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !entry.IsDir() && strings.HasSuffix(name, ".gfx") {
			files = append(files, name)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	names := make(map[string]bool)
	for _, name := range files {
		source, err := std.FS.ReadFile(name)
		if err != nil {
			t.Fatal(err)
		}
		file, err := parser.Parse(string(source))
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		for _, decl := range file.Decls {
			switch decl := decl.(type) {
			case *ast.ClassDecl:
				names["class "+decl.Name] = true
			case *ast.TypeDecl:
				names["type "+decl.Name] = true
			case *ast.InstanceDecl:
				names["instance "+decl.Class+"["+decl.TypeCon+"]"] = true
			case *ast.FuncDecl:
				names["func "+decl.Name] = true
			default:
				t.Fatalf("%s: unexpected declaration %T", name, decl)
			}
		}
	}
	if len(files) == 0 {
		t.Fatal("std embedded filesystem contains no .gfx files")
	}
	want := []string{
		"class Functor", "class Monad", "class Show", "class Eq", "type Ordering", "class Ord",
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
		"func strLength", "func strConcat", "func join", "func split", "func lines", "func words",
		"func trim", "func toUpper", "func toLower", "func startsWith", "func endsWith", "func contains",
		"func replace", "func substring", "func chars", "func repeat", "func parseInt", "func showPadLeft",
		"func readLine", "func readAll", "func args", "func exit", "func getEnv",
		"func fork", "func await", "func parMapIO", "func race", "func parBoth", "func timeout",
		"func sleep", "func monotonicMillis",
		"instance Show[Int]", "instance Show[String]", "instance Show[Bool]",
		"instance Eq[Int]", "instance Ord[Int]", "instance Eq[String]", "instance Ord[String]",
		"instance Eq[Bool]", "instance Ord[Bool]", "instance Eq[Unit]",
		"instance Eq[Tuple2]", "instance Ord[Tuple2]",
		"instance Functor[IO]", "instance Monad[IO]",
		"func map", "func flatMap", "func forEach", "func mapM",
		"func fold", "func length", "func append", "func filter", "func reverse",
		"func fst", "func snd", "func zip",
		"func head", "func tail", "func take", "func drop", "func sum", "func concatMap",
		"func elem", "func sortBy", "func sort", "func maximum", "func minimum",
	}
	for _, name := range want {
		if !names[name] {
			t.Errorf("missing std declaration %s", name)
		}
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
