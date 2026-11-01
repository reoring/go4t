package rt

import "sort"

// Map and Set share immutable AVL nodes. Updates copy only the search path;
// cached heights and sizes keep lookup/update logarithmic and size constant-time.
// The distinct wrappers remain opaque to generated gofx programs.
type mapValue struct{ root *collNode }
type setValue struct{ root *collNode }
type arrayValue []any

type collNode struct {
	key, value   any
	left, right  *collNode
	height, size int
}

type collCompare func(any, any) any

func collHeight(n *collNode) int {
	if n == nil {
		return 0
	}
	return n.height
}

func collSize(n *collNode) int {
	if n == nil {
		return 0
	}
	return n.size
}

func collBranch(key, value any, left, right *collNode) *collNode {
	return &collNode{key: key, value: value, left: left, right: right,
		height: 1 + max(collHeight(left), collHeight(right)), size: 1 + collSize(left) + collSize(right)}
}

// Balance constructs the new path nodes without ever mutating a shared child.
func collBalance(key, value any, left, right *collNode) *collNode {
	if collHeight(left) > collHeight(right)+1 {
		if collHeight(left.left) >= collHeight(left.right) {
			return collBranch(left.key, left.value, left.left, collBranch(key, value, left.right, right))
		}
		middle := left.right
		return collBranch(middle.key, middle.value,
			collBranch(left.key, left.value, left.left, middle.left), collBranch(key, value, middle.right, right))
	}
	if collHeight(right) > collHeight(left)+1 {
		if collHeight(right.right) >= collHeight(right.left) {
			return collBranch(right.key, right.value, collBranch(key, value, left, right.left), right.right)
		}
		middle := right.left
		return collBranch(middle.key, middle.value,
			collBranch(key, value, left, middle.left), collBranch(right.key, right.value, middle.right, right.right))
	}
	return collBranch(key, value, left, right)
}

func collOrder(cmp collCompare, a, b any) int {
	if Shapes.OrderingOf == nil {
		panic("rt.Shapes is not initialized: OrderingOf is required")
	}
	return Shapes.OrderingOf(cmp(a, b))
}

func collLookup(root *collNode, key any, cmp collCompare) *collNode {
	for root != nil {
		switch order := collOrder(cmp, key, root.key); {
		case order < 0:
			root = root.left
		case order > 0:
			root = root.right
		default:
			return root
		}
	}
	return nil
}

func collInsert(root *collNode, key, value any, cmp collCompare, replace bool) *collNode {
	if root == nil {
		return collBranch(key, value, nil, nil)
	}
	switch order := collOrder(cmp, key, root.key); {
	case order < 0:
		left := collInsert(root.left, key, value, cmp, replace)
		if left == root.left {
			return root
		}
		return collBalance(root.key, root.value, left, root.right)
	case order > 0:
		right := collInsert(root.right, key, value, cmp, replace)
		if right == root.right {
			return root
		}
		return collBalance(root.key, root.value, root.left, right)
	default:
		if !replace {
			return root
		}
		return collBranch(key, value, root.left, root.right)
	}
}

func collRemoveMin(root *collNode) (*collNode, *collNode) {
	if root.left == nil {
		return root, root.right
	}
	minimum, left := collRemoveMin(root.left)
	return minimum, collBalance(root.key, root.value, left, root.right)
}

func collDelete(root *collNode, key any, cmp collCompare) *collNode {
	if root == nil {
		return nil
	}
	switch order := collOrder(cmp, key, root.key); {
	case order < 0:
		left := collDelete(root.left, key, cmp)
		if left == root.left {
			return root
		}
		return collBalance(root.key, root.value, left, root.right)
	case order > 0:
		right := collDelete(root.right, key, cmp)
		if right == root.right {
			return root
		}
		return collBalance(root.key, root.value, root.left, right)
	default:
		if root.left == nil {
			return root.right
		}
		if root.right == nil {
			return root.left
		}
		minimum, right := collRemoveMin(root.right)
		return collBalance(minimum.key, minimum.value, root.left, right)
	}
}

func collWalk(root *collNode, visit func(*collNode)) {
	if root == nil {
		return
	}
	collWalk(root.left, visit)
	visit(root)
	collWalk(root.right, visit)
}

// Reverse traversal prepends directly to the generated list, avoiding an
// intermediate Go slice while producing ascending-key order.
func collList(root *collNode, rest any, value func(*collNode) any) any {
	if root == nil {
		return rest
	}
	rest = collList(root.right, rest, value)
	return collList(root.left, Shapes.Cons(value(root), rest), value)
}

func EmptyMap() any { return mapValue{} }

func MapInsert(m, key, value, compare any) any {
	return mapValue{collInsert(m.(mapValue).root, key, value, compare.(func(any, any) any), true)}
}

func MapLookup(m, key, compare any) any {
	if n := collLookup(m.(mapValue).root, key, compare.(func(any, any) any)); n != nil {
		return SomeOf(n.value)
	}
	return NoneOf()
}

func MapDelete(m, key, compare any) any {
	return mapValue{collDelete(m.(mapValue).root, key, compare.(func(any, any) any))}
}

func MapMember(m, key, compare any) any {
	return collLookup(m.(mapValue).root, key, compare.(func(any, any) any)) != nil
}

func MapSize(m any) any { return int64(collSize(m.(mapValue).root)) }

// Duplicate keys use the last value in the input list.
func MapFromList(xs, compare any) any {
	cmp := compare.(func(any, any) any)
	var root *collNode
	for {
		pair, rest, ok := Shapes.Uncons(xs)
		if !ok {
			return mapValue{root}
		}
		key, value := Shapes.Untuple2(pair)
		root = collInsert(root, key, value, cmp, true)
		xs = rest
	}
}

