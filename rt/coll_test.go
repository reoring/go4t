package rt

import (
	"reflect"
	"testing"
)

type collectionOrdering struct{ sign int }
type collectionPair struct{ key, value any }

func collectionShapes(t *testing.T) {
	t.Helper()
	installTestShapes(t)
	Shapes.OrderingOf = func(o any) int { return o.(collectionOrdering).sign }
	Shapes.Tuple2 = func(a, b any) any { return collectionPair{a, b} }
	Shapes.Untuple2 = func(p any) (any, any) { pair := p.(collectionPair); return pair.key, pair.value }
}

func collectionCompare(a, b any) any {
	x, y := a.(int64), b.(int64)
	if x < y {
		return collectionOrdering{-1}
	}
	if x > y {
		return collectionOrdering{1}
	}
	return collectionOrdering{0}
}

func collectionList(values ...any) any {
	xs := Shapes.Nil()
	for i := len(values) - 1; i >= 0; i-- {
		xs = Shapes.Cons(values[i], xs)
	}
	return xs
}

func collectionValues(xs any) []any {
	values := []any{}
	for {
		x, rest, ok := Shapes.Uncons(xs)
		if !ok {
			return values
		}
		values = append(values, x)
		xs = rest
	}
}

func TestMapOrderingAndPersistence(t *testing.T) {
	collectionShapes(t)
	empty := EmptyMap()
	m := empty
	for _, key := range []int64{4, 2, 6, 1, 3, 5, 7} {
		m = MapInsert(m, key, key*10, collectionCompare)
	}
	if MapSize(empty) != int64(0) || MapLookup(empty, int64(4), collectionCompare) != (testNone{}) {
		t.Fatal("insertion changed the empty map")
	}
	if got := collectionValues(MapKeys(m)); !reflect.DeepEqual(got, []any{int64(1), int64(2), int64(3), int64(4), int64(5), int64(6), int64(7)}) {
		t.Fatalf("keys = %v", got)
	}
	if got := MapLookup(m, int64(5), collectionCompare); got != (testSome{int64(50)}) {
		t.Fatalf("lookup = %#v", got)
	}
	updated := MapInsert(m, int64(4), int64(400), collectionCompare)
	if MapSize(updated) != int64(7) || MapLookup(m, int64(4), collectionCompare) != (testSome{int64(40)}) {
		t.Fatal("replacement changed size or original map")
	}
	deleted := MapDelete(updated, int64(4), collectionCompare)
	if MapMember(deleted, int64(4), collectionCompare) != false || MapSize(deleted) != int64(6) {
		t.Fatal("delete failed")
	}
	if MapLookup(updated, int64(4), collectionCompare) != (testSome{int64(400)}) {
		t.Fatal("delete mutated original map")
	}
	if got := MapDelete(deleted, int64(99), collectionCompare); got != deleted {
		t.Fatal("missing-key delete should return the same map")
	}
	for _, key := range []int64{1, 7, 3, 5, 2, 6} {
		deleted = MapDelete(deleted, key, collectionCompare)
	}
	if MapSize(deleted) != int64(0) || len(collectionValues(MapToList(deleted))) != 0 {
		t.Fatal("deleting all entries did not empty the map")
	}
}

