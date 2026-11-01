package codegen

import (
	"testing"

	"gofx/ast"
	"gofx/modules"
	"gofx/parser"
	"gofx/types"
)

func TestProjectionSectionsRuntime(t *testing.T) {
	file, err := parser.Parse(`package main
type Row = Row{qty: Int}
type Box = Box{row: Row}
type Common[A] = First{value: A} | Second{count: Int, value: A}
func main() IO[Unit] {
  let rows = [Row{qty: 3}, Row{qty: 7}]
  let project: func(Row) Int = (_.qty)
  let values: func(List[Row]) List[Int] = map(_, _.qty)
  do {
    println(show(map(rows, (_.qty))))
    println(show(rows |> map(_.qty)))
    println(show(project(Row{qty: 9})))
    println(show(values(rows)))
    println(show(map([(3, "a"), (7, "b")], (_.0))))
    println(show(map([(3, "a"), (7, "b")], _.1)))
    println(show(map([((3, "a"), true)], _.0.1)))
    println(show(map([Box{row: Row{qty: 11}}], (_.row.qty))))
    println(show(map([Box{row: Row{qty: 13}}], _.row.qty)))
    println(show(map([First{value: 17}, Second{count: 99, value: 19}], _.value)))
  }
}`)
	if err != nil {
		t.Fatal(err)
	}
	loaded, err := modules.LoadFiles([]*ast.File{file}, ".")
	if err != nil {
		t.Fatal(err)
	}
	program, err := types.Check(loaded)
	if err != nil {
		t.Fatal(err)
	}
	generated, err := Generate(program, "main")
	if err != nil {
		t.Fatal(err)
	}
	want := "[3, 7]\n[3, 7]\n9\n[3, 7]\n[3, 7]\n[a, b]\n[a]\n[11]\n[13]\n[17, 19]\n"
	if got := runGeneratedModules(t, generated); got != want {
		t.Fatalf("output = %q, want %q", got, want)
	}
}
