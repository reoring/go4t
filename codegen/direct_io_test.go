package codegen

import (
	"bytes"
	goast "go/ast"
	"go/format"
	goparser "go/parser"
	"go/token"
	"os"
	"strings"
	"testing"

	"go4t/ast"
	"go4t/types"
)

func TestDirectIORequiresStandardInstance(t *testing.T) {
	instance := &types.Instance{Class: &types.Class{Name: "Monad"}, TypeCon: types.IO, Decl: &ast.InstanceDecl{}}
	e := &ast.Do{ExprBase: annotated(&types.Ann{Monad: &types.DictRef{Kind: types.DictInstance, Instance: instance}})}
	g := &generator{
		standard: map[string]string{"Monad": "std/prelude.Monad"},
		sources:  map[ast.Decl]string{instance.Decl: "std/io/io.gt"},
	}
	if !g.directIO(e) {
		t.Fatal("standard IO instance not recognized")
	}
	g.sources[instance.Decl] = "user.gt"
	if g.directIO(e) {
		t.Fatal("user-defined instance must keep Bind even with the same rendered name")
	}
	g.sources[instance.Decl] = "std/io/io.gt"
	g.standard["Monad"] = "user.Monad"
	if g.directIO(e) {
		t.Fatal("user-defined class must keep Bind")
	}
}

func generatedFunction(t *testing.T, source []byte, name string) string {
	t.Helper()
	fset := token.NewFileSet()
	file, err := goparser.ParseFile(fset, "generated.go", source, 0)
	if err != nil {
		t.Fatal(err)
	}
	for _, decl := range file.Decls {
		if f, ok := decl.(*goast.FuncDecl); ok && f.Name.Name == name {
			var out bytes.Buffer
			if err := format.Node(&out, fset, f); err != nil {
				t.Fatal(err)
			}
			return out.String()
		}
	}
	t.Fatalf("missing generated function %s", name)
	return ""
}

func TestDirectIOLowering(t *testing.T) {
	source := qualityProgram(t, "direct.gt", `package main
import "std/prelude"
func direct() IO[Int] { do {
  (a, b) <- ioPure((2, 3))
  let n = a + b
  func plus(x Int) Int { x + n }
  let f = func(x Int) => plus(x)
  ioPure(unit())
  ioPure(f(n))
} }
func optional() Option[Int] { do { n <- Some(2); Some(n + 1) } }
func result() Result[String, Int] { do { n <- Ok(2); Ok(n + 1) } }
func generic[M[_] Monad](action M[Int]) M[Int] { do { n <- action; prelude.pure(n) } }
func main() IO[Unit] { do { n <- direct(); println(show(n)) } }
`)
	direct := generatedFunction(t, source, "direct_")
	for _, want := range []string{"rt.IO(", "context.Context", "rt.Run(ctx,", "var n_ int64"} {
		if !strings.Contains(direct, want) {
			t.Errorf("direct IO missing %q:\n%s", want, direct)
		}
	}
	if strings.Contains(direct, ".Bind(") || strings.Count(direct, "rt.IO(") != 1 {
		t.Fatalf("direct do must use one IO closure without Bind:\n%s", direct)
	}
	for _, name := range []string{"optional_", "result_", "generic_"} {
		body := generatedFunction(t, source, name)
		if !strings.Contains(body, ".Bind(") || strings.Contains(body, "rt.Run(") {
			t.Errorf("nonconcrete IO must keep Bind:\n%s", body)
		}
	}
	output, err := runQualityGo(t, source, false)
	if err != nil || output != "10\n" {
		t.Fatalf("direct IO patterns/local functions: %v\n%s", err, output)
	}
}