func TestMapConversionsUnionMapAndFold(t *testing.T) {
	collectionShapes(t)
	m := MapFromList(collectionList(collectionPair{int64(3), "c"}, collectionPair{int64(1), "old"}, collectionPair{int64(2), "b"}, collectionPair{int64(1), "a"}), collectionCompare)
	want := []any{collectionPair{int64(1), "a"}, collectionPair{int64(2), "b"}, collectionPair{int64(3), "c"}}
	if got := collectionValues(MapToList(m)); !reflect.DeepEqual(got, want) {
		t.Fatalf("map round trip = %v", got)
	}
	if got := collectionValues(MapValues(m)); !reflect.DeepEqual(got, []any{"a", "b", "c"}) {
		t.Fatalf("values = %v", got)
	}
	other := MapFromList(collectionList(collectionPair{int64(2), "wrong"}, collectionPair{int64(4), "d"}), collectionCompare)
	union := MapUnion(m, other, collectionCompare)
	if MapLookup(union, int64(2), collectionCompare) != (testSome{"b"}) || MapLookup(union, int64(4), collectionCompare) != (testSome{"d"}) || MapSize(m) != int64(3) {
		t.Fatal("union must be left-biased and persistent")
	}
	mapped := MapMap(union, func(v any) any { return v.(string) + "!" })
	if got := MapFold(mapped, "", func(acc, key, value any) any { return acc.(string) + value.(string) }); got != "a!b!c!d!" {
		t.Fatalf("ascending fold = %v", got)
	}
	if MapLookup(union, int64(1), collectionCompare) != (testSome{"a"}) {
		t.Fatal("mapMap mutated its input")
	}
	reverseCompare := func(a, b any) any { return collectionCompare(b, a) }
	reversed := MapFromList(MapToList(m), reverseCompare)
	if got := collectionValues(MapKeys(reversed)); !reflect.DeepEqual(got, []any{int64(3), int64(2), int64(1)}) {
		t.Fatalf("custom comparator ignored: %v", got)
	}
}

func TestMapLogarithmicHundredThousand(t *testing.T) {
	collectionShapes(t)
	const count = 100000
	comparisons := 0
	cmp := func(a, b any) any { comparisons++; return collectionCompare(a, b) }
	m := EmptyMap()
	for i := int64(0); i < count; i++ {
		m = MapInsert(m, i, i, cmp)
	}
	// An unbalanced tree takes ~5 billion comparisons on this ascending input.
	// A generous deterministic bound catches quadratic growth without a wall clock.
	if comparisons > count*24 {
		t.Fatalf("100k inserts took %d comparisons, want at most %d", comparisons, count*24)
	}
	if MapSize(m) != int64(count) {
		t.Fatalf("size = %v", MapSize(m))
	}
	comparisons = 0
	for i := int64(0); i < count; i++ {
		if MapLookup(m, i, cmp) != (testSome{i}) {
			t.Fatalf("lookup %d failed", i)
		}
	}
	if comparisons > count*24 {
		t.Fatalf("100k lookups took %d comparisons", comparisons)
	}
	original := m
	comparisons = 0
	for i := int64(0); i < count; i++ {
		m = MapDelete(m, i, cmp)
	}
	if comparisons > count*24 || MapSize(m) != int64(0) || MapSize(original) != int64(count) {
		t.Fatalf("deletion: comparisons=%d size=%v old size=%v", comparisons, MapSize(m), MapSize(original))
	}
}

func TestSetOperations(t *testing.T) {
	collectionShapes(t)
	a := SetFromList(collectionList(int64(3), int64(1), int64(2), int64(1)), collectionCompare)
	b := SetFromList(collectionList(int64(2), int64(4)), collectionCompare)
	for _, tt := range []struct {
		name string
		got  any
		want []any
	}{
		{"sorted dedupe", a, []any{int64(1), int64(2), int64(3)}},
		{"union", SetUnion(a, b, collectionCompare), []any{int64(1), int64(2), int64(3), int64(4)}},
		{"intersection", SetIntersection(a, b, collectionCompare), []any{int64(2)}},
		{"difference", SetDifference(a, b, collectionCompare), []any{int64(1), int64(3)}},
		{"empty intersection", SetIntersection(a, EmptySet(), collectionCompare), []any{}},
		{"empty difference", SetDifference(a, a, collectionCompare), []any{}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if got := collectionValues(SetToList(tt.got)); !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("got %v, want %v", got, tt.want)
			}
		})
	}
	changed := SetDelete(SetInsert(a, int64(5), collectionCompare), int64(1), collectionCompare)
	if SetSize(a) != int64(3) || SetMember(a, int64(1), collectionCompare) != true || SetMember(a, int64(5), collectionCompare) != false || SetMember(changed, int64(5), collectionCompare) != true {
		t.Fatal("set update changed original or lost inserted key")
	}
	if SetInsert(a, int64(2), collectionCompare) != a || SetDelete(a, int64(9), collectionCompare) != a {
		t.Fatal("set no-op should retain root")
	}
}

