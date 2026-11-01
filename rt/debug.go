package rt

import "strconv"

// DebugString renders a string as a quoted Go string literal through the erased ABI.
func DebugString(s any) any { return strconv.Quote(s.(string)) }
