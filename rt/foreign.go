package rt

import (
	"cmp"
	"slices"
)

// Foreign record registrations are emitted beside the ordinary generated ADTs.
// Keys include the shim import path, so unrelated binding modules cannot collide.
type foreignRecordShape struct {
	make  func([]any) any
	field func(any, int) any
}

var foreignRecords = map[string]foreignRecordShape{}

func RegisterForeignRecord(key string, makeRecord func([]any) any, field func(any, int) any) {
	foreignRecords[key] = foreignRecordShape{makeRecord, field}
}
func ForeignRecord(key string, fields []any) any {
	shape, ok := foreignRecords[key]
	if !ok {
		panic("go4t foreign record not registered: " + key)
	}
	return shape.make(fields)
}
func ForeignRecordField(key string, value any, index int) any {
	shape, ok := foreignRecords[key]
	if !ok {
		panic("go4t foreign record not registered: " + key)
	}
	return shape.field(value, index)
}
func OptionValue(value any) (any, bool) { return Shapes.OptionValue(value) }

// ForeignMap builds the ordinary immutable Map directly from copied entries.
// Scalar ordering is the same ordering used by the prelude's Ord instances.
func ForeignMap(entries [][2]any) any {
	slices.SortFunc(entries, func(a, b [2]any) int { return foreignCompare(a[0], b[0]) })
	var build func(int, int) *collNode
	build = func(lo, hi int) *collNode {
		if lo == hi {
			return nil
		}
		mid := (lo + hi) / 2
		return collBranch(entries[mid][0], entries[mid][1], build(lo, mid), build(mid+1, hi))
	}
	return mapValue{root: build(0, len(entries))}
}
func foreignCompare(a, b any) int {
	switch a := a.(type) {
	case int64:
		return cmp.Compare(a, b.(int64))
	case float64:
		return cmp.Compare(a, b.(float64))
	case string:
		return cmp.Compare(a, b.(string))
	case bool:
		if a == b.(bool) {
			return 0
		}
		if !a {
			return -1
		}
		return 1
	default:
		panic("go4t foreign map: non-scalar key")
	}
}
func ForeignMapEach(value any, visit func(any, any)) {
	collWalk(value.(mapValue).root, func(n *collNode) { visit(n.key, n.value) })
}
