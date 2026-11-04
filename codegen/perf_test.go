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
	source := qualityProgram(t, "scalar.gt", `package main
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
	for _, fragment := range []string{"func addInts_(a_ int64, b_ int64) int64", "var result_ int64 = (a_ + b_)", "func scale_(x_ float64, yes_ bool) float64", "for {", "//line scalar.gt:"} {
		if !bytes.Contains(source, []byte(fragment)) {
			t.Errorf("missing %q", fragment)
		}
	}
	output, err := runQualityGo(t, source, false)
	if err != nil || output != "42 3.0 500000500000 [2, 3]\n" {
		t.Fatalf("output %q: %v", output, err)
	}
}

func TestPerfSpecialization(t *testing.T) {
	source := qualityProgram(t, "special.gt", `package main
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
	if err != nil || output != "2 2.0 4\n" {
		t.Fatalf("output %q: %v", output, err)
	}
}

func TestPerfNumericRuntimeSemantics(t *testing.T) {
	source := qualityProgram(t, "numbers.gt", `package main
import "std/float"
func main() IO[Unit] {
 println("${9223372036854775807 + 1} ${1.0 / 0.0} ${float.isNaN(0.0 / 0.0)} ${show(-0.0)} ${float.nan() < 1.0} ${float.nan() == float.nan()}")
}`)
	output, err := runQualityGo(t, source, false)
	if err != nil || output != "-9223372036854775808 +Inf true -0.0 true false\n" {
		t.Fatalf("output %q: %v", output, err)
	}
}

