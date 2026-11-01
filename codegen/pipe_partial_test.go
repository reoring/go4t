package codegen

import (
	"strings"
	"testing"

	"gofx/ast"
	"gofx/parser"
	"gofx/types"
)

func TestPipePartialSourceSmoke(t *testing.T) {
	file, err := parser.Parse(`package main
extern func finish(n Int) IO[Unit] = "example/foreign".Echo
func add(a Int, b Int) Int { a + b }
func main() IO[Unit] {
 let inc = add(1, _)
 let times = (2 * _)
 let combine = add(_, _)
 3
 |> inc
 |> add(4)
 |> times
 |> combine(_, 5)
 |> (func(n) => n + 1)
 |> finish
}
`)
	if err != nil {
		t.Fatal(err)
	}
	program, err := types.Check([]*ast.File{file})
	if err != nil {
		t.Fatal(err)
	}
	out := assertSource(t, program, "func(p0 any) any", "func(p0 any, p1 any) any", "rt.MulInt", "add_(", "finish_(")
	if strings.Contains(out, "|>") {
		t.Fatal("pipe operator survived parser desugaring")
	}
}
