package std_test

import (
	"context"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"go4t/ast"
	"go4t/codegen"
	"go4t/modules"
	"go4t/parser"
	"go4t/std"
	"go4t/types"
)

func TestStdInventoryParses(t *testing.T) {
	var files []string
	if err := fs.WalkDir(std.FS, ".", func(name string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !entry.IsDir() && strings.HasSuffix(name, ".gt") {
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
		t.Fatal("std embedded filesystem contains no .gt files")
	}
	want := []string{
		"class Functor", "class Monad", "class Show", "class Eq", "type Ordering", "class Ord",
		"type Option", "type List", "type State",
		"type IoError", "instance Show[IoError]", "func ioRaise", "func ioErrorMessage", "func tryIO",
		"instance Functor[Option]", "instance Monad[Option]",
		"instance Functor[List]", "instance Monad[List]",
		"instance Functor[State]", "instance Monad[State]",
		"func runState", "func evalState", "func execState", "func get", "func put", "func modify",
		"func fromOption", "func maybe", "func isSome", "func isNone",
		"instance Show[List]", "instance Show[Tuple2]",
		"func ioPure", "func ioBind", "func ioFail", "func println",
		"func showInt", "func showBool", "func showString", "func unit",
		"func when", "func unless",
		"func eqInt", "func ltInt", "func eqString", "func ltString", "func eqBool",
		"func minInt", "func maxInt", "func abs",
		"func strLength", "func strConcat", "func join", "func split", "func lines", "func words",
		"func trim", "func toUpper", "func toLower", "func startsWith", "func endsWith", "func contains",
		"func replace", "func substring", "func chars", "func repeat", "func parseInt", "func showPadLeft",
		"func readLine", "func readAll", "func args", "func exit", "func getEnv",
		"func fork", "func await", "func cancel", "func parMapIO", "func race", "func parBoth", "func timeout", "func withTimeout",
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

func TestTextExamplesParse(t *testing.T) {
	for _, name := range []string{"wc", "strings"} {
		t.Run(name, func(t *testing.T) {
			source, err := os.ReadFile("../examples/" + name + "/main.gt")
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

func TestPreludeWhenUnless(t *testing.T) {
	const source = `package main
import "std/prelude"

func optionWhen(cond Bool) Option[Unit] {
  do {
    prelude.when(cond, None)
    Some(unit())
  }
}

func optionUnless(cond Bool) Option[Unit] {
  do {
    prelude.unless(cond, None)
    Some(unit())
  }
}

func main() IO[Unit] {
  do {
    ran <- prelude.when(true, println("when action"))
    skipped <- prelude.when(false, ioFail("when false ran"))
    unlessRan <- prelude.unless(false, println("unless action"))
    unlessSkipped <- prelude.unless(true, ioFail("unless true ran"))
    println(debug((ran, skipped)))
    println(debug((unlessRan, unlessSkipped)))
    println(debug(optionWhen(true)))
    println(debug(optionWhen(false)))
    println(debug(optionUnless(false)))
    println(debug(optionUnless(true)))
    println(debug(prelude.when(true, Some(unit()))))
    println(debug(prelude.unless(false, Some(unit()))))
  }
}
`
	user, err := parser.Parse(source)
	if err != nil {
		t.Fatal(err)
	}
	user.Name = "prelude_when.gt"
	mods, err := modules.LoadFiles([]*ast.File{user}, ".")
	if err != nil {
		t.Fatal(err)
	}
	p, err := types.Check(mods)
	if err != nil {
		t.Fatal(err)
	}
	generated, err := codegen.Generate(p, "main")
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	root, err := filepath.Abs("..")
	if err != nil {
		t.Fatal(err)
	}
	generated["go.mod"] = []byte("module preludetest\n\ngo 1.22\n\nrequire go4t v0.0.0\nreplace go4t => " + root + "\n")
	for name, content := range generated {
		if err := os.WriteFile(filepath.Join(dir, name), content, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	command := exec.CommandContext(ctx, "go", "run", ".")
	command.Dir = dir
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("go run generated prelude program: %v\n%s", err, output)
	}
	want := "when action\nunless action\n((), ())\n((), ())\nNone\nSome(())\nNone\nSome(())\nSome(())\nSome(())\n"
	if string(output) != want {
		t.Fatalf("output = %q, want %q", output, want)
	}
}
