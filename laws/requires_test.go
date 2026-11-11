package laws

import (
	"math"
	"strings"
	"testing"
)

func TestRequiresFixtures(t *testing.T) {
	for _, file := range []string{"requires_order.gt", "requires_ttl.gt", "requires_combination.gt"} {
		t.Run(file, func(t *testing.T) {
			if errors := CheckProgram(loadFixture(t, file)); len(errors) != 0 {
				t.Fatal(errors)
			}
		})
	}
	for _, tc := range []struct {
		file    string
		count   int
		message string
	}{
		{"requires_wrap_bad.gt", 2, "stuck normal forms differ"},
		{"requires_rewrite_bad.gt", 1, "rewrite successor: precondition not established:"},
	} {
		t.Run(tc.file, func(t *testing.T) {
			errors := CheckProgram(loadFixture(t, tc.file))
			if len(errors) != tc.count {
				t.Fatalf("diagnostics = %v", errors)
			}
			for _, err := range errors {
				if !strings.Contains(err.Msg, tc.message) {
					t.Fatal(err)
				}
			}
		})
	}
}

func TestPreconditionHolds(t *testing.T) {
	e := newEvaluator(loadFixture(t, "requires_order.gt"))
	law, err := e.extractLaw("successor")
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		value int64
		want  bool
	}{{math.MinInt64, true}, {0, true}, {math.MaxInt64 - 1, true}, {math.MaxInt64, false}} {
		e.fuel = StepLimit
		env := child(nil)
		env.values["a"] = integer(tc.value)
		if err := e.assumePrecondition(law, env); err != nil {
			t.Fatal(err)
		}
		got, err := e.preconditionHolds(law, env)
		if err != nil || got != tc.want {
			t.Fatalf("%d: got %v, %v; want %v", tc.value, got, err, tc.want)
		}
	}
}
