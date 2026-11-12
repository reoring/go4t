// Package testgo lets a test state which Go release it needs.
//
// The compiler supports every Go release from the `go` line in go.mod upward,
// and CI runs the suite on that minimum and on the current stable release.
// A test that binds a standard-library API introduced later (iter, maps.Keys,
// slices.Sorted) or a third-party module with a newer `go` directive calls
// Require so that it is skipped, with the reason, on older toolchains instead
// of failing for a reason unrelated to go4t.
package testgo

import (
	"fmt"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"testing"
)

var version = sync.OnceValues(func() (int, error) {
	out, err := exec.Command("go", "env", "GOVERSION").Output()
	if err != nil {
		return 0, fmt.Errorf("go env GOVERSION: %w", err)
	}
	return minor(strings.TrimSpace(string(out)))
})

// minor extracts N from "go1.N", "go1.N.P" and "go1.NrcK".
func minor(goVersion string) (int, error) {
	rest, ok := strings.CutPrefix(goVersion, "go1.")
	if !ok {
		return 0, fmt.Errorf("unrecognized Go version %q", goVersion)
	}
	end := 0
	for end < len(rest) && rest[end] >= '0' && rest[end] <= '9' {
		end++
	}
	return strconv.Atoi(rest[:end])
}

// Require skips the test unless the `go` command on PATH is at least Go 1.N.
// The reason names what the test needs from that release.
func Require(t testing.TB, n int, reason string) {
	t.Helper()
	have, err := version()
	if err != nil {
		t.Skipf("needs Go 1.%d (%s): %v", n, reason, err)
	}
	if have < n {
		t.Skipf("needs Go 1.%d (%s); running Go 1.%d", n, reason, have)
	}
}
