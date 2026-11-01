package rt

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
