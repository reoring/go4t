package std_test

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"gofx/ast"
	"gofx/codegen"
	"gofx/modules"
	"gofx/parser"
	"gofx/types"
)

func TestDerivingPreludeProgram(t *testing.T) {
	const source = `package main
import "std/list"
import "std/state"
import "std/prelude"

type Tree[A] = Tip | Leaf(A) | Fork(Tree[A], Tree[A]) deriving Functor, Foldable, Traversable, Debug
type Nested[E, A] = Nested{tag: E, values: (A, List[Option[A]], (String, A)), children: List[Nested[E, A]]} deriving Functor, Foldable, Traversable
type Record = Record{number: Int, text: String} deriving Debug
type Empty[A] = Empty deriving Functor, Foldable, Traversable
instance Show[Record] { show(a) { "display" } }
func emptyOption() Option[Int] { None }
func emptyList() List[Int] { Nil }
func safe(n Int) Option[Int] { if n == 0 { None } else { Some(10 / (n - 1)) } }
func step(n Int) state.State[List[Int], Int] { do { state.modify(func(xs) => append(xs, [n])); prelude.pure(n + 10) } }
func main() IO[Unit] {
  let tree = Fork(Leaf(1), Fork(Leaf(2), Leaf(3)))
  let tip: Tree[Int] = Tip
  let empty: Empty[Int] = Empty
  let nested = Nested("keep", (1, [Some(2), None], ("tag", 3)), [Nested("child", (4, [], ("fixed", 5)), [])])
  let ordered = prelude.traverse(nested, step)
  do {
    println("${1}|${"raw"}|${true}|${Some("raw")}|${emptyOption()}|${["a", "b"]}|${("raw", false)}|${LT}|${EQ}|${GT}")
    println("${debug(1)}|${debug("a\n\"\\\t")}|${debug(false)}|${debug(unit())}|${debug(GT)}")
    println("${debug(Some("x"))}|${debug(emptyOption())}|${debug(["a", "b"])}|${debug(emptyList())}|${debug(("x", true))}")
    println("${Record(1, "x")}|${debug(Record(1, "x"))}")
    println(debug(tree))
    println(show(list.toList(prelude.fmap(tree, func(n) => n + 10))))
    println("${prelude.foldr(tree, "z", func(n, rest) => "(" + show(n) + rest + ")")}|${list.foldl(tree, 0, func(acc, n) => acc * 10 + n)}")
    println("${list.toList(tip)}|${list.toList(empty)}|${list.toList(emptyOption())}|${list.toList(Some(7))}")
    println("${prelude.foldr(emptyList(), 9, func(a, b) => a - b)}|${prelude.foldr(Some(3), 9, func(a, b) => a - b)}|${list.foldl(Some(3), 9, func(a, b) => a - b)}")
    println(show(list.toList(prelude.fmap(nested, func(n) => n * 2))))
    println(show(list.foldMapList(nested, func(n) => [n, 0 - n])))
    println(show(state.execState(ordered, [])))
    println(show(list.toList(state.evalState(ordered, []))))
    println("${prelude.traverse([0, 1], safe)}|${debug(prelude.traverse(Fork(Leaf(0), Leaf(1)), safe))}")
    println("${prelude.traverse(emptyList(), safe)}|${prelude.traverse(emptyOption(), safe)}|${prelude.traverse(Some(3), safe)}")
  }
}
`
	var user *ast.File
	var err error
	user, err = parser.Parse(source)
	if err != nil {
		t.Fatal(err)
	}
	user.Name = "deriving.gfx"
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
	mod := "module derivingtest\n\ngo 1.22\n\nrequire gofx v0.0.0\nreplace gofx => " + root + "\n"
	generated["go.mod"] = []byte(mod)
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
		t.Fatalf("go run generated deriving program: %v\n%s", err, output)
	}
	want := strings.Join([]string{
		`1|raw|true|Some(raw)|None|[a, b]|(raw, false)|LT|EQ|GT`,
		`1|"a\n\"\\\t"|false|()|GT`,
		`Some("x")|None|Cons("a", Cons("b", Nil))|Nil|("x", true)`,
		`display|Record{number: 1, text: "x"}`,
		`Fork(Leaf(1), Fork(Leaf(2), Leaf(3)))`,
		`[11, 12, 13]`,
		`(1(2(3z)))|123`,
		`[]|[]|[]|[7]`,
		`9|-6|6`,
		`[2, 4, 6, 8, 10]`,
		`[1, -1, 2, -2, 3, -3, 4, -4, 5, -5]`,
		`[1, 2, 3, 4, 5]`,
		`[11, 12, 13, 14, 15]`,
		`None|None`,
		`Some([])|Some(None)|Some(Some(5))`,
	}, "\n") + "\n"
	if string(output) != want {
		t.Errorf("generated output:\n%s\nwant:\n%s", output, want)
	}
}