func MapToList(m any) any {
	return collList(m.(mapValue).root, Shapes.Nil(), func(n *collNode) any { return Tuple2Of(n.key, n.value) })
}

func MapKeys(m any) any {
	return collList(m.(mapValue).root, Shapes.Nil(), func(n *collNode) any { return n.key })
}

func MapValues(m any) any {
	return collList(m.(mapValue).root, Shapes.Nil(), func(n *collNode) any { return n.value })
}

func MapUnion(a, b, compare any) any {
	root := a.(mapValue).root
	cmp := compare.(func(any, any) any)
	collWalk(b.(mapValue).root, func(n *collNode) { root = collInsert(root, n.key, n.value, cmp, false) })
	return mapValue{root}
}

func collMap(root *collNode, f func(any) any) *collNode {
	if root == nil {
		return nil
	}
	left := collMap(root.left, f)
	value := f(root.value)
	right := collMap(root.right, f)
	return collBranch(root.key, value, left, right)
}

func MapMap(m, f any) any {
	return mapValue{collMap(m.(mapValue).root, f.(func(any) any))}
}

func MapFold(m, init, f any) any {
	fold := f.(func(any, any, any) any)
	collWalk(m.(mapValue).root, func(n *collNode) { init = fold(init, n.key, n.value) })
	return init
}

func EmptySet() any { return setValue{} }

func SetInsert(s, value, compare any) any {
	return setValue{collInsert(s.(setValue).root, value, nil, compare.(func(any, any) any), false)}
}

func SetDelete(s, value, compare any) any {
	return setValue{collDelete(s.(setValue).root, value, compare.(func(any, any) any))}
}

func SetMember(s, value, compare any) any {
	return collLookup(s.(setValue).root, value, compare.(func(any, any) any)) != nil
}

func SetSize(s any) any { return int64(collSize(s.(setValue).root)) }

func SetFromList(xs, compare any) any {
	cmp := compare.(func(any, any) any)
	var root *collNode
	for {
		x, rest, ok := Shapes.Uncons(xs)
		if !ok {
			return setValue{root}
		}
		root = collInsert(root, x, nil, cmp, false)
		xs = rest
	}
}

func SetToList(s any) any {
	return collList(s.(setValue).root, Shapes.Nil(), func(n *collNode) any { return n.key })
}

func SetUnion(a, b, compare any) any {
	root := a.(setValue).root
	cmp := compare.(func(any, any) any)
	collWalk(b.(setValue).root, func(n *collNode) { root = collInsert(root, n.key, nil, cmp, false) })
	return setValue{root}
}

func SetIntersection(a, b, compare any) any {
	root, other := a.(setValue).root, b.(setValue).root
	if other == nil {
		return EmptySet()
	}
	cmp := compare.(func(any, any) any)
	collWalk(a.(setValue).root, func(n *collNode) {
		if collLookup(other, n.key, cmp) == nil {
			root = collDelete(root, n.key, cmp)
		}
	})
	return setValue{root}
}

func SetDifference(a, b, compare any) any {
	root := a.(setValue).root
	cmp := compare.(func(any, any) any)
	collWalk(b.(setValue).root, func(n *collNode) { root = collDelete(root, n.key, cmp) })
	return setValue{root}
}

func ArrayFromList(xs any) any {
	var values arrayValue
	for {
		x, rest, ok := Shapes.Uncons(xs)
		if !ok {
			return values
		}
		values = append(values, x)
		xs = rest
	}
}

func ArrayToList(a any) any {
	values := a.(arrayValue)
	xs := Shapes.Nil()
	for i := len(values) - 1; i >= 0; i-- {
		xs = Shapes.Cons(values[i], xs)
	}
	return xs
}

func ArrayLength(a any) any { return int64(len(a.(arrayValue))) }

func ArrayGet(a, index any) any {
	values, i := a.(arrayValue), index.(int64)
	if i < 0 || i >= int64(len(values)) {
		return NoneOf()
	}
	return SomeOf(values[i])
}

func ArraySet(a, index, value any) any {
	values, i := a.(arrayValue), index.(int64)
	if i < 0 || i >= int64(len(values)) {
		return a
	}
	result := make(arrayValue, len(values))
	copy(result, values)
	result[i] = value
	return result
}

func ArrayMap(a, f any) any {
	values, apply := a.(arrayValue), f.(func(any) any)
	result := make(arrayValue, len(values))
	for i, value := range values {
		result[i] = apply(value)
	}
	return result
}

func ArrayFold(a, init, f any) any {
	fold := f.(func(any, any) any)
	for _, value := range a.(arrayValue) {
		init = fold(init, value)
	}
	return init
}

func ArrayPush(a, value any) any {
	values := a.(arrayValue)
	result := make(arrayValue, len(values)+1)
	copy(result, values)
	result[len(values)] = value
	return result
}

// Slice is half-open, with both bounds clamped to [0, length]. A reversed
// interval is empty. Sharing the storage is safe because every write copies.
func ArraySlice(a, start, end any) any {
	values := a.(arrayValue)
	n := int64(len(values))
	lo, hi := min(max(start.(int64), 0), n), min(max(end.(int64), 0), n)
	if hi <= lo {
		return arrayValue(nil)
	}
	return values[lo:hi:hi]
}

func ArraySortBy(a, compare any) any {
	values := a.(arrayValue)
	if len(values) < 2 {
		return a
	}
	cmp := compare.(func(any, any) any)
	result := make(arrayValue, len(values))
	copy(result, values)
	sort.SliceStable(result, func(i, j int) bool { return collOrder(cmp, result[i], result[j]) < 0 })
	return result
}