func TestArrayBoundariesAndPersistence(t *testing.T) {
	collectionShapes(t)
	a := ArrayFromList(collectionList(int64(3), int64(1), int64(2)))
	for _, i := range []int64{-1, 3, 1 << 62} {
		if got := ArrayGet(a, i); got != (testNone{}) {
			t.Errorf("get(%d) = %#v", i, got)
		}
		if got := collectionValues(ArrayToList(ArraySet(a, i, int64(9)))); !reflect.DeepEqual(got, []any{int64(3), int64(1), int64(2)}) {
			t.Errorf("out-of-range set(%d) = %v", i, got)
		}
	}
	if ArrayLength(a) != int64(3) || ArrayGet(a, int64(1)) != (testSome{int64(1)}) {
		t.Fatal("length or indexing failed")
	}
	updated := ArraySet(a, int64(1), int64(9))
	pushed := ArrayPush(a, int64(4))
	sorted := ArraySortBy(a, collectionCompare)
	mapped := ArrayMap(a, func(v any) any { return v.(int64) * 2 })
	for _, tt := range []struct {
		name string
		got  any
		want []any
	}{
		{"original", a, []any{int64(3), int64(1), int64(2)}},
		{"set", updated, []any{int64(3), int64(9), int64(2)}},
		{"push", pushed, []any{int64(3), int64(1), int64(2), int64(4)}},
		{"sort", sorted, []any{int64(1), int64(2), int64(3)}},
		{"map", mapped, []any{int64(6), int64(2), int64(4)}},
		{"slice", ArraySlice(a, int64(1), int64(3)), []any{int64(1), int64(2)}},
		{"clamped slice", ArraySlice(a, int64(-9), int64(99)), []any{int64(3), int64(1), int64(2)}},
		{"reversed slice", ArraySlice(a, int64(2), int64(1)), []any{}},
		{"negative slice", ArraySlice(a, int64(-2), int64(-1)), []any{}},
		{"past end slice", ArraySlice(a, int64(4), int64(8)), []any{}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if got := collectionValues(ArrayToList(tt.got)); !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("got %v, want %v", got, tt.want)
			}
		})
	}
	if got := ArrayFold(a, int64(0), func(acc, v any) any { return acc.(int64)*10 + v.(int64) }); got != int64(312) {
		t.Fatalf("left fold = %v", got)
	}
	slice := ArraySlice(a, int64(0), int64(2))
	ArraySet(slice, int64(0), int64(8))
	ArrayPush(slice, int64(8))
	ArraySortBy(slice, collectionCompare)
	if got := collectionValues(ArrayToList(a)); !reflect.DeepEqual(got, []any{int64(3), int64(1), int64(2)}) {
		t.Fatal("writing or sorting a slice mutated shared storage")
	}
	empty := ArrayFromList(Shapes.Nil())
	if ArrayLength(empty) != int64(0) || ArrayGet(empty, int64(0)) != (testNone{}) || ArrayFold(empty, "initial", func(a, b any) any { t.Fatal("empty fold called callback"); return nil }) != "initial" {
		t.Fatal("empty array boundary failed")
	}
}

func TestArraySortStable(t *testing.T) {
	collectionShapes(t)
	a := ArrayFromList(collectionList(collectionPair{int64(2), "a"}, collectionPair{int64(1), "b"}, collectionPair{int64(2), "c"}))
	cmp := func(a, b any) any { return collectionCompare(a.(collectionPair).key, b.(collectionPair).key) }
	want := []any{collectionPair{int64(1), "b"}, collectionPair{int64(2), "a"}, collectionPair{int64(2), "c"}}
	if got := collectionValues(ArrayToList(ArraySortBy(a, cmp))); !reflect.DeepEqual(got, want) {
		t.Fatalf("stable sort = %v", got)
	}
}
