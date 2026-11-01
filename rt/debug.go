package rt

import (
	"strconv"
	"strings"
)

// DebugString renders a string as a quoted Go string literal through the erased ABI.
func DebugString(s any) any { return strconv.Quote(s.(string)) }

// DebugFloat preserves Float's structural distinction from Int through the erased ABI.
func DebugFloat(f any) any {
	s := strconv.FormatFloat(f.(float64), 'g', -1, 64)
	if !strings.ContainsAny(s, ".e") && !strings.Contains(s, "NaN") && !strings.Contains(s, "Inf") {
		s += ".0"
	}
	return s
}
