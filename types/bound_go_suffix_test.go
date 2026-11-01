package types_test

import (
	"strings"
	"testing"
)

// A Go type that bind renamed with the Go suffix is pointed to from the
// diagnostic for its unsuffixed spelling.
func TestBoundGoSuffixHint(t *testing.T) {
	root := moduleFixture(t, map[string]string{
		"main.gfx":         "package main\nimport \"sql\"\nfunc f(r sql.Result) Int { 1 }\nfunc main() IO[Unit] { ioPure(unit()) }",
		"sql/bindings.gfx": "package sql\ntype ResultGo = ResultGoHandle",
	})
	_, err := checkModuleFixture(t, root)
	if err == nil || !strings.Contains(err.Error(), "bound as sql.ResultGo because Result is a prelude name") {
		t.Fatalf("diagnostic = %v", err)
	}
}