func TestPerfOperators(t *testing.T) {
	source := qualityProgram(t, "operators.gt", `package main
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
	want := "([9, 5, 14, 3, 1], [false, true, false, false, true, true]) ([2.0, 1.0, 0.75, 3.0, -1.5], true) false true true\ntrue true false true true true false true true true true\n"
	if err != nil || output != want {
		t.Fatalf("output %q: %v", output, err)
	}
}

func TestPerfGeneratedVet(t *testing.T) {
	fixture, err := os.ReadFile("../examples/perf/main.gt")
	if err != nil {
		t.Fatal(err)
	}
	source := qualityProgram(t, "perf.gt", string(fixture))
	vetQualityGo(t, source)
}

func vetQualityGo(t *testing.T, source []byte) {
	t.Helper()
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
	source := qualityProgram(t, "local-alias.gt", `package main
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

func TestPerfNativeSortingPreservesOrdering(t *testing.T) {
	source := qualityProgram(t, "native-sort.gt", `package main
import "std/array"
import "std/list"
type Backwards = Backwards(Int) deriving Eq, Show
instance Ord[Backwards] {
 compare(a, b) { match (a, b) { (Backwards(x), Backwards(y)) => compare(y, x) } }
}
func ordered[A Ord](xs List[A]) List[A] { list.sort(xs) }
func main() IO[Unit] { do {
 println("${list.sort([3, 1, 2, 1])} ${list.sortBy([3, 1, 2], func(a, b) => compare(a, b))}")
 let words = ["z", "a", "a"]
 let keyed = [(2, "x"), (1, "a"), (1, "b")]
 println("${list.sort(words)} ${list.sortBy([1, 3, 2], func(a, b) => compare(b, a))}")
 println("${list.sortOn(keyed, func(p) => p.0)}")
 println("${list.sortOn(keyed, func(p) => Backwards(p.0))}")
 println("${ordered([Backwards(1), Backwards(3), Backwards(2)])}")
 let original = array.arrayFromList([3, 1, 2])
 println("${array.arraySortBy(original, compare)} ${original} ${list.sortBy(Nil, func(a Int, b Int) => compare(a, b))}")
} }`)
	for _, fragment := range []string{"Go4tNativeOrd() int", "rt.NativeCompare(", "rt.ListSortOn("} {
		if !bytes.Contains(source, []byte(fragment)) {
			t.Errorf("missing native ordering path %q", fragment)
		}
	}
	output, err := runQualityGo(t, source, false)
	want := "[1, 1, 2, 3] [1, 2, 3]\n[a, a, z] [3, 2, 1]\n[(1, a), (1, b), (2, x)]\n[(2, x), (1, a), (1, b)]\n[3, 2, 1]\n#[1, 2, 3] #[3, 1, 2] []\n"
	if err != nil || output != want {
		t.Fatalf("output %q, want %q: %v", output, want, err)
	}
}

func TestPerfNativeFoldsAndRecordPayloads(t *testing.T) {
	source := qualityProgram(t, "native-fold.gt", `package main
import "std/array"
import "std/float"
import "std/list"
type Body = Body{n: Int, x: Float, yes: Bool} deriving Eq
func step(a Body, b Body) Body { a{n: a.n + b.n, x: a.x + b.x, yes: a.yes && b.yes} }
func advanceCount(a Body, b Body) Body { a{n: a.n + 1, x: float.toFloat(a.n)} }
func replace(a Body, b Body) Body { b{x: a.x} }
func first[A](a A, b A) A { a }
func add(a Int, b Int) Int { a + b }
func main() IO[Unit] { do {
 let create = Body
 let original = create(1, -0.0, true)
 let bodies = array.arrayFromList([Body{n: 2, x: 1.5, yes: true}, Body{n: 3, x: 2.5, yes: false}])
 let result = array.arrayFold(bodies, original, step)
 println("${result.n} ${result.x} ${result.yes} ${original.n} ${float.showFloat(original.x)}")
 println("${array.arrayFold(bodies, original, first) == original}")
 println("${list.fold([1, 2, 3], 0, add)} ${list.fold([1], 9223372036854775807, func(a, b) => a + b)}")
 let f = add
 println("${list.fold([1, 2], 0, f)} ${list.length([true, false])} ${list.sum([1, 2, 3])}")
 println("${array.arrayFold(array.arrayFromList([1.5, 2.5]), 0.0, func(a, b) => a + b)} ${list.fold(Nil, 7, add)}")
 println("${float.isNaN(array.arrayFold(array.arrayFromList([float.nan()]), 0.0, func(a, b) => a + b))}")
 let advanced = list.fold(array.arrayToList(bodies), original, advanceCount)
 let replaced = array.arrayFold(bodies, original, replace)
 println("${advanced.n} ${advanced.x} ${replaced.n} ${float.showFloat(replaced.x)} ${original.n}")
} }`)
	for _, fragment := range []string{"rt.ListFoldScalar(", "rt.ArrayFoldScalar(", "rt.ListFoldRecord(", "rt.ArrayFoldRecord(", "func step__scalar", "F0 int64", "F1 float64", "F2 bool"} {
		if !bytes.Contains(source, []byte(fragment)) {
			t.Errorf("missing native scalar path %q", fragment)
		}
	}
	output, err := runQualityGo(t, source, false)
	want := "6 4.0 false 1 -0.0\ntrue\n6 -9223372036854775808\n3 2 6\n4.0 7\ntrue\n3 2.0 3 -0.0 1\n"
	if err != nil || output != want {
		t.Fatalf("output %q, want %q: %v", output, want, err)
	}
}

func TestPerfNativeFoldErasedNilElement(t *testing.T) {
	output, err := runQualityGo(t, []byte(`package main
import ("fmt"; "github.com/reoring/go4t/rt")
func main() {
 rt.Shapes.Uncons = func(xs any) (any, any, bool) { return nil, nil, xs != nil }
 xs := rt.ArrayPush(rt.ArrayFromList(nil), nil)
 count := func(n int64, value any) int64 { if value != nil { panic("changed element") }; return n + 1 }
 fmt.Println(rt.ArrayFoldScalar(xs, int64(0), count), rt.ListFoldScalar(true, int64(0), count))
}`), false)
	if err != nil || output != "1 1\n" {
		t.Fatalf("nil erased elements: %q: %v", output, err)
	}
}
