package codegen

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"go4t/ast"
	"go4t/modules"
	"go4t/parser"
	"go4t/types"
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
import "std/result"
import "std/io"
import "std/string"
import "std/list"
import "std/prelude"
func checkPositive(n Int) Result[String, Int] {
  if n > 0 { Ok(n) } else { Err("nonpositive") }
}
func validate(text String) Result[String, Int] {
  do {
    n <- result.okOr(string.parseInt(text), "not an int")
    valid <- checkPositive(n)
    prelude.pure(valid * 2)
  }
}
func shortCircuit(r Result[String, Int]) Result[String, Int] {
  do { x <- r; prelude.pure(x / 0) }
}
func panicAction() IO[Int] { do { n <- ioPure(0); ioPure(1 / n) } }
func badIO() IO[Int] { io.ioFailWith(42) }
func checkError(e io.IoError) IO[Unit] {
  do {
    captured <- io.tryIO(io.ioRaise(e))
    let typed: Result[io.IoError, Int] = captured
    println("${typed == Err(e)}|${io.ioErrorMessage(e)}|${show(e)}|${debug(e)}")
  }
}
func main() IO[Unit] {
  do {
    let good: Result[String, Int] = Ok(4)
    let bad: Result[String, Int] = Err("bad")
    println(show(validate("3")))
    println(show(validate("no")))
    println(show(validate("0")))
    println(show(shortCircuit(bad)))
    println(show(prelude.fmap(good, func(n) => n + 1)))
    println(show(prelude.fmap(bad, func(n) => n / 0)))
    println(show(result.mapErr(bad, string.strLength)))
    println(show(result.mapErr(good, string.strLength)))
    println(show((result.fromResult(good, 9), result.fromResult(bad, 9))))
    println(show((result.toOption(good), result.toOption(bad))))
    println(show((result.isOk(good), result.isErr(good))))
    println(show((result.isOk(bad), result.isErr(bad))))
    println(result.either(good, func(e) => "error " + e, prelude.showInt))
    println(result.either(bad, func(e) => "error " + e, prelude.showInt))
    println(show((good == Ok(4), bad < good)))
    success <- io.tryIO(result.liftResult(good))
    failure <- io.tryIO(result.liftResult(bad))
    typedFailure <- io.tryIO(badIO())
    panicFailure <- io.tryIO(panicAction())
    println(show(success))
    println(show(failure))
    println(show(typedFailure))
    println(show(panicFailure))
    list.forEach([io.NotFound("missing"), io.PermissionDenied("private"), io.AlreadyExists("exists"), io.Timeout, io.Cancelled, io.Panic("boom"), io.Network("offline"), io.Failure("")], checkError)
    missing <- io.tryIO(io.readFile("/nonexistent"))
    match missing {
      Err(io.NotFound(message)) => println("not found: " + message)
      | _ => ioFail("expected NotFound")
    }
  }
}
`
	user, err := parser.Parse(source)
	if err != nil {
		t.Fatal(err)
	}
	modules, err := modules.LoadFiles([]*ast.File{user}, ".")
	if err != nil {
		t.Fatal(err)
	}
	program, err := types.Check(modules)
	if err != nil {
		t.Fatal(err)
	}
	generated, err := generate(program, "main")
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
		"Ok(4)", "Err(bad)", "Err(42)", "Err(runtime error: integer divide by zero)",
		`true|missing|missing|NotFound("missing")`,
		`true|private|private|PermissionDenied("private")`,
		`true|exists|exists|AlreadyExists("exists")`,
		`true|context deadline exceeded|context deadline exceeded|Timeout`,
		`true|context canceled|context canceled|Cancelled`,
		`true|boom|boom|Panic("boom")`,
		`true|offline|offline|Network("offline")`,
		`true|||Failure("")`, "",
	}, "\n")
	if !strings.HasPrefix(string(output), want+"not found: ") || !strings.Contains(string(output)[len(want):], "/nonexistent") {
		t.Fatalf("stdout = %q, want %q", output, want)
	}
}
