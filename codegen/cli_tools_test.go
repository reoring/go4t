package codegen

import (
	goast "go/ast"
	"go/parser"
	"go/token"
	"testing"
)

func TestPackageEntryPoint(t *testing.T) {
	for _, pkg := range []string{"", "main", "embedded"} {
		source, err := generate(fixture(), pkg)
		if err != nil {
			t.Fatal(err)
		}
		file, err := parser.ParseFile(token.NewFileSet(), "main.go", source, 0)
		if err != nil {
			t.Fatal(err)
		}
		wantPackage, wantEntry := pkg, "Main"
		if pkg == "" || pkg == "main" {
			wantPackage, wantEntry = "main", "main"
		}
		if file.Name.Name != wantPackage {
			t.Errorf("package = %q, want %q", file.Name.Name, wantPackage)
		}
		entries := 0
		for _, decl := range file.Decls {
			f, ok := decl.(*goast.FuncDecl)
			if !ok || (f.Name.Name != "Main" && f.Name.Name != "main") {
				continue
			}
			entries++
			if f.Name.Name != wantEntry || f.Type.Params.NumFields() != 0 {
				t.Errorf("entry = %s, want zero-argument %s", f.Name.Name, wantEntry)
			}
			if wantEntry == "Main" {
				if f.Type.Results.NumFields() != 1 {
					t.Fatal("Main must return rt.IO")
				}
				result, ok := f.Type.Results.List[0].Type.(*goast.SelectorExpr)
				if !ok || result.Sel.Name != "IO" || result.X.(*goast.Ident).Name != "rt" {
					t.Fatalf("Main result = %#v, want rt.IO", f.Type.Results)
				}
			}
		}
		if entries != 1 {
			t.Errorf("entry count = %d, want 1", entries)
		}
	}
}
