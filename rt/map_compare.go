package rt

import "cmp"

// nativeOrd is a comparator token, not a claim inferred from a key's Go type.
// Only compiler-marked prelude dictionaries can produce one.
type nativeOrd uint8

// NativeOrdKind identifies prelude Int (1) and String (2) ordering. Unmarked
// dictionaries and arbitrary comparator functions return zero.
func NativeOrdKind(value any) int {
	var kind int
	switch value := value.(type) {
	case nativeOrd:
		kind = int(value)
	case interface{ Go4tNativeOrd() int }:
		kind = value.Go4tNativeOrd()
	}
	if kind == 1 || kind == 2 {
		return kind
	}
	return 0
}

// NativeCompare preserves an Ord witness across the runtime comparator boundary.
// Ordinary function values remain ordinary functions everywhere else.
func NativeCompare(dict interface{ Compare(any, any) any }) any {
	if kind := NativeOrdKind(dict); kind != 0 {
		return nativeOrd(kind)
	}
	return dict.Compare
}

type collCompare struct {
	native nativeOrd
	call   func(any, any) any
}

func collComparator(value any) collCompare {
	if kind, ok := value.(nativeOrd); ok {
		return collCompare{native: kind}
	}
	return collCompare{call: value.(func(any, any) any)}
}

func collOrder(order collCompare, a, b any) int {
	if order.native != 0 {
		switch a := a.(type) {
		case int64:
			if order.native == 1 {
				return cmp.Compare(a, b.(int64))
			}
		case string:
			if order.native == 2 {
				return cmp.Compare(a, b.(string))
			}
		}
		panic("rt.NativeCompare: key does not match native Ord")
	}
	if Shapes.OrderingOf == nil {
		panic("rt.Shapes is not initialized: OrderingOf is required")
	}
	return Shapes.OrderingOf(order.call(a, b))
}
