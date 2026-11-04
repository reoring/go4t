package rt

import (
	"context"
	"errors"
	"reflect"
	"testing"
)

func runMutMap(t *testing.T, action any) any {
	t.Helper()
	value, err := action.(IO)(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	return value
}

func TestMutMapDeferredSnapshotsAndFreshHandles(t *testing.T) {
	collectionShapes(t)
	compare := NativeCompare(collectionNativeOrd{1})
	create := MutMapNew(compare)
	m := runMutMap(t, create)
	other := runMutMap(t, create)
	if m == other {
		t.Fatal("repeated new returned the same handle")
	}
	insert := MutMapInsert(m, int64(2), "two")
	lookup := MutMapLookup(m, int64(2))
	if runMutMap(t, lookup) != (testNone{}) || runMutMap(t, MutMapSize(m)) != int64(0) {
		t.Fatal("constructing an IO performed an insertion")
	}
	runMutMap(t, insert)
	runMutMap(t, MutMapInsert(m, int64(1), "one"))
	runMutMap(t, MutMapInsert(m, int64(3), "three"))
	if runMutMap(t, lookup) != (testSome{"two"}) || runMutMap(t, MutMapSize(other)) != int64(0) {
		t.Fatal("deferred read or fresh handle isolation failed")
	}
	freeze := MutMapFreeze(m)
	list := runMutMap(t, MutMapToList(m))
	frozen := runMutMap(t, freeze)
	want := []any{collectionPair{int64(1), "one"}, collectionPair{int64(2), "two"}, collectionPair{int64(3), "three"}}
	if got := collectionValues(list); !reflect.DeepEqual(got, want) {
		t.Fatalf("ascending snapshot = %v", got)
	}
	runMutMap(t, MutMapInsert(m, int64(2), "replaced"))
	runMutMap(t, MutMapDelete(m, int64(1)))
	runMutMap(t, MutMapDelete(m, int64(99)))
	if runMutMap(t, MutMapSize(m)) != int64(2) || MapSize(frozen) != int64(3) || !reflect.DeepEqual(collectionValues(MapToList(frozen)), want) || !reflect.DeepEqual(collectionValues(list), want) {
		t.Fatal("mutable operations changed a snapshot or miscounted replacement/delete")
	}
	if MapSize(runMutMap(t, freeze)) != int64(2) {
		t.Fatal("freeze was observed at IO construction time")
	}
	thaw := MutMapThaw(frozen, compare)
	thawed := runMutMap(t, thaw)
	thawedAgain := runMutMap(t, thaw)
	runMutMap(t, MutMapDelete(thawed, int64(2)))
	if thawed == thawedAgain || runMutMap(t, MutMapSize(thawedAgain)) != int64(3) || MapSize(frozen) != int64(3) {
		t.Fatal("thaw shared mutable nodes")
	}
	if runMutMap(t, MutMapSize(runMutMap(t, MutMapThaw(EmptyMap(), compare)))) != int64(0) {
		t.Fatal("empty thaw was not empty")
	}
}

func TestMutMapNativeAndCustomOrdering(t *testing.T) {
	collectionShapes(t)
	for _, tt := range []struct {
		compare any
		keys    []any
	}{
		{NativeCompare(collectionNativeOrd{1}), []any{int64(-1 << 63), int64(0), int64(1<<63 - 1)}},
		{NativeCompare(collectionNativeOrd{2}), []any{"", "a", "é", "日"}},
		{NativeCompare(collectionReverseOrd{}), []any{int64(3), int64(2), int64(1)}},
	} {
		m := runMutMap(t, MutMapNew(tt.compare))
		for i := len(tt.keys) - 1; i >= 0; i-- {
			runMutMap(t, MutMapInsert(m, tt.keys[i], int64(i)))
		}
		for i, pair := range collectionValues(runMutMap(t, MutMapToList(m))) {
			if pair != (collectionPair{tt.keys[i], int64(i)}) || runMutMap(t, MutMapLookup(m, tt.keys[i])) != (testSome{int64(i)}) {
				t.Fatalf("wrong ordering/lookup for %v", tt.keys)
			}
		}
	}
	compare := func(a, b any) any { return collectionCompare(a.(int64)/10, b.(int64)/10) }
	m := runMutMap(t, MutMapNew(compare))
	runMutMap(t, MutMapInsert(m, int64(11), "old"))
	runMutMap(t, MutMapInsert(m, int64(19), "new"))
	if got := collectionValues(runMutMap(t, MutMapToList(m))); !reflect.DeepEqual(got, []any{collectionPair{int64(19), "new"}}) {
		t.Fatalf("equivalent replacement did not retain the new key: %v", got)
	}
	// Thaw orders by its Ord, not the explicit ordering of the input map.
	original := MapFromList(collectionList(collectionPair{int64(1), "a"}, collectionPair{int64(2), "b"}), NativeCompare(collectionReverseOrd{}))
	thawed := runMutMap(t, MutMapThaw(original, NativeCompare(collectionNativeOrd{1})))
	if got := collectionValues(MapKeys(runMutMap(t, MutMapFreeze(thawed)))); !reflect.DeepEqual(got, []any{int64(1), int64(2)}) {
		t.Fatalf("thaw ignored its comparator: %v", got)
	}
}

func checkMutMapTree(t *testing.T, n *mutMapNode, lower, upper int64) (int, int) {
	t.Helper()
	if n == nil {
		return 0, 0
	}
	key := n.key.(int64)
	if key <= lower || key >= upper {
		t.Fatalf("key %d outside (%d, %d)", key, lower, upper)
	}
	lh, ls := checkMutMapTree(t, n.left, lower, key)
	rh, rs := checkMutMapTree(t, n.right, key, upper)
	if lh > rh+1 || rh > lh+1 || n.height != 1+max(lh, rh) {
		t.Fatalf("unbalanced node %d: height %d children %d/%d", key, n.height, lh, rh)
	}
	return n.height, ls + rs + 1
}

func TestMutMapBalanceAndAllocationCounts(t *testing.T) {
	collectionShapes(t)
	m := runMutMap(t, MutMapNew(NativeCompare(collectionNativeOrd{1}))).(*MutMap)
	const count = 4096
	for i := int64(0); i < count; i++ {
		runMutMap(t, MutMapInsert(m, i, i*3))
	}
	frozen := runMutMap(t, MutMapFreeze(m))
	for i := int64(0); i < count; i++ {
		key := i * 73 % count
		if runMutMap(t, MutMapLookup(m, key)) != (testSome{key * 3}) {
			t.Fatalf("missing key %d", key)
		}
		runMutMap(t, MutMapDelete(m, key))
		if i%31 == 0 {
			_, size := checkMutMapTree(t, m.root, -1, count)
			if int64(size) != m.size || m.size != count-i-1 {
				t.Fatalf("incorrect size %d/%d", size, m.size)
			}
		}
	}
	if m.root != nil || MapSize(frozen) != int64(count) {
		t.Fatal("delete failed or mutated frozen tree")
	}
	ctx := context.Background()
	var key, value any = int64(1), int64(2)
	var root *mutMapNode
	if got := testing.AllocsPerRun(100, func() { root, _, _ = mutMapInsert(nil, key, value, m.compare, ctx) }); got != 1 {
		t.Fatalf("new key allocations = %v, want 1", got)
	}
	if got := testing.AllocsPerRun(100, func() { root, _, _ = mutMapInsert(root, key, value, m.compare, ctx) }); got != 0 {
		t.Fatalf("replacement allocations = %v, want 0", got)
	}
	if got := testing.AllocsPerRun(100, func() { _ = mutMapLookup(root, key, m.compare) }); got != 0 {
		t.Fatalf("tree lookup allocations = %v, want 0", got)
	}
}

func TestMutMapConcurrentFibers(t *testing.T) {
	collectionShapes(t)
	m := runMutMap(t, MutMapNew(NativeCompare(collectionNativeOrd{1})))
	gate, release := concTestGate(t)
	fibers := make([]any, 8)
	for i := range fibers {
		start := int64(i * 100)
		fibers[i] = runMutMap(t, Fork(IO(func(ctx context.Context) (any, error) {
			<-gate
			for key := start; key < start+100; key++ {
				if _, err := MutMapInsert(m, key, key).(IO)(ctx); err != nil {
					return nil, err
				}
				if key%2 == 0 {
					if _, err := MutMapDelete(m, key).(IO)(ctx); err != nil {
						return nil, err
					}
				}
			}
			return Unit{}, nil
		})))
	}
	release()
	for _, fiber := range fibers {
		result := receiveConcTest(t, runConcTest(Await(fiber)))
		if result.err != nil {
			t.Fatal(result.err)
		}
	}
	if runMutMap(t, MutMapSize(m)) != int64(400) {
		t.Fatal("concurrent operations lost updates")
	}
	for key := int64(0); key < 800; key++ {
		var want any = testNone{}
		if key%2 != 0 {
			want = testSome{key}
		}
		if got := runMutMap(t, MutMapLookup(m, key)); got != want {
			t.Fatalf("key %d = %v, want %v", key, got, want)
		}
	}
}

func TestMutMapCancellationAndComparatorPanic(t *testing.T) {
	collectionShapes(t)
	compare := NativeCompare(collectionNativeOrd{1})
	m := runMutMap(t, MutMapNew(compare))
	runMutMap(t, MutMapInsert(m, int64(1), "old"))
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	for _, action := range []any{MutMapNew(compare), MutMapInsert(m, int64(1), "new"), MutMapLookup(m, int64(1)), MutMapDelete(m, int64(1)), MutMapSize(m), MutMapToList(m), MutMapFreeze(m), MutMapThaw(EmptyMap(), compare)} {
		value, err := action.(IO)(ctx)
		var failure *Failure
		if value != nil || !errors.Is(err, context.Canceled) || !errors.As(err, &failure) || failure.Kind != Cancelled {
			t.Fatalf("cancelled action = (%v, %v)", value, err)
		}
	}
	if runMutMap(t, MutMapLookup(m, int64(1))) != (testSome{"old"}) {
		t.Fatal("cancelled write committed")
	}
	for _, remove := range []bool{false, true} {
		ctx, cancel := context.WithCancel(context.Background())
		shouldCancel, shouldPanic := false, false
		cmp := func(a, b any) any {
			if shouldCancel {
				cancel()
			}
			if shouldPanic {
				panic("comparator panic")
			}
			return collectionCompare(a, b)
		}
		m := runMutMap(t, MutMapNew(cmp))
		runMutMap(t, MutMapInsert(m, int64(1), "old"))
		action := MutMapInsert(m, int64(1), "new")
		if remove {
			action = MutMapDelete(m, int64(1))
		}
		shouldCancel = true
		if _, err := action.(IO)(ctx); !errors.Is(err, context.Canceled) {
			t.Fatalf("cancel during comparison = %v", err)
		}
		shouldCancel, shouldPanic = false, true
		func() {
			defer func() {
				if got := recover(); got != "comparator panic" {
					t.Fatalf("panic = %v", got)
				}
			}()
			runMutMap(t, action)
		}()
		shouldPanic = false
		if runMutMap(t, MutMapLookup(m, int64(1))) != (testSome{"old"}) {
			t.Fatal("cancelled or panicking comparator mutated the tree")
		}
		cancel()
	}
}
