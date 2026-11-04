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

func TestMapEmptyIdentityAndNoopUpdates(t *testing.T) {
	collectionShapes(t)
	empty := EmptyMap()
	identity := func(v any) any { return v }
	for name, value := range map[string]any{
		"constructor": EmptyMap(),
		"from list":   MapFromList(collectionList(), collectionCompare),
		"delete":      MapDelete(empty, int64(1), collectionCompare),
		"union":       MapUnion(empty, empty, collectionCompare),
		"map":         MapMap(empty, identity),
		"foreign":     ForeignMap(nil),
		"freeze":      runMutMap(t, MutMapFreeze(runMutMap(t, MutMapNew(collectionCompare)))),
		"last delete": MapDelete(MapInsert(empty, int64(1), "one", collectionCompare), int64(1), collectionCompare),
	} {
		if value != empty {
			t.Errorf("%s did not preserve empty-map identity", name)
		}
	}
	m := MapInsert(empty, int64(1), "one", collectionCompare)
	if MapDelete(m, int64(2), collectionCompare) != m {
		t.Fatal("missing-key delete changed identity")
	}
	if MapUnion(m, empty, collectionCompare) != m || MapUnion(m, m, collectionCompare) != m {
		t.Fatal("unchanged left-biased union changed identity")
	}
	var zero mapValue
	if MapSize(&zero) != int64(0) || MapLookup(&zero, int64(1), collectionCompare) != (testNone{}) {
		t.Fatal("zero map payload is not empty")
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

type collectionNativeOrd struct{ kind int }

func (d collectionNativeOrd) Go4tNativeOrd() int { return d.kind }
func (d collectionNativeOrd) Compare(a, b any) any {
	panic("native comparator called dictionary")
}

type collectionReverseOrd struct{}

func (collectionReverseOrd) Compare(a, b any) any { return collectionCompare(b, a) }

func TestMapNativeOrdWitness(t *testing.T) {
	collectionShapes(t)
	for _, tt := range []struct {
		name string
		kind int
		keys []any
	}{
		{"Int", 1, []any{int64(-1 << 63), int64(-1), int64(0), int64(1), int64(1<<63 - 1)}},
		{"String", 2, []any{"", "\x00", "a", "a\x00", "z", "é", "日", "🙂"}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			dict := collectionNativeOrd{tt.kind}
			compare := NativeCompare(dict)
			if NativeOrdKind(dict) != tt.kind || NativeOrdKind(compare) != tt.kind {
				t.Fatal("native witness was lost")
			}
			m := EmptyMap()
			for i := len(tt.keys) - 1; i >= 0; i-- {
				m = MapInsert(m, tt.keys[i], int64(i), compare)
			}
			if got := collectionValues(MapKeys(m)); !reflect.DeepEqual(got, tt.keys) {
				t.Fatalf("native ordering = %v, want %v", got, tt.keys)
			}
			for i, key := range tt.keys {
				if got := MapLookup(m, key, compare); got != (testSome{int64(i)}) {
					t.Fatalf("lookup %v = %v", key, got)
				}
			}
			replaced := MapInsert(m, tt.keys[0], "new", compare)
			deleted := MapDelete(replaced, tt.keys[0], compare)
			if MapLookup(m, tt.keys[0], compare) != (testSome{int64(0)}) ||
				MapLookup(replaced, tt.keys[0], compare) != (testSome{"new"}) ||
				MapMember(deleted, tt.keys[0], compare) != false {
				t.Fatal("native updates did not preserve snapshots")
			}
			roundTrip := MapFromList(MapToList(m), compare)
			if got := collectionValues(MapToList(roundTrip)); !reflect.DeepEqual(got, collectionValues(MapToList(m))) {
				t.Fatal("native list round trip changed entries")
			}
			union := MapUnion(m, replaced, compare)
			if MapLookup(union, tt.keys[0], compare) != (testSome{int64(0)}) {
				t.Fatal("native union lost left bias")
			}
		})
	}
	custom := NativeCompare(collectionReverseOrd{})
	if NativeOrdKind(custom) != 0 || NativeOrdKind(collectionCompare) != 0 {
		t.Fatal("arbitrary comparator received a native witness")
	}
	m := MapFromList(collectionList(collectionPair{int64(1), "a"}, collectionPair{int64(3), "c"}, collectionPair{int64(2), "b"}), custom)
	if got := collectionValues(MapKeys(m)); !reflect.DeepEqual(got, []any{int64(3), int64(2), int64(1)}) {
		t.Fatalf("unmarked dictionary was bypassed: %v", got)
	}
}

func TestMapBranchedSnapshots(t *testing.T) {
	collectionShapes(t)
	compare := NativeCompare(collectionNativeOrd{1})
	snapshots := []any{EmptyMap()}
	expected := []map[int64]int64{{}}
	for i := 1; i <= 512; i++ {
		parent := i * 17 % len(snapshots)
		want := make(map[int64]int64, len(expected[parent])+1)
		for key, value := range expected[parent] {
			want[key] = value
		}
		key := int64(i * 37 % 101)
		m := snapshots[parent]
		if i%4 == 0 {
			m = MapDelete(m, key, compare)
			delete(want, key)
		} else {
			m = MapInsert(m, key, int64(i), compare)
			want[key] = int64(i)
		}
		snapshots = append(snapshots, m)
		expected = append(expected, want)
	}
	for i, m := range snapshots {
		want := expected[i]
		checkCollectionTree(t, m.(*mapValue).root, -1, 101)
		if MapSize(m) != int64(len(want)) {
			t.Fatalf("snapshot %d size = %v, want %d", i, MapSize(m), len(want))
		}
		for key := int64(0); key < 101; key++ {
			value, present := want[key]
			var option any = testNone{}
			if present {
				option = testSome{value}
			}
			if got := MapLookup(m, key, compare); got != option {
				t.Fatalf("snapshot %d key %d = %v, want %v", i, key, got, option)
			}
		}
	}
}

func TestMapReplacementRetainsNewEquivalentKey(t *testing.T) {
	collectionShapes(t)
	compare := func(a, b any) any {
		return collectionCompare(a.(int64)/10, b.(int64)/10)
	}
	original := MapInsert(EmptyMap(), int64(11), "old", compare)
	replaced := MapInsert(original, int64(19), "new", compare)
	if got := collectionValues(MapToList(original)); !reflect.DeepEqual(got, []any{collectionPair{int64(11), "old"}}) {
		t.Fatalf("original entries = %v", got)
	}
	if got := collectionValues(MapToList(replaced)); !reflect.DeepEqual(got, []any{collectionPair{int64(19), "new"}}) {
		t.Fatalf("replacement entries = %v", got)
	}
}
