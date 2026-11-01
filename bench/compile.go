package bench

import (
	"fmt"
	"strings"
)

// CompileSource deterministically emits n derived data types, n constrained
// functions and n match expressions. n must be nonnegative. At n=400 the
// workload is 5,203 lines, excluding the separately injected diagnostic probe.
func CompileSource(n int) string {
	if n < 0 {
		panic("compile scale must be nonnegative")
	}
	var source strings.Builder
	source.WriteString("package main\n\n")
	for i := range n {
		fmt.Fprintf(&source, `type Item%d[A] = Item%d(A) deriving Eq, Ord, Debug

func inspect%d[A Eq + Debug](value Item%d[A], expected A) String {
  match value {
    Item%d(x) => if eq(x, expected) {
      debug(value)
    } else {
      debug(x)
    }
  }
}

// End of item %d.
`, i, i, i, i, i, i)
	}
	source.WriteString("func main() IO[Unit] { ioPure(unit()) }\n")
	return source.String()
}

// diagnosticProbe adds one mismatch under depth nested Option constructors.
// The checker currently returns a single diagnostic; its full rendered length
// (including position) is the longest diagnostic for this injection.
func diagnosticProbe(depth int) string {
	return "\nfunc broken() " + strings.Repeat("Option[", depth) + "Int" +
		strings.Repeat("]", depth) + " { " + strings.Repeat("Some(", depth) +
		"true" + strings.Repeat(")", depth) + " }\n"
}
