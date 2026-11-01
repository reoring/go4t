package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestPass9GeneratedProgram(t *testing.T) {
	dir := t.TempDir()
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	mod := "module pass9test\n\ngo 1.22\n\nrequire gofx v0.0.0\nreplace gofx => " + root + "\n"
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte(mod), 0o644); err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(dir, "pass9.gfx")
	source := `package main

type Box[A] = Box{value: A}
instance Eq[Box[A]] given Eq[A] {
    eq(a, b) { match (a, b) { | (Box(x), Box(y)) => x == y } }
}
instance Ord[Box[A]] given Ord[A] {
    compare(a, b) { match (a, b) { | (Box(x), Box(y)) => compare(x, y) } }
}
func crash() Bool { 1 / 0 == 0 }
func same[A Ord](a A, b A) Bool { a == b }
func guarded(x Option[Int]) Int {
    match x {
    | Some(n) if n > 2 => n
    | Some(n) if n > 0 => n + 10
    | Some(n) => n + 20
    | None => 0
    }
}
func main() IO[Unit] {
    let pair: func((Int, Int)) Int = func((a, b)) => a + b
    let boxed: func(Box[Int]) Int = func(Box{value: x}) => x
    let aliased: func((Int, Int)) Int = func(whole @ (x, _)) => whole.1 + x
    let ignored: func(Int) Int = func(_) => 8
    do {
        println(show((false && crash(), true || crash())))
        println("(${true && true}, ${true && false}, ${false || true}, ${false || false})")
        println("(${1 == 1}, ${1 != 2}, ${"a" == "a"}, ${false != true})")
        println("(${1 < 2}, ${1 <= 1}, ${2 > 1}, ${1 >= 1})")
        println("(${2 < 1}, ${2 <= 1}, ${1 > 2}, ${1 >= 2})")
        println("(${1 < 1}, ${1 <= 2}, ${1 > 1}, ${2 >= 1})")
        println("(${"a" < "b"}, ${false < true}, ${EQ < GT}, ${Some(1) < Some(2)})")
        println("(${Box(1) == Box(1)}, ${Box(1) < Box(2)}, ${same(Box(1), Box(1))})")
        println("(${Box(Box(1)) < Box(Box(2))}, ${[1, 2] == [1, 2]}, ${(1, 2) < (1, 3)})")
        println("(${pair((2, 3))}, ${boxed(Box(7))}, ${aliased((4, 6))}, ${ignored(0)})")
        println("(${guarded(Some(4))}, ${guarded(Some(2))}, ${guarded(Some(0))}, ${guarded(None)})")
        println(show((minInt(), maxInt())))
        pure(unit())
    }
}
`
	if err := os.WriteFile(file, []byte(source), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := run([]string{"build", file}); err != nil {
		t.Fatal(err)
	}
	command := exec.Command("go", "run", "./pass9_gen")
	command.Dir = dir
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("go run ./pass9_gen: %v\n%s", err, output)
	}
	want := strings.Join([]string{
		"(false, true)",
		"(true, false, true, false)",
		"(true, true, true, true)",
		"(true, true, true, true)",
		"(false, false, false, false)",
		"(false, true, false, true)",
		"(true, true, true, true)",
		"(true, true, true)",
		"(true, true, true)",
		"(5, 7, 10, 8)",
		"(4, 12, 20, 0)",
		"(-9223372036854775808, 9223372036854775807)",
	}, "\n") + "\n"
	if string(output) != want {
		t.Errorf("generated program output:\n%s\nwant:\n%s", output, want)
	}
}
