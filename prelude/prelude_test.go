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
	wantDeriving := map[string][]string{
		"Ordering": {"Eq", "Ord", "Show"},
		"Option":   {"Eq", "Ord", "Show"},
		"List":     {"Eq", "Ord"},
	}
	for _, decl := range file.Decls {
		switch decl := decl.(type) {
		case *ast.TypeDecl:
			if want := wantDeriving[decl.Name]; !reflect.DeepEqual(decl.Deriving, want) {
				t.Errorf("%s deriving = %v, want %v", decl.Name, decl.Deriving, want)
			}
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
