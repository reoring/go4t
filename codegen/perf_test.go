package codegen

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestPerfScalarBoundaries(t *testing.T) {
	source := qualityProgram(t, "scalar.gfx", `package main
func addInts(a Int, b Int) Int { let result = a + b; result }
func scale(x Float, yes Bool) Float { if yes { x * 2.0 } else { -x } }
func loopInts(n Int, total Int) Int { if n == 0 { total } else { loopInts(n - 1, total + n) } }
func main() IO[Unit] {
 let f = addInts
 let pair = (f(20, 22), scale(1.5, true))
 let (n, _) = pair
 do {
  a <- ioPure(n)
  println("${a} ${pair.1} ${loopInts(1000000, 0)} ${map([1, 2], func(n) => addInts(n, 1))}")
 }
}`)
	for _, fragment := range []string{"func addInts_(a_ int64, b_ int64) int64", "var result_ int64 = (a_ + b_)", "func scale_(x_ float64, yes_ bool) float64", "for {", "//line scalar.gfx:"} {
		if !bytes.Contains(source, []byte(fragment)) {
			t.Errorf("missing %q", fragment)
		}
	}
	output, err := runQualityGo(t, source, false)
	if err != nil || output != "42 3 500000500000 [2, 3]\n" {
		t.Fatalf("output %q: %v", output, err)
	}
}

func TestPerfSpecialization(t *testing.T) {
	source := qualityProgram(t, "special.gfx", `package main
func pick[A Ord](a A, b A) A { if a < b { b } else { a } }
func main() IO[Unit] { println("${pick(1, 2)} ${pick(2.0, 1.0)} ${pick(3, 4)}") }
`)
	if !strings.Contains(string(source), "func pick__special1(a_ int64, b_ int64) int64") {
		t.Fatal("missing concrete Int copy")
	}
	if strings.Count(string(source), "func pick__special") != 2 {
		t.Fatal("copies are not deduplicated by dictionaries")
	}
	output, err := runQualityGo(t, source, false)
	if err != nil || output != "2 2 4\n" {
		t.Fatalf("output %q: %v", output, err)
	}
}

func TestPerfNumericRuntimeSemantics(t *testing.T) {
	source := qualityProgram(t, "numbers.gfx", `package main
import "std/float"
func main() IO[Unit] {
 println("${9223372036854775807 + 1} ${1.0 / 0.0} ${float.isNaN(0.0 / 0.0)} ${show(-0.0)} ${float.nan() < 1.0} ${float.nan() == float.nan()}")
}`)
	output, err := runQualityGo(t, source, false)
	if err != nil || output != "-9223372036854775808 +Inf true -0 true false\n" {
		t.Fatalf("output %q: %v", output, err)
	}
}

func TestPerfOperators(t *testing.T) {
	source := qualityProgram(t, "operators.gfx", `package main
import "std/float"
func ints(a Int, b Int) (List[Int], List[Bool]) {
 ([a + b, a - b, a * b, a / b, a % b], [a == b, a != b, a < b, a <= b, a > b, a >= b])
}
func floats(a Float, b Float) (List[Float], Bool) {
 ([a + b, a - b, a * b, a / b, -a], a != b)
}
func boom() Bool { 1 / 0 == 0 }
func main() IO[Unit] { do {
 println("${ints(7, 2)} ${floats(1.5, 0.5)} ${false && boom()} ${true || boom()} ${!false}")
 println("${float.nan() <= float.nan()} ${float.nan() >= float.nan()} ${float.nan() > 0.0} ${0.0 > float.nan()} ${0.0 >= float.nan()} ${float.nan() <= 0.0} ${0.0 <= float.nan()} ${false < true} ${false <= false} ${true > false} ${true >= true}")
} }`)
	output, err := runQualityGo(t, source, false)
	want := "([9, 5, 14, 3, 1], [false, true, false, false, true, true]) ([2, 1, 0.75, 3, -1.5], true) false true true\ntrue true false true true true false true true true true\n"
	if err != nil || output != want {
		t.Fatalf("output %q: %v", output, err)
	}
}

func TestPerfGeneratedVet(t *testing.T) {
	fixture, err := os.ReadFile("../examples/perf/main.gfx")
	if err != nil {
		t.Fatal(err)
	}
	source := qualityProgram(t, "perf.gfx", string(fixture))
	path := filepath.Join(t.TempDir(), "main.go")
	if err := os.WriteFile(path, source, 0600); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if output, err := exec.CommandContext(ctx, "go", "vet", path).CombinedOutput(); err != nil {
		t.Fatalf("generated Go vet: %v\n%s", err, output)
	}
}

func TestPerfLocalAliasParameters(t *testing.T) {
	source := qualityProgram(t, "local-alias.gfx", `package main
alias Number = Int
alias Identity[A] = A
func main() IO[Unit] {
 func increment(n Identity[Number]) Number { n + 1 }
 func loop(n Number, total Number) Number {
  if n == 0 { total } else { loop(n - 1, increment(total)) }
 }
 println(show(loop(1000000, 0)))
}`)
	if !bytes.Contains(source, []byte("func(p0 int64) int64")) {
		t.Fatal("local alias parameter was not unboxed")
	}
	output, err := runQualityGo(t, source, true)
	if err != nil || output != "1000000\n" {
		t.Fatalf("output %q: %v", output, err)
	}
}
