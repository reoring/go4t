package types_test

import (
	"gofx/ast"
	"gofx/parser"
	"gofx/types"
	"strings"
	"testing"
)

func indexPos(source, needle string, offset int) ast.Pos {
	at := strings.Index(source, needle) + offset
	prefix := source[:at]
	return ast.Pos{Line: strings.Count(prefix, "\n") + 1, Col: len([]rune(prefix[strings.LastIndex(prefix, "\n")+1:])) + 1}
}
func TestPositionIndexScopesAndComments(t *testing.T) {
	source := `package main
// The identity function.
// Preserves its input type.
func identity[A](x A) A { x }
// Not documentation: a blank line intervenes.

func count() Int { 1 }
func main() IO[Unit] {
 let x = 1
 let f = func(x String) => x
 let y = { let x = x + 1; x }
 let recursive = func(n Int) => if n == 0 { 0 } else { recursive(n - 1) }
 func even(n Int) Bool { if n == 0 { true } else { odd(n - 1) } }
 func odd(n Int) Bool { if n == 0 { false } else { even(n - 1) } }
 identity(x)
 f("x")
 y
 recursive(1)
 even(2)
 ioPure(unit())
}
`
	pre, err := parser.Parse(testPrelude)
	if err != nil {
		t.Fatal(err)
	}
	pre.Name = "prelude.gfx"
	file, err := parser.Parse(source)
	if err != nil {
		t.Fatal(err)
	}
	file.Name = "main.gfx"
	program, err := types.Check([]*ast.Module{{Files: []*ast.File{pre, file}}})
	if err != nil {
		t.Fatal(err)
	}
	index := program.PositionIndex()
	for _, tt := range []struct {
		needle      string
		offset      int
		declaration string
		declOffset  int
		typ         string
	}{
		{"=> x", 3, "func(x String)", 5, "String"},
		{"x + 1", 0, "let x = 1", 4, "Int"},
		{"; x }", 2, "let x = x", 4, "Int"},
		{"identity(x)", 9, "let x = 1", 4, "Int"},
		{"recursive(n -", 0, "let recursive", 4, "func(Int) Int"},
		{"odd(n -", 0, "func odd", 5, "func(Int) Bool"},
		{"x A) A", 2, "identity[A]", 9, "A"},
	} {
		binding := index.At(file.Name, indexPos(source, tt.needle, tt.offset))
		if binding == nil || binding.Definition == nil {
			t.Fatalf("missing %s binding: %+v", tt.needle, binding)
		}
		if binding.Definition.NamePos != indexPos(source, tt.declaration, tt.declOffset) || types.TypeString(binding.Type) != tt.typ {
			t.Errorf("%s: %+v definition=%+v type=%s", tt.needle, binding, binding.Definition, types.TypeString(binding.Type))
		}
	}
	binding := index.At(file.Name, indexPos(source, "identity[A]", 0))
	if binding.Definition.Doc != "The identity function.\nPreserves its input type." || types.TypeString(binding.Type) != "func(A) A" {
		t.Fatalf("identity: %+v", binding)
	}
	count := index.At(file.Name, indexPos(source, "count()", 0))
	if count.Definition.Doc != "" {
		t.Errorf("nonadjacent doc: %q", count.Definition.Doc)
	}
	if index.At(file.Name, indexPos(source, "identity[A]", 8)) != nil {
		t.Error("exclusive identifier end included")
	}
	if index.At("unknown.gfx", ast.Pos{Line: 1, Col: 1}) != nil {
		t.Error("unknown file has binding")
	}
	// Index construction must not consume or modify checking state.
	if _, err := types.Check([]*ast.Module{{Files: []*ast.File{pre, file}}}); err != nil {
		t.Fatal(err)
	}
}

func TestPositionIndexInsideInterpolation(t *testing.T) {
	source := `package main
type Box = Box{value: Int}
func main() IO[Unit] {
 let box = Box{value: 1}
 // A local identity.
 func outer(n Int) Int { n }
 let text = "${{ func local(n Int) Int { n }; local(box.value) }}"
 outer(1)
 ioPure(unit())
}
`
	pre, err := parser.Parse(testPrelude)
	if err != nil {
		t.Fatal(err)
	}
	pre.Name = "prelude.gfx"
	file, err := parser.Parse(source)
	if err != nil {
		t.Fatal(err)
	}
	file.Name = "main.gfx"
	program, err := types.Check([]*ast.Module{{Files: []*ast.File{pre, file}}})
	if err != nil {
		t.Fatal(err)
	}
	index := program.PositionIndex()
	for _, tt := range []struct {
		needle      string
		offset      int
		declaration string
		declOffset  int
	}{
		{"local(box", 0, "func local", 5},
		{"box.value", 4, "value: Int", 0},
	} {
		binding := index.At(file.Name, indexPos(source, tt.needle, tt.offset))
		if binding == nil || binding.Definition == nil || binding.Definition.NamePos != indexPos(source, tt.declaration, tt.declOffset) {
			t.Fatalf("definition of %s: %+v", tt.needle, binding)
		}
	}
	binding := index.At(file.Name, indexPos(source, "outer(1)", 0))
	if binding == nil || binding.Definition == nil || binding.Definition.Doc != "A local identity." {
		t.Fatalf("local function documentation: %+v", binding)
	}
}
