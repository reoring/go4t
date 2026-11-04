package rt

import (
	"context"
	"sync"
)

// MutMap is an opaque ordered handle. It must not be copied after first use.
// The creation-time comparator is immutable; the mutex protects every tree
// observation, including snapshots. Individual operations, not sequences of
// lookup and insert, are atomic.
type MutMap struct {
	mu      sync.Mutex
	root    *mutMapNode
	compare collCompare
	size    int64
}

type mutMapNode struct {
	key, value  any
	left, right *mutMapNode
	height      int
}

func (m *MutMap) lock(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return ClassifyError(err)
	}
	m.mu.Lock()
	if err := ctx.Err(); err != nil {
		m.mu.Unlock()
		return ClassifyError(err)
	}
	return nil
}

// MutMapNew allocates a fresh handle each time its IO is executed.
func MutMapNew(compare any) any {
	order := collComparator(compare)
	return IO(func(ctx context.Context) (any, error) {
		if err := ctx.Err(); err != nil {
			return nil, ClassifyError(err)
		}
		return &MutMap{compare: order}, nil
	})
}

func MutMapInsert(m, key, value any) any {
	return IO(func(ctx context.Context) (any, error) {
		m := m.(*MutMap)
		if err := m.lock(ctx); err != nil {
			return nil, err
		}
		defer m.mu.Unlock()
		root, added, err := mutMapInsert(m.root, key, value, m.compare, ctx)
		if err != nil {
			return nil, ClassifyError(err)
		}
		m.root = root
		if added {
			m.size++
		}
		return Unit{}, nil
	})
}

func MutMapLookup(m, key any) any {
	return IO(func(ctx context.Context) (any, error) {
		m := m.(*MutMap)
		if err := m.lock(ctx); err != nil {
			return nil, err
		}
		defer m.mu.Unlock()
		if n := mutMapLookup(m.root, key, m.compare); n != nil {
			return SomeOf(n.value), nil
		}
		return NoneOf(), nil
	})
}

func MutMapDelete(m, key any) any {
	return IO(func(ctx context.Context) (any, error) {
		m := m.(*MutMap)
		if err := m.lock(ctx); err != nil {
			return nil, err
		}
		defer m.mu.Unlock()
		root, removed, err := mutMapDelete(m.root, key, m.compare, ctx)
		if err != nil {
			return nil, ClassifyError(err)
		}
		m.root = root
		if removed {
			m.size--
		}
		return Unit{}, nil
	})
}

func MutMapSize(m any) any {
	return IO(func(ctx context.Context) (any, error) {
		m := m.(*MutMap)
		if err := m.lock(ctx); err != nil {
			return nil, err
		}
		defer m.mu.Unlock()
		return m.size, nil
	})
}

func MutMapToList(m any) any {
	return IO(func(ctx context.Context) (any, error) {
		m := m.(*MutMap)
		if err := m.lock(ctx); err != nil {
			return nil, err
		}
		defer m.mu.Unlock()
		return mutMapList(m.root, Shapes.Nil()), nil
	})
}

// Freeze copies nodes in O(n); immutable keys and values may safely be shared.
func MutMapFreeze(m any) any {
	return IO(func(ctx context.Context) (any, error) {
		m := m.(*MutMap)
		if err := m.lock(ctx); err != nil {
			return nil, err
		}
		defer m.mu.Unlock()
		return mapValue{root: mutMapFreeze(m.root)}, nil
	})
}

func MutMapThaw(value, compare any) any {
	order := collComparator(compare)
	return IO(func(ctx context.Context) (any, error) {
		if err := ctx.Err(); err != nil {
			return nil, ClassifyError(err)
		}
		m := &MutMap{compare: order}
		var failure error
		// Reinsert using the supplied Ord even if the source was constructed
		// with an explicit mapFromListBy comparator of a different order.
		collWalk(value.(mapValue).root, func(n *collNode) {
			if failure != nil {
				return
			}
			var added bool
			m.root, added, failure = mutMapInsert(m.root, n.key, n.value, order, ctx)
			if added {
				m.size++
			}
		})
		if failure != nil {
			return nil, ClassifyError(failure)
		}
		return m, nil
	})
}

func mutMapHeight(n *mutMapNode) int {
	if n == nil {
		return 0
	}
	return n.height
}

func mutMapRefresh(n *mutMapNode) {
	n.height = 1 + max(mutMapHeight(n.left), mutMapHeight(n.right))
}

