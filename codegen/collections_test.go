package codegen

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"gofx/ast"
	"gofx/modules"
	"gofx/parser"
	"gofx/types"
)

func TestCollectionShapes(t *testing.T) {
	p, _, _ := comparisonFixture()
	list(p)
	option(p)
	out := assertSource(t, p,
		"func rt_tuple2(a, b any) any",
		"return &Tuple2{F0: a, F1: b}",
		"func rt_untuple2(p any) (any, any)",
		"func rt_orderingOf(o any) int",
		"case *Ordering_LT:\n\t\treturn -1",
		"case *Ordering_EQ:\n\t\treturn 0",
		"case *Ordering_GT:\n\t\treturn 1",
		`panic("rt.Shapes.OrderingOf: invalid Ordering")`)
	for _, binding := range []string{"Tuple2: rt_tuple2", "Untuple2: rt_untuple2", "OrderingOf: rt_orderingOf"} {
		if !strings.Contains(strings.Join(strings.Fields(out), " "), binding) {
			t.Errorf("shape binding missing: %s", binding)
		}
	}
}

func TestCollectionsPreludeRuntime(t *testing.T) {
	const source = `package main
import collections "std/map"
import "std/set"
import "std/array"
import "std/result"
func assertCollection(label String, ok Bool) IO[Unit] {
  if ok { ioPure(unit()) } else { ioFail(label) }
}
type Desc = Desc(Int) deriving Eq
instance Ord[Desc] {
  compare(a, b) { match (a, b) { (Desc(x), Desc(y)) => compare(y, x) } }
}
func main() IO[Unit] {
  let original = collections.mapFromList([("b", 2), ("a", 1), ("b", 3)])
  let changed = collections.mapAlter(original, "a", func(o) => Some(result.fromOption(o, 0) + 10))
  let added = collections.mapAlter(original, "c", func(o) => Some(result.fromOption(o, 0) + 4))
  let deleted = collections.mapAlter(original, "b", func(o) => None)
  let grouped = collections.groupBy([("bob", 1), ("ada", 2), ("bob", 3)], func(p) => p.0)
  let a = array.arrayFromList([3, 1, 2])
  let emptyM: Map[Int, Int] = collections.emptyMap()
  let emptyS: Set[Int] = set.emptySet()
  let emptyA: Array[Int] = array.arrayFromList([])
  let s = set.setFromList([3, 1, 3, 2])
  let other = set.setFromList([2, 4])
  do {
    assertCollection("map show/order", show(original) == "{a: 1, b: 3}")
    assertCollection("map alter existing", collections.mapLookup(changed, "a") == Some(11))
    assertCollection("map alter insert", collections.mapLookup(added, "c") == Some(4))
    assertCollection("map alter delete", collections.mapToList(deleted) == [("a", 1)])
    assertCollection("map persistence", collections.mapLookup(original, "a") == Some(1))
    assertCollection("map adjust missing", collections.mapAdjust(original, "absent", func(v) => v / 0) == original)
    assertCollection("map adjust existing", collections.mapLookup(collections.mapAdjust(original, "a", func(v) => v + 1), "a") == Some(2))
    assertCollection("map alter absent None", collections.mapAlter(original, "absent", func(o) => None) == original)
    assertCollection("map union bias", collections.mapUnion(original, collections.mapFromList([("a", 9), ("c", 4)])) == collections.mapFromList([("a", 1), ("b", 3), ("c", 4)]))
    assertCollection("map traversal", collections.mapKeys(original) == ["a", "b"] && collections.mapValues(original) == [1, 3])
    assertCollection("map fold order", collections.mapFold(original, "", func(acc, k, v) => acc + k + show(v)) == "a1b3")
    assertCollection("map functor", map(original, func(v) => show(v)) == collections.mapFromList([("a", "1"), ("b", "3")]))
    assertCollection("map equality false", original != changed)
    assertCollection("group order", collections.mapLookup(grouped, "bob") == Some([("bob", 1), ("bob", 3)]))
    assertCollection("empty group", collections.mapSize(collections.groupBy([], func(x Int) => x)) == 0)
    assertCollection("custom Ord", collections.mapValues(collections.mapFromList([(Desc(1), "one"), (Desc(2), "two")])) == ["two", "one"])
    assertCollection("map empty", show(emptyM) == "{}" && collections.mapSize(emptyM) == 0 && collections.mapLookup(emptyM, 0) == None)
    assertCollection("set display/dedupe", show(s) == "#{1, 2, 3}" && set.setSize(s) == 3)
    assertCollection("set union", set.setToList(set.setUnion(s, other)) == [1, 2, 3, 4])
    assertCollection("set intersection", set.setIntersection(s, other) == set.setFromList([2]))
    assertCollection("set difference", set.setDifference(s, other) == set.setFromList([1, 3]))
    assertCollection("set update", set.setMember(set.setInsert(emptyS, 1), 1) && !set.setMember(set.setDelete(s, 1), 1))
    assertCollection("set persistence", set.setMember(s, 1) && set.setSize(emptyS) == 0)
    assertCollection("set empty", show(emptyS) == "#{}" && !set.setMember(emptyS, 1) && emptyS != s)
    assertCollection("array display", show(a) == "#[3, 1, 2]")
    assertCollection("array random access", array.arrayGet(a, 1) == Some(1) && array.arrayGet(a, -1) == None && array.arrayGet(a, 3) == None)
    assertCollection("array update", array.arraySet(a, 1, 9) == array.arrayFromList([3, 9, 2]) && array.arraySet(a, 3, 9) == a)
    assertCollection("array functor", map(a, func(v) => v * 2) == array.arrayFromList([6, 2, 4]))
    assertCollection("array push/slice", array.arrayToList(array.arraySlice(array.arrayPush(a, 4), 1, 4)) == [1, 2, 4])
    assertCollection("array slice clamp", array.arraySlice(a, -8, 99) == a && array.arrayLength(array.arraySlice(a, 2, 1)) == 0)
    assertCollection("array fold", array.arrayFold(a, 0, func(acc, v) => acc * 10 + v) == 312)
    assertCollection("array sort", array.arraySortBy(a, func(x, y) => compare(x, y)) == array.arrayFromList([1, 2, 3]))
    assertCollection("array persistence", array.arrayToList(a) == [3, 1, 2] && a != array.arraySet(a, 0, 0))
    assertCollection("array empty", show(emptyA) == "#[]" && array.arrayLength(emptyA) == 0)
  }
}`
	u, err := parser.Parse(source)
	if err != nil {
		t.Fatal(err)
	}
	modules, err := modules.LoadFiles([]*ast.File{u}, ".")
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
	if err := os.WriteFile(path, generated, 0600); err != nil {
		t.Fatal(err)
	}
	output, err := exec.Command("go", "run", path).CombinedOutput()
	if err != nil {
		t.Fatalf("generated collections program: %v\n%s", err, output)
	}
	if len(output) != 0 {
		t.Fatalf("unexpected output: %s", output)
	}
}
