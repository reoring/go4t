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
func assertCollection(label String, ok Bool) IO[Unit] {
  if ok { ioPure(unit()) } else { ioFail(label) }
}
type Desc = Desc(Int) deriving Eq
instance Ord[Desc] {
  compare(a, b) { match (a, b) { (Desc(x), Desc(y)) => compare(y, x) } }
}
func main() IO[Unit] {
  let original = mapFromList([("b", 2), ("a", 1), ("b", 3)])
  let changed = mapAlter(original, "a", func(o) => Some(fromOption(o, 0) + 10))
  let added = mapAlter(original, "c", func(o) => Some(fromOption(o, 0) + 4))
  let deleted = mapAlter(original, "b", func(o) => None)
  let grouped = groupBy([("bob", 1), ("ada", 2), ("bob", 3)], func(p) => p.0)
  let a = arrayFromList([3, 1, 2])
  let emptyM: Map[Int, Int] = emptyMap()
  let emptyS: Set[Int] = emptySet()
  let emptyA: Array[Int] = arrayFromList([])
  let s = setFromList([3, 1, 3, 2])
  let other = setFromList([2, 4])
  do {
    assertCollection("map show/order", show(original) == "{a: 1, b: 3}")
    assertCollection("map alter existing", mapLookup(changed, "a") == Some(11))
    assertCollection("map alter insert", mapLookup(added, "c") == Some(4))
    assertCollection("map alter delete", mapToList(deleted) == [("a", 1)])
    assertCollection("map persistence", mapLookup(original, "a") == Some(1))
    assertCollection("map adjust missing", mapAdjust(original, "absent", func(v) => v / 0) == original)
    assertCollection("map adjust existing", mapLookup(mapAdjust(original, "a", func(v) => v + 1), "a") == Some(2))
    assertCollection("map alter absent None", mapAlter(original, "absent", func(o) => None) == original)
    assertCollection("map union bias", mapUnion(original, mapFromList([("a", 9), ("c", 4)])) == mapFromList([("a", 1), ("b", 3), ("c", 4)]))
    assertCollection("map traversal", mapKeys(original) == ["a", "b"] && mapValues(original) == [1, 3])
    assertCollection("map fold order", mapFold(original, "", func(acc, k, v) => acc + k + show(v)) == "a1b3")
    assertCollection("map functor", map(original, func(v) => show(v)) == mapFromList([("a", "1"), ("b", "3")]))
    assertCollection("map equality false", original != changed)
    assertCollection("group order", mapLookup(grouped, "bob") == Some([("bob", 1), ("bob", 3)]))
    assertCollection("empty group", mapSize(groupBy([], func(x Int) => x)) == 0)
    assertCollection("custom Ord", mapValues(mapFromList([(Desc(1), "one"), (Desc(2), "two")])) == ["two", "one"])
    assertCollection("map empty", show(emptyM) == "{}" && mapSize(emptyM) == 0 && mapLookup(emptyM, 0) == None)
    assertCollection("set display/dedupe", show(s) == "#{1, 2, 3}" && setSize(s) == 3)
    assertCollection("set union", setToList(setUnion(s, other)) == [1, 2, 3, 4])
    assertCollection("set intersection", setIntersection(s, other) == setFromList([2]))
    assertCollection("set difference", setDifference(s, other) == setFromList([1, 3]))
    assertCollection("set update", setMember(setInsert(emptyS, 1), 1) && !setMember(setDelete(s, 1), 1))
    assertCollection("set persistence", setMember(s, 1) && setSize(emptyS) == 0)
    assertCollection("set empty", show(emptyS) == "#{}" && !setMember(emptyS, 1) && emptyS != s)
    assertCollection("array display", show(a) == "#[3, 1, 2]")
    assertCollection("array random access", arrayGet(a, 1) == Some(1) && arrayGet(a, -1) == None && arrayGet(a, 3) == None)
    assertCollection("array update", arraySet(a, 1, 9) == arrayFromList([3, 9, 2]) && arraySet(a, 3, 9) == a)
    assertCollection("array functor", map(a, func(v) => v * 2) == arrayFromList([6, 2, 4]))
    assertCollection("array push/slice", arrayToList(arraySlice(arrayPush(a, 4), 1, 4)) == [1, 2, 4])
    assertCollection("array slice clamp", arraySlice(a, -8, 99) == a && arrayLength(arraySlice(a, 2, 1)) == 0)
    assertCollection("array fold", arrayFold(a, 0, func(acc, v) => acc * 10 + v) == 312)
    assertCollection("array sort", arraySortBy(a, func(x, y) => compare(x, y)) == arrayFromList([1, 2, 3]))
    assertCollection("array persistence", arrayToList(a) == [3, 1, 2] && a != arraySet(a, 0, 0))
    assertCollection("array empty", show(emptyA) == "#[]" && arrayLength(emptyA) == 0)
  }
}`
	p, err := parser.Parse(prelude.Source)
	if err != nil {
		t.Fatal(err)
	}
	u, err := parser.Parse(source)
	if err != nil {
		t.Fatal(err)
	}
	program, err := types.Check([]*ast.File{p, u})
	if err != nil {
		t.Fatal(err)
	}
	generated, err := Generate(program, "main")
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
