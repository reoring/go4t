package rt

import "strings"

func AddInt(a, b any) any { return a.(int64) + b.(int64) }
func SubInt(a, b any) any { return a.(int64) - b.(int64) }
func MulInt(a, b any) any { return a.(int64) * b.(int64) }
func DivInt(a, b any) any { return a.(int64) / b.(int64) }
func NegInt(a any) any    { return -a.(int64) }

func EqInt(a, b any) any { return a.(int64) == b.(int64) }
func NeInt(a, b any) any { return a.(int64) != b.(int64) }
func LtInt(a, b any) any { return a.(int64) < b.(int64) }
func LeInt(a, b any) any { return a.(int64) <= b.(int64) }
func GtInt(a, b any) any { return a.(int64) > b.(int64) }
func GeInt(a, b any) any { return a.(int64) >= b.(int64) }

func AddString(a, b any) any { return a.(string) + b.(string) }

// Concat joins the string values produced by interpolation text and Show calls.
// Like the other erased operations, its arguments must have the checked type.
func Concat(parts ...any) any {
	if len(parts) == 1 {
		return parts[0].(string)
	}
	size := 0
	for _, part := range parts {
		size += len(part.(string))
	}
	var out strings.Builder
	out.Grow(size)
	for _, part := range parts {
		out.WriteString(part.(string))
	}
	return out.String()
}

func EqString(a, b any) any { return a.(string) == b.(string) }
func NeString(a, b any) any { return a.(string) != b.(string) }
func LtString(a, b any) any { return a.(string) < b.(string) }
func LeString(a, b any) any { return a.(string) <= b.(string) }
func GtString(a, b any) any { return a.(string) > b.(string) }
func GeString(a, b any) any { return a.(string) >= b.(string) }

func EqBool(a, b any) any { return a.(bool) == b.(bool) }
func NeBool(a, b any) any { return a.(bool) != b.(bool) }
func And(a, b any) any    { return a.(bool) && b.(bool) }
func Or(a, b any) any     { return a.(bool) || b.(bool) }
func Not(a any) any       { return !a.(bool) }
