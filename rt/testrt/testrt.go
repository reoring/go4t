// Package testrt supplies the erased IO boundary for gofx test's generated
// harness. It is command support, not an addition to the language prelude.
package testrt

import (
	"fmt"

	"gofx/rt"
)

// Run executes a suspended test exactly once, reports its outcome, and returns
// a successful IO containing whether the test passed. An IO error is a failed
// test, not a short circuit of the remaining harness.
func Run(name, action any) any {
	return rt.IO(func() (any, error) {
		result, err := action.(func() any)().(rt.IO)()
		if err != nil {
			fmt.Printf("FAIL %s: %s\n", name.(string), err)
			return false, nil
		}
		passed := result.(bool)
		if passed {
			fmt.Printf("PASS %s\n", name.(string))
		} else {
			fmt.Printf("FAIL %s\n", name.(string))
		}
		return passed, nil
	})
}