func mutMapRotateLeft(n *mutMapNode) *mutMapNode {
	top := n.right
	n.right, top.left = top.left, n
	mutMapRefresh(n)
	mutMapRefresh(top)
	return top
}

func mutMapRotateRight(n *mutMapNode) *mutMapNode {
	top := n.left
	n.left, top.right = top.right, n
	mutMapRefresh(n)
	mutMapRefresh(top)
	return top
}

func mutMapBalance(n *mutMapNode) *mutMapNode {
	mutMapRefresh(n)
	if mutMapHeight(n.left) > mutMapHeight(n.right)+1 {
		if mutMapHeight(n.left.left) < mutMapHeight(n.left.right) {
			n.left = mutMapRotateLeft(n.left)
		}
		return mutMapRotateRight(n)
	}
	if mutMapHeight(n.right) > mutMapHeight(n.left)+1 {
		if mutMapHeight(n.right.right) < mutMapHeight(n.right.left) {
			n.right = mutMapRotateRight(n.right)
		}
		return mutMapRotateLeft(n)
	}
	return n
}

// Searching allocates nothing. Language-level IO and Some wrappers have their
// usual costs; they are not tree allocations.
func mutMapLookup(n *mutMapNode, key any, order collCompare) *mutMapNode {
	for n != nil {
		switch cmp := collOrder(order, key, n.key); {
		case cmp < 0:
			n = n.left
		case cmp > 0:
			n = n.right
		default:
			return n
		}
	}
	return nil
}

// Comparisons finish before the first write. A comparator panic or cancellation
// observed at that commit boundary leaves the tree unchanged.
func mutMapInsert(n *mutMapNode, key, value any, order collCompare, ctx context.Context) (*mutMapNode, bool, error) {
	if n == nil {
		if err := ctx.Err(); err != nil {
			return nil, false, err
		}
		return &mutMapNode{key: key, value: value, height: 1}, true, nil
	}
	var child *mutMapNode
	var added bool
	var err error
	switch cmp := collOrder(order, key, n.key); {
	case cmp < 0:
		child, added, err = mutMapInsert(n.left, key, value, order, ctx)
		if err != nil {
			return n, false, err
		}
		n.left = child
	case cmp > 0:
		child, added, err = mutMapInsert(n.right, key, value, order, ctx)
		if err != nil {
			return n, false, err
		}
		n.right = child
	default:
		if err := ctx.Err(); err != nil {
			return n, false, err
		}
		n.key, n.value = key, value
		return n, false, nil
	}
	if !added {
		return n, false, nil
	}
	return mutMapBalance(n), true, nil
}

func mutMapRemoveMin(n *mutMapNode) (*mutMapNode, *mutMapNode) {
	if n.left == nil {
		return n, n.right
	}
	minimum, left := mutMapRemoveMin(n.left)
	n.left = left
	return minimum, mutMapBalance(n)
}

func mutMapDelete(n *mutMapNode, key any, order collCompare, ctx context.Context) (*mutMapNode, bool, error) {
	if n == nil {
		return nil, false, ctx.Err()
	}
	var child *mutMapNode
	var removed bool
	var err error
	switch cmp := collOrder(order, key, n.key); {
	case cmp < 0:
		child, removed, err = mutMapDelete(n.left, key, order, ctx)
		if err != nil || !removed {
			return n, false, err
		}
		n.left = child
	case cmp > 0:
		child, removed, err = mutMapDelete(n.right, key, order, ctx)
		if err != nil || !removed {
			return n, false, err
		}
		n.right = child
	default:
		if err := ctx.Err(); err != nil {
			return n, false, err
		}
		if n.left == nil {
			return n.right, true, nil
		}
		if n.right == nil {
			return n.left, true, nil
		}
		minimum, right := mutMapRemoveMin(n.right)
		n.key, n.value, n.right = minimum.key, minimum.value, right
	}
	return mutMapBalance(n), true, nil
}

func mutMapList(n *mutMapNode, rest any) any {
	if n == nil {
		return rest
	}
	rest = mutMapList(n.right, rest)
	return mutMapList(n.left, Shapes.Cons(Tuple2Of(n.key, n.value), rest))
}

func mutMapFreeze(n *mutMapNode) *collNode {
	if n == nil {
		return nil
	}
	return collBranch(n.key, n.value, mutMapFreeze(n.left), mutMapFreeze(n.right))
}
