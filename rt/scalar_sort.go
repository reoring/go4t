package rt

import (
	"cmp"
	"slices"
)

// Native scalar equality makes unstable sorting indistinguishable from stable
// sorting. Other comparators, including equal keys of records, remain stable.
func arraySortBy(a, compare any) any {
	values := a.(arrayValue)
	if len(values) < 2 {
		return a
	}
	switch NativeOrdKind(compare) {
	case 1:
		native := make([]int64, len(values))
		for i, value := range values {
			native[i] = value.(int64)
		}
		slices.Sort(native)
		result := make(arrayValue, len(values))
		for i, value := range native {
			result[i] = value
		}
		return result
	case 2:
		native := make([]string, len(values))
		for i, value := range values {
			native[i] = value.(string)
		}
		slices.Sort(native)
		result := make(arrayValue, len(values))
		for i, value := range native {
			result[i] = value
		}
		return result
	}
	compareFn := collComparator(compare)
	result := slices.Clone(values)
	slices.SortStableFunc(result, func(a, b any) int {
		return collOrder(compareFn, a, b)
	})
	return result
}

// ListSortBy uses only fresh storage; the input list and its elements are never
// changed. The compiler supplies a native tag only for the prelude ordering.
func ListSortBy(xs, compare any) any {
	return ArrayToList(arraySortBy(ArrayFromList(xs), compare))
}

// ListSortOn evaluates each key once, from left to right, and preserves the
// original order among equal keys, including keys with a custom Ord instance.
func ListSortOn(xs, key, compare any) any {
	type keyed struct{ key, value any }
	project := key.(func(any) any)
	var values []keyed
	for {
		value, rest, ok := Shapes.Uncons(xs)
		if !ok {
			break
		}
		values = append(values, keyed{project(value), value})
		xs = rest
	}
	switch NativeOrdKind(compare) {
	case 1:
		slices.SortStableFunc(values, func(a, b keyed) int { return cmp.Compare(a.key.(int64), b.key.(int64)) })
	case 2:
		slices.SortStableFunc(values, func(a, b keyed) int { return cmp.Compare(a.key.(string), b.key.(string)) })
	default:
		compareFn := collComparator(compare)
		slices.SortStableFunc(values, func(a, b keyed) int { return collOrder(compareFn, a.key, b.key) })
	}
	result := Shapes.Nil()
	for i := len(values) - 1; i >= 0; i-- {
		result = Shapes.Cons(values[i].value, result)
	}
	return result
}
