package codegen

import (
	"testing"

	"github.com/reoring/go4t/ast"
	"github.com/reoring/go4t/parser"
	"github.com/reoring/go4t/types"
)

func TestDeclsUnnamedConstants(t *testing.T) {
	file, err := parser.Parse(`package main
let number = 42
let text = "constant"
let flag = true
let fraction = -1.5
type Row = {qty: Int}
let row: Row = Row{qty: number}
let callback: func(Int) Int = func(n Int) => n + number
func use() Int { let f = callback; f(row.qty) }
func main() IO[Unit] { main() }
`)
	if err != nil {
		t.Fatal(err)
	}
	p, err := types.Check([]*ast.Module{{Files: []*ast.File{file}}})
	if err != nil {
		t.Fatal(err)
	}
	assertSource(t, p, "func number_() int64", "number_()", "func row_() any", "type Row_Row struct")
}
