package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

const declsProgram = `package main
type Row = {qty: Int} deriving Eq, Debug
alias Pair[A,] = (A, A,)
let base = 3
let _offset = 2
let limit: Int = add(base, _offset)
func add(a Int, b Int) Int { a + b }
func duplicate[A,](x A) Pair[A,] { (x, x,) }
func main() IO[Unit] { do {
  let row = Row{qty: limit}
  let updated = row{qty: row.qty + 1}
  let amount = match updated { Row{qty: n} => n }
  println(debug(updated))
  println(show(row == Row(limit)))
  println(show(amount))
  println(show(duplicate(limit)))
  println(show([1, 2] |> map(_ + limit)))
  println(show(fold([1, 2, 3], 0, _ + _)))
} }
`

func TestDeclsRun(t *testing.T) {
	dir := toolsModule(t)
	file := toolsWrite(t, filepath.Join(dir, "main.gt"), declsProgram)
	out, stderr, status := toolsCommand(t, dir, "", "run", file)
	want := "Row{qty: 6}\ntrue\n6\n(5, 5)\n[6, 7]\n6\n"
	if out != want || stderr != "" || status != 0 {
		t.Fatalf("stdout=%q stderr=%q status=%d", out, stderr, status)
	}
}

func TestDeclsModuleRun(t *testing.T) {
	dir := toolsModule(t)
	toolsWrite(t, filepath.Join(dir, "go4t.mod"), "module decls\n")
	if err := os.Mkdir(filepath.Join(dir, "values"), 0o755); err != nil {
		t.Fatal(err)
	}
	toolsWrite(t, filepath.Join(dir, "values", "values.gt"), `package values
let _private = 4
let answer: Int = increase(_private)
func increase(n Int) Int { n + 1 }
`)
	file := toolsWrite(t, filepath.Join(dir, "main.gt"), `package main
import "decls/values"
let answer: Int = values.answer
func main() IO[Unit] { println(show(answer)) }
`)
	out, stderr, status := toolsCommand(t, dir, "", "run", file)
	if out != "5\n" || stderr != "" || status != 0 {
		t.Fatalf("stdout=%q stderr=%q status=%d", out, stderr, status)
	}
	toolsWrite(t, file, "package main\nimport \"decls/values\"\nfunc main() IO[Unit] { println(show(values._private)) }\n")
	_, stderr, status = toolsCommand(t, dir, "", "check", file)
	if status != 1 || !strings.Contains(stderr, "private") {
		t.Fatalf("privacy stderr=%q status=%d", stderr, status)
	}
}

func TestDeclsEmitGoExport(t *testing.T) {
	dir := toolsModule(t)
	file := toolsWrite(t, filepath.Join(dir, "source.gt"), `package main
type Row = {qty: Int} deriving Eq, Debug
let limit = 7
let _private = 9
let row: Row = Row{qty: limit}
func main() IO[Unit] { ioPure(unit()) }
`)
	out, stderr, status := toolsCommand(t, dir, "", "build", "--emit-go", filepath.Join(dir, "embedded"), "--package", "embedded", file)
	if out != "" || stderr != "" || status != 0 {
		t.Fatalf("emit: %q %q %d", out, stderr, status)
	}
	toolsWrite(t, filepath.Join(dir, "main.go"), `package main
import (
 "fmt"
 "clitoolstest/embedded"
)
func main() {
 var n int64 = embedded.Limit()
 var row embedded.Row = embedded.RowFn()
 fmt.Println(n, row.Qty)
}
`)
	command := exec.Command("go", "run", ".")
	command.Dir = dir
	if output, err := command.CombinedOutput(); err != nil || string(output) != "7 7\n" {
		t.Fatalf("export: %s %v", output, err)
	}
	generated, err := os.ReadFile(filepath.Join(dir, "embedded", "go4t_export.go"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(generated), "func Private(") {
		t.Fatal("private constant exported")
	}
}

func TestDeclsTestDiscovery(t *testing.T) {
	dir := toolsModule(t)
	file := toolsWrite(t, filepath.Join(dir, "tests.gt"), `package main
let testFalse = false
let go4tTestRun = false
func testOrdinary() Bool { !testFalse }
`)
	out, stderr, status := toolsCommand(t, dir, "", "test", file)
	if out != "PASS testOrdinary\n1 passed, 0 failed\n" || stderr != "" || status != 0 {
		t.Fatalf("stdout=%q stderr=%q status=%d", out, stderr, status)
	}
}

func TestDeclsPreludeShadow(t *testing.T) {
	dir := toolsModule(t)
	file := toolsWrite(t, filepath.Join(dir, "main.gt"), "package main\nlet length = 3\nfunc main() IO[Unit] { println(show(length)) }\n")
	_, stderr, status := toolsCommand(t, dir, "", "check", file)
	if status != 0 || !strings.Contains(stderr, "shadows prelude") {
		t.Fatalf("stderr=%q status=%d", stderr, status)
	}
}
