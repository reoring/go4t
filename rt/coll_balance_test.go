package rt

import (
	"math/rand/v2"
	"testing"
)

func TestMapMixedUpdatesRemainBalanced(t *testing.T) {
	collectionShapes(t)
	rng := rand.New(rand.NewPCG(42, 17))
	m := EmptyMap()
	const count = 2048
	for _, key := range rng.Perm(count) {
		m = MapInsert(m, int64(key), int64(key), collectionCompare)
	}
	original := m
	for i, key := range rng.Perm(count) {
		m = MapDelete(m, int64(key), collectionCompare)
		if MapSize(m) != int64(count-i-1) {
			t.Fatalf("size after deleting %d = %v", key, MapSize(m))
		}
		checkCollectionTree(t, m.(*mapValue).root, -1, count)
	}
	for key := int64(0); key < count; key++ {
		if MapLookup(original, key, collectionCompare) != (testSome{key}) {
			t.Fatalf("old snapshot lost key %d", key)
		}
	}
}

func checkCollectionTree(t *testing.T, node *collNode, lower, upper int64) (height, size int) {
	t.Helper()
	if node == nil {
		return 0, 0
	}
	key := node.key.(int64)
	if key <= lower || key >= upper {
		t.Fatalf("key %d outside (%d, %d)", key, lower, upper)
	}
	lh, ls := checkCollectionTree(t, node.left, lower, key)
	rh, rs := checkCollectionTree(t, node.right, key, upper)
	height, size = 1+max(lh, rh), 1+ls+rs
	if lh-rh > 1 || rh-lh > 1 || node.height != height || node.size != size {
		t.Fatalf("invalid balance/cache at %d: heights=(%d,%d), cached=(%d,%d), want=(%d,%d)", key, lh, rh, node.height, node.size, height, size)
	}
	return height, size
}

func TestCollectionMissingShapes(t *testing.T) {
	collectionShapes(t)
	for _, tt := range []struct {
		name, want string
		call       func()
	}{
		{"tuple", "rt.Shapes is not initialized: Tuple2 is required", func() { Shapes.Tuple2 = nil; Tuple2Of(1, 2) }},
		{"ordering", "rt.Shapes is not initialized: OrderingOf is required", func() {
			Shapes.OrderingOf = nil
			m := MapInsert(EmptyMap(), int64(1), int64(1), collectionCompare)
			MapInsert(m, int64(2), int64(2), collectionCompare)
		}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			defer func() {
				if got := recover(); got != tt.want {
					t.Errorf("panic = %v, want %q", got, tt.want)
				}
			}()
			tt.call()
		})
	}
}
