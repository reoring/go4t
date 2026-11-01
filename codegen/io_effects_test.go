package codegen

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"gofx/ast"
	"gofx/parser"
	"gofx/prelude"
	"gofx/types"
)

func TestResultShapeRegistration(t *testing.T) {
	for _, withListOption := range []bool{false, true} {
		p := fixture()
		if withListOption {
			list(p)
			option(p)
		}
		result := &types.DataType{Name: "Result"}
		result.Ctors = []*types.Ctor{
			{Name: "Err", Data: result, Fields: []types.Type{types.String}},
			{Name: "Ok", Data: result, Fields: []types.Type{types.Int}, Index: 1},
		}
		p.DataTypes[result.Name] = result
		assertSource(t, p,
			"func rt_ok(a any) any", "return &Result_Ok{F0: a}",
			"func rt_err(e any) any", "return &Result_Err{F0: e}",
			"rt.Shapes.Ok = rt_ok", "rt.Shapes.Err = rt_err")
	}
}

func TestResultHelpersExecute(t *testing.T) {
	const source = `package main
func checkPositive(n Int) Result[String, Int] {
  if n > 0 { Ok(n) } else { Err("nonpositive") }
}
func validate(text String) Result[String, Int] {
  do {
    n <- okOr(parseInt(text), "not an int")
    valid <- checkPositive(n)
    pure(valid * 2)
  }
}
func shortCircuit(r Result[String, Int]) Result[String, Int] {
  do { x <- r; pure(x / 0) }
}
func panicAction() IO[Int] { do { n <- ioPure(0); ioPure(1 / n) } }
func badIO() IO[Int] { ioFailWith(42) }
func main() IO[Unit] {
  do {
    let good: Result[String, Int] = Ok(4)
    let bad: Result[String, Int] = Err("bad")
    println(show(validate("3")))
    println(show(validate("no")))
    println(show(validate("0")))
    println(show(shortCircuit(bad)))
    println(show(fmap(good, func(n) => n + 1)))
    println(show(fmap(bad, func(n) => n / 0)))
    println(show(mapErr(bad, strLength)))
    println(show(mapErr(good, strLength)))
    println(show((fromResult(good, 9), fromResult(bad, 9))))
    println(show((toOption(good), toOption(bad))))
    println(show((isOk(good), isErr(good))))
    println(show((isOk(bad), isErr(bad))))
    println(either(good, func(e) => "error " + e, showInt))
    println(either(bad, func(e) => "error " + e, showInt))
    println(show((good == Ok(4), bad < good)))
    success <- tryIO(liftResult(good))
    failure <- tryIO(liftResult(bad))
    typedFailure <- tryIO(badIO())
    panicFailure <- tryIO(panicAction())
    println(show(success))
    println(show(failure))
    println(show(typedFailure))
    println(show(panicFailure))
  }
}
`
	var files []*ast.File
	for _, src := range []string{prelude.Source, source} {
		file, err := parser.Parse(src)
		if err != nil {
			t.Fatal(err)
		}
		files = append(files, file)
	}
	program, err := types.Check(files)
	if err != nil {
		t.Fatal(err)
	}
	generated, err := Generate(program, "main")
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "main.go")
	if err := os.WriteFile(path, generated, 0o600); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("go", "run", path)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("go run: %v\n%s", err, output)
	}
	want := strings.Join([]string{
		"Ok(6)", "Err(not an int)", "Err(nonpositive)", "Err(bad)",
		"Ok(5)", "Err(bad)", "Err(3)", "Ok(4)", "(4, 9)",
		"(Some(4), None)", "(true, false)", "(false, true)", "4", "error bad", "(true, true)",
		"Ok(4)", "Err(bad)", "Err(42)", "Err(runtime error: integer divide by zero)", "",
	}, "\n")
	if string(output) != want {
		t.Fatalf("stdout = %q, want %q", output, want)
	}
}