func TestDirectIOExecutionBoundaries(t *testing.T) {
	source := qualityProgram(t, "direct-boundaries.gt", `package main
import "std/io"
func sequence(make func() Int, first IO[Int], next func(Int) IO[Int]) IO[Int] { do {
  let offset = make()
  n <- first
  next(n + offset)
} }
func failed() IO[Int] { do { ioFail("step failed"); ioPure(42) } }
func after(first IO[Int], calc func(Int) Int) IO[Int] { do {
  n <- first
  let answer = calc(n)
  ioPure(answer)
} }
func failedBeforePanic(zero Int) IO[Int] { do {
  ioFail("first failure")
  let impossible = 1 / zero
  ioPure(impossible)
} }
func main() IO[Unit] { do {
  recovered <- io.tryIO(failed())
  println(debug(recovered))
  ordered <- io.tryIO(failedBeforePanic(0))
  println(debug(ordered))
} }
`)
	source = append(source, []byte(`
func init() {
  calls := 0
  makeValue := func() any { calls++; return int64(2) }
  first := rt.IO(func(context.Context) (any, error) { calls++; return int64(3), nil })
  next := func(n any) any { calls++; return rt.Pure(n) }
  action := sequence_(makeValue, first, next).(rt.IO)
  if calls != 0 { panic("IO construction performed work") }
  for i := 1; i <= 2; i++ {
    value, err := action(context.Background())
    if err != nil || value != int64(5) || calls != i * 3 { panic("IO rerun/order") }
  }
  ctx, cancel := context.WithCancel(context.Background())
  cancelled := rt.IO(func(context.Context) (any, error) { cancel(); return int64(3), nil })
  reached := false
  never := func(any) any { reached = true; return rt.Pure(int64(0)) }
  _, err := sequence_(makeValue, cancelled, never).(rt.IO)(ctx)
  if reached || err.(*rt.Failure).Kind != rt.Cancelled { panic("cancellation did not stop the next step") }
  entered := false
  before := rt.IO(func(context.Context) (any, error) { entered = true; return int64(0), nil })
  _, err = sequence_(makeValue, before, never).(rt.IO)(ctx)
  if entered || reached || err.(*rt.Failure).Kind != rt.Cancelled { panic("pre-cancelled effect ran") }
  _, err = sequence_(makeValue, rt.Fail("step failed"), never).(rt.IO)(context.Background())
  if reached || err.(*rt.Failure).Kind != rt.FailureKind || err.Error() != "step failed" { panic("failed step did not short circuit") }
  letCalls := 0
  calc := func(n any) any { letCalls++; return n.(int64) + 1 }
  _, err = after_(rt.Fail("before let"), calc).(rt.IO)(context.Background())
  if letCalls != 0 || err.Error() != "before let" { panic("let ran after failed step") }
  ordered := rt.IO(func(context.Context) (any, error) { letCalls = 10; return int64(2), nil })
  value, err := after_(ordered, calc).(rt.IO)(context.Background())
  if letCalls != 11 || value != int64(3) || err != nil { panic("let was hoisted before effect") }
}
`)...)
	output, err := runQualityGo(t, source, false)
	if err != nil || output != "Err(Failure(\"step failed\"))\nErr(Failure(\"first failure\"))\n" {
		t.Fatalf("direct IO execution boundaries: %v\n%s", err, output)
	}
}

func TestMillionIOTailIterationsWithoutStackGrowth(t *testing.T) {
	source := qualityProgram(t, "io-million.gt", `package main
func loop(n Int, a Int, b Int) IO[Int] {
  if n == 0 { ioPure(a * 10 + b) } else { do {
    next <- ioPure(n - 1)
    let remaining = next
    match remaining { 0 => loop(remaining, b, a); _ => loop(remaining, b, a) }
  } }
}
func direct(n Int) IO[Int] { if n == 0 { ioPure(7) } else { direct(n - 1) } }
func forever() IO[Unit] { forever() }
func captures(n Int, fs List[func(Unit) Int]) IO[Int] {
  if n == 0 { ioPure(fold(fs, 0, func(acc, f) => acc + f(unit()))) }
  else { do { ioPure(unit()); captures(n - 1, Cons(func(_) => n, fs)) } }
}
func main() IO[Unit] { do {
  let action = loop(1000000, 1, 2)
  first <- action
  second <- action
  plain <- direct(1000000)
  captured <- captures(3, Nil)
  println("${first} ${second} ${plain} ${captured}")
} }
`)
	for _, name := range []string{"loop_", "direct_", "forever_", "captures_"} {
		body := generatedFunction(t, source, name)
		for _, want := range []string{"rt.IO(", "for {", "ctx.Err()", "continue"} {
			if !strings.Contains(body, want) {
				t.Fatalf("IO tail function missing %q:\n%s", want, body)
			}
		}
	}
	source = append(source, []byte(`
type countdownContext struct { context.Context; remaining int }
func (ctx *countdownContext) Err() error {
  ctx.remaining--
  if ctx.remaining <= 0 { return context.Canceled }
  return nil
}
func init() {
  action := forever_().(rt.IO)
  ctx := &countdownContext{Context: context.Background(), remaining: 100}
  _, err := action(ctx)
  if err.(*rt.Failure).Kind != rt.Cancelled || ctx.remaining != 0 { panic("IO loop must check cancellation each iteration") }
}
`)...)
	output, err := runQualityGo(t, source, true)
	if err != nil || output != "12 12 7 6\n" {
		t.Fatalf("million IO iterations: %v\n%s", err, output)
	}
}

const nonTailIOProgram = `package main
import "std/io"
import "std/conc"
func middle(n Int) IO[Int] {
  if n == 0 { ioPure(0) } else { do {
    value <- middle(n - 1)
    ioPure(value + 1)
  } }
}
func main() IO[Unit] { do {
  fiber <- conc.fork(middle(5))
  forked <- conc.await(fiber)
  folded <- io.foldLines("INPUT", 0, func(total, _line) => do {
    value <- middle(6)
    ioPure(total + value)
  })
  recovered <- io.tryIO(middle(7))
  println("${forked} ${folded} ${debug(recovered)}")
} }
`

func TestNonTailIOCallsKeepContinuations(t *testing.T) {
	path := t.TempDir() + "/lines.txt"
	if err := os.WriteFile(path, []byte("one\ntwo\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	source := qualityProgram(t, "io-middle.gt", strings.ReplaceAll(nonTailIOProgram, "INPUT", path))
	body := generatedFunction(t, source, "middle_")
	if strings.Contains(body, "continue") || !strings.Contains(body, "rt.Run(ctx, middle_(") {
		t.Fatalf("mid-do recursive call must retain its continuation:\n%s", body)
	}
	output, err := runQualityGo(t, source, false)
	if err != nil || output != "5 12 Ok(7)\n" {
		t.Fatalf("non-tail IO under fork/foldLines/tryIO: %v\n%s", err, output)
	}
}
