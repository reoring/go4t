package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

const patternsProgram = `package main

type Color = A | B | C deriving Eq, Debug
type Choice = Left(Int) | Right(Int)
type Row = Row{color: Color, value: Int}
func color(x Color) Int { match x { | A | B => 1 | C => 0 } }
func bound(x Choice) Int { match x { Left(n) | Right(n) => n } }
func guarded(pair (Int, Int)) Int {
  match pair { (n, _) | (_, n) if n > 0 => n; _ => 0 }
}
func nested(x Option[Color]) Int { match x { Some(A | B) => 1; Some(C) | None => 0 } }
func record(x Row) Int { match x { Row{color: A | B, value: n} => n; Row{..} => 0 } }
func integer(x Int) Int { match x { -1 | -2 => 1; 3 | 4 => 2; _ => 0 } }
func text(x String) Int { match x { "a" | "b" => 1; _ => 0 } }
func floating(x Float) Int { match x { -2.5 | -1.5 => 1; _ => 0 } }
func tuple(x (Color, Color)) Int { match x { (A | B, A | B,) => 1; _ => 0 } }
func list(xs List[Color]) Int { match xs { [A | B, ..rest] => 1 + count(rest, 0); _ => 0 } }
func asPattern(x Color) Color { match x { whole @ (A | B) => whole; C => C } }
func count(xs List[Color], n Int) Int {
  match xs { Cons(A | B, rest) => count(rest, n + 1); Cons(C, rest) => count(rest, n); Nil => n }
}
func make(n Int, xs List[Color]) List[Color] { if n == 0 { xs } else { make(n - 1, Cons(A, xs)) } }
func main() IO[Unit] {
  do {
    println("color: ${color(A)},${color(B)},${color(C)}")
    println("bound: ${bound(Left(2))},${bound(Right(3))}")
    println("guard: ${guarded((-1, 7))},${guarded((2, 7))},${guarded((-1, -2))}")
    println("nested: ${nested(Some(A))},${nested(Some(B))},${nested(Some(C))},${nested(None)}")
    println("record: ${record(Row(B, 8))},${record(Row(C, 8))}")
    println("int: ${integer(-1)},${integer(-2)},${integer(3)},${integer(4)},${integer(5)}")
    println("string: ${text("a")},${text("b")},${text("c")}")
    println("float: ${floating(-2.5)},${floating(-1.5)},${floating(0.0)}")
    println("tuple: ${tuple((A, B))},${tuple((B, A))},${tuple((B, C))}")
    println("list: ${list([A, B, C])},${list([B, A])},${list([C])}")
    println("as: ${asPattern(A) == A},${asPattern(B) == B}")
    println("tail: ${count(make(100000, []), 0)}")
    ioPure(unit())
  }
}
`

const patternsOutput = `color: 1,1,0
bound: 2,3
guard: 7,2,0
nested: 1,1,0,0
record: 8,0
int: 1,1,2,2,0
string: 1,1,0
float: 1,1,0
tuple: 1,1,0
list: 2,2,0
as: true,true
tail: 100000
`

func TestOrPatternsGeneratedProgram(t *testing.T) {
	dir := t.TempDir()
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	mod := "module patternstest\n\ngo 1.22\n\nrequire github.com/reoring/go4t v0.0.0\nreplace github.com/reoring/go4t => " + root + "\n"
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte(mod), 0o644); err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(dir, "patterns.gt")
	if err := os.WriteFile(file, []byte(patternsProgram), 0o644); err != nil {
		t.Fatal(err)
	}
	bin := filepath.Join(dir, "patterns")
	if err := run([]string{"build", "-o", bin, file}); err != nil {
		t.Fatal(err)
	}
	command := exec.Command(bin)
	command.Dir = dir
	output, err := command.CombinedOutput()
	if err != nil || string(output) != patternsOutput {
		t.Fatalf("program error = %v\noutput:\n%s\nwant:\n%s", err, output, patternsOutput)
	}
}
